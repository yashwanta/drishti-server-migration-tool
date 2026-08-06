package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/cutover"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
	"github.com/drishti/hypershift/internal/platform/proxmox"
	"github.com/drishti/hypershift/internal/preflight"
	"github.com/drishti/hypershift/internal/remediation"
	"github.com/drishti/hypershift/internal/reports"
	"github.com/drishti/hypershift/internal/validation"
)

type MigrationHandlers struct {
	*Handlers
	jobEng            *job.Engine
	pre               *preflight.Engine
	cut               *cutover.Engine
	factory           platform.AdapterFactory
	workDir           string
	executionDisabled bool
	executionLocked   bool
	real              *proxmox.Probe
}

func NewMigrationHandlers(h *Handlers, jobEng *job.Engine, factory platform.AdapterFactory, workDir string) *MigrationHandlers {
	return &MigrationHandlers{Handlers: h, jobEng: jobEng, pre: preflight.New(), cut: cutover.New(), factory: factory, workDir: workDir, executionDisabled: true, executionLocked: true}
}

// DisableExecution permanently locks mutating migration routes for this
// process. Adapter, persistence, and worker wiring cannot override the lock.
func (m *MigrationHandlers) DisableExecution() {
	m.executionDisabled = true
	m.executionLocked = true
}

// ConfigureLabExecution keeps migration mutation routes fail-closed unless the
// runtime is exactly lab mode and both independent operator interlocks are
// enabled. Mock, live, and production can never be unlocked by this method.
// The permanent lock remains set so legacy enabling hooks cannot change the
// decision after startup.
func (m *MigrationHandlers) ConfigureLabExecution(mode config.RunMode, enableLabMigration, enablePlatformMutation bool) {
	m.executionDisabled = true
	m.executionLocked = true
	if mode == config.ModeLab && enableLabMigration && enablePlatformMutation {
		m.executionDisabled = false
	}
}

// EnableLabRemoteMigration installs the real Proxmox migration path. Callers
// must only invoke this after the explicit lab mutation interlock is enabled.
func (m *MigrationHandlers) EnableLabRemoteMigration(real *proxmox.Probe, enableExecution bool) {
	m.real = real
	if !m.executionLocked {
		m.executionDisabled = !enableExecution
	}
}

func (m *MigrationHandlers) RegisterMigration(mx *http.ServeMux) {
	mx.HandleFunc("POST /api/v1/plans/{id}/preflight", m.runPreflight)
	mx.HandleFunc("POST /api/v1/plans/{id}/approve", m.approvePlan)
	mx.HandleFunc("POST /api/v1/plans/{id}/execute", m.executeMigration)
	mx.HandleFunc("GET /api/v1/jobs", m.listJobs)
	mx.HandleFunc("GET /api/v1/jobs/{id}", m.getJob)
	mx.HandleFunc("POST /api/v1/jobs/{id}/validate", m.validateJob)
	mx.HandleFunc("POST /api/v1/jobs/{id}/cutover", m.cutoverJob)
	mx.HandleFunc("POST /api/v1/jobs/{id}/rollback", m.rollbackJob)
	mx.HandleFunc("GET /api/v1/jobs/{id}/report", m.getReport)
}

type preflightResponse struct {
	PlanID  string                  `json:"plan_id"`
	Pass    bool                    `json:"pass"`
	Checks  []domain.PreflightCheck `json:"checks"`
	Blocked []string                `json:"blocked,omitempty"`
}

func (m *MigrationHandlers) resolveSourceTarget(plan domain.Plan) (domain.Connection, domain.Connection, error) {
	srcConn, ok := m.mock.Connection(plan.SourceConnID)
	if !ok {
		return domain.Connection{}, domain.Connection{}, errNotFound("source connection")
	}
	tgtConn, ok := m.mock.Connection(plan.TargetConnID)
	if !ok {
		return domain.Connection{}, domain.Connection{}, errNotFound("target connection")
	}
	return srcConn, tgtConn, nil
}

func errNotFound(name string) error { return &notFoundErr{name: name} }

type notFoundErr struct{ name string }

func (e *notFoundErr) Error() string { return e.name + " not found" }

func (m *MigrationHandlers) runPreflight(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	plan, ok, err := m.plans.GetContext(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, err := m.resolveSourceTarget(plan)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: err.Error(), Code: "bad_request"})
		return
	}
	if m.real != nil {
		checks, err := m.real.RemotePreflight(r.Context(), plan, srcConn, tgtConn)
		if err != nil {
			writeErr(w, http.StatusBadGateway, errorBody{Error: "real preflight failed", Detail: err.Error(), Code: "preflight_failed"})
			return
		}
		pass := true
		blocked := []string{}
		for _, check := range checks {
			if check.Status == domain.CheckFail {
				pass = false
				blocked = append(blocked, check.Message)
			}
		}
		plan.Status = domain.PlanPreflight
		plan.PreflightPassed = pass
		if err := m.plans.PutContext(r.Context(), plan); err != nil {
			writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, preflightResponse{PlanID: plan.ID, Pass: pass, Checks: checks, Blocked: blocked})
		return
	}
	srcAdapter, tgtAdapter, err := m.resolveAdapters(srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "platform adapter unavailable", Detail: err.Error(), Code: "adapter_unavailable"})
		return
	}
	ctx := r.Context()
	vm, err := srcAdapter.VM(ctx, plan.SourceVMID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: err.Error(), Code: "bad_request"})
		return
	}
	node, err := tgtAdapter.Node(ctx, plan.TargetNodeID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: err.Error(), Code: "bad_request"})
		return
	}
	if len(plan.StorageMaps) == 0 {
		plan.StorageMaps = autoStorageMaps(vm, node)
		if err := m.plans.PutContext(r.Context(), plan); err != nil {
			writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
			return
		}
	}
	result := m.pre.Run(plan, vm, node)
	plan.Status = domain.PlanPreflight
	plan.PreflightPassed = result.Pass
	if err := m.plans.PutContext(r.Context(), plan); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if err := m.audit.AppendContext(r.Context(), domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: auth.Actor(r.Context()), Action: "plan.preflight", Target: plan.ID, Result: boolStr(result.Pass), Detail: "Preflight checks ran."}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, preflightResponse{PlanID: plan.ID, Pass: result.Pass, Checks: result.Checks, Blocked: result.Blocked})
}

func boolStr(b bool) string {
	if b {
		return "pass"
	}
	return "blocked"
}

func autoStorageMaps(vm domain.VM, node domain.TargetNode) []domain.StorageMap {
	var maps []domain.StorageMap
	defaultStorage := ""
	if len(node.Storage) > 0 {
		defaultStorage = node.Storage[0].ID
	}
	for _, d := range vm.Disks {
		maps = append(maps, domain.StorageMap{SourceDiskID: d.ID, TargetStorageID: defaultStorage, TargetFormat: domain.DiskQCOW2})
	}
	return maps
}

func (m *MigrationHandlers) approvePlan(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	plan, ok, err := m.plans.GetContext(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	if plan.Status != domain.PlanPreflight || !plan.PreflightPassed {
		writeErr(w, http.StatusConflict, errorBody{Error: "plan requires a passing preflight before approval", Code: "preflight_required"})
		return
	}
	var approval struct {
		ApproveSourcePowerOff bool `json:"approve_source_power_off"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &approval, 1<<10); err != nil {
			writeErr(w, http.StatusBadRequest, errorBody{Error: "invalid approval request", Detail: err.Error(), Code: "bad_request"})
			return
		}
	}
	detail := "Migration plan approved; source power-off was not authorized."
	if approval.ApproveSourcePowerOff {
		detail = "Migration plan and source power-off for the named source VM were explicitly approved."
	}
	event := domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: auth.Actor(r.Context()), Action: "plan.approve", Target: plan.ID, Result: "approved", Detail: detail}
	plan, ok, err = m.plans.ApproveContext(r.Context(), plan.ID, approval.ApproveSourcePowerOff, event, m.audit)
	if err != nil {
		writeErr(w, http.StatusConflict, errorBody{Error: err.Error(), Code: "preflight_required"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (m *MigrationHandlers) executeMigration(w http.ResponseWriter, r *http.Request) {
	if m.executionDisabled {
		m.writeExecutionDisabled(w)
		return
	}
	id := normalizeID(r.PathValue("id"))
	plan, ok, err := m.plans.GetContext(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	if plan.Status != domain.PlanApproved {
		writeErr(w, http.StatusConflict, errorBody{Error: "plan must be approved before execution", Code: "conflict"})
		return
	}
	srcConn, tgtConn, err := m.resolveSourceTarget(plan)
	if err != nil {
		writeErr(w, http.StatusBadRequest, errorBody{Error: err.Error(), Code: "bad_request"})
		return
	}
	if m.real != nil {
		result, err := m.real.StartRemoteMigration(r.Context(), proxmox.RemoteMigrationRequest{Plan: plan, Source: srcConn, Target: tgtConn})
		if err != nil {
			writeErr(w, http.StatusConflict, errorBody{Error: "remote migration refused", Detail: err.Error(), Code: "migration_refused"})
			return
		}
		plan.TargetVMID = &result.TargetVMID
		if err := m.plans.PutContext(r.Context(), plan); err != nil {
			writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
			return
		}
		j, err := m.jobEng.StartExternal(r.Context(), plan, result.UPID)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
			defer cancel()
			err := m.real.WaitRemoteMigration(ctx, srcConn, result, func(progress proxmox.RemoteMigrationProgress) {
				_ = m.jobEng.UpdateExternal(ctx, j.ID, progress.Message)
			})
			if err == nil {
				err = m.real.VerifyRemoteMigration(ctx, srcConn, tgtConn, result)
			}
			_ = m.jobEng.FinishExternal(ctx, j.ID, err)
		}()
		writeJSON(w, http.StatusAccepted, j)
		return
	}
	srcAdapter, tgtAdapter, err := m.resolveAdapters(srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "platform adapter unavailable", Detail: err.Error(), Code: "adapter_unavailable"})
		return
	}
	ctx := r.Context()
	vm, err := srcAdapter.VM(ctx, plan.SourceVMID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, errorBody{Error: "source VM lookup failed", Detail: err.Error(), Code: "source_lookup_failed"})
		return
	}
	node, err := tgtAdapter.Node(ctx, plan.TargetNodeID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, errorBody{Error: "target node lookup failed", Detail: err.Error(), Code: "target_lookup_failed"})
		return
	}
	if len(plan.StorageMaps) == 0 {
		plan.StorageMaps = autoStorageMaps(vm, node)
		if err := m.plans.PutContext(r.Context(), plan); err != nil {
			writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
			return
		}
	}
	j, err := m.jobEng.ExecutePlan(ctx, plan, vm, node, srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errorBody{Error: err.Error(), Code: "execution_failed"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (m *MigrationHandlers) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := m.jobEng.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (m *MigrationHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok, err := m.jobEng.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

type validateResponse struct {
	Passed bool                  `json:"passed"`
	Checks []validationCheckJSON `json:"checks"`
}
type validationCheckJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (m *MigrationHandlers) validateJob(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok, err := m.jobEng.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok, err := m.plans.GetContext(r.Context(), j.PlanID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, tgtAdapter, err := m.resolveAdapters(srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "platform adapter unavailable", Detail: err.Error(), Code: "adapter_unavailable"})
		return
	}
	ctx := r.Context()
	vm, err := srcAdapter.VM(ctx, plan.SourceVMID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, errorBody{Error: "source VM lookup failed", Detail: err.Error(), Code: "source_lookup_failed"})
		return
	}
	prof := validation.Default(vm)
	vmid := 0
	if plan.TargetVMID != nil {
		vmid = *plan.TargetVMID
	}
	state, _ := tgtAdapter.VMState(ctx, vmid)
	facts := remediation.CollectFacts(vm)
	result := validation.Run(prof, vm, state, facts)
	checks := make([]validationCheckJSON, len(result.Checks))
	for i, c := range result.Checks {
		checks[i] = validationCheckJSON{Name: c.Name, Status: c.Status, Detail: c.Detail}
	}
	if err := m.audit.AppendContext(r.Context(), domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: auth.Actor(r.Context()), Action: "job.validate", Target: j.ID, Result: boolStr(result.Passed), Detail: "Validation ran."}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, validateResponse{Passed: result.Passed, Checks: checks})
}

type cutoverResponse struct {
	Success           bool     `json:"success"`
	Steps             []string `json:"steps"`
	Warning           string   `json:"warning,omitempty"`
	RetentionDeadline string   `json:"retention_deadline,omitempty"`
}

func (m *MigrationHandlers) cutoverJob(w http.ResponseWriter, r *http.Request) {
	if m.executionDisabled {
		m.writeExecutionDisabled(w)
		return
	}
	id := normalizeID(r.PathValue("id"))
	j, ok, err := m.jobEng.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok, err := m.plans.GetContext(r.Context(), j.PlanID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, tgtAdapter, err := m.resolveAdapters(srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "platform adapter unavailable", Detail: err.Error(), Code: "adapter_unavailable"})
		return
	}
	ctx := r.Context()
	vm, err := srcAdapter.VM(ctx, plan.SourceVMID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, errorBody{Error: "source VM lookup failed", Detail: err.Error(), Code: "source_lookup_failed"})
		return
	}
	facts := remediation.CollectFacts(vm)
	prof := validation.Default(vm)
	vmid := 0
	if plan.TargetVMID != nil {
		vmid = *plan.TargetVMID
	}
	state, _ := tgtAdapter.VMState(ctx, vmid)
	valResult := validation.Run(prof, vm, state, facts)
	res, err := m.cut.Cutover(ctx, srcAdapter, tgtAdapter, plan, vm, facts, valResult, "vmbr0", 10)
	if err != nil {
		writeErr(w, http.StatusConflict, errorBody{Error: res.Warning, Code: "cutover_refused"})
		return
	}
	j.State = domain.JobSucceeded
	fin := time.Now().UTC()
	j.FinishedAt = &fin
	if err := m.jobEng.Save(r.Context(), j); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if err := m.audit.AppendContext(r.Context(), domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: auth.Actor(r.Context()), Action: "job.cutover", Target: j.ID, Result: "succeeded", Detail: "Cutover completed."}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, cutoverResponse{Success: res.Success, Steps: res.Steps, RetentionDeadline: res.RetentionDeadline.Format(time.RFC3339)})
}

type rollbackResponse struct {
	Success              bool     `json:"success"`
	ManualActionRequired bool     `json:"manual_action_required"`
	Steps                []string `json:"steps"`
	Warning              string   `json:"warning,omitempty"`
}

func (m *MigrationHandlers) rollbackJob(w http.ResponseWriter, r *http.Request) {
	if m.executionDisabled {
		m.writeExecutionDisabled(w)
		return
	}
	id := normalizeID(r.PathValue("id"))
	j, ok, err := m.jobEng.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok, err := m.plans.GetContext(r.Context(), j.PlanID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, tgtAdapter, err := m.resolveAdapters(srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "platform adapter unavailable", Detail: err.Error(), Code: "adapter_unavailable"})
		return
	}
	ctx := r.Context()
	res, err := m.cut.Rollback(ctx, srcAdapter, tgtAdapter, plan)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errorBody{Error: res.Warning, Code: "rollback_failed"})
		return
	}
	j.State = domain.JobRollbackPrepared
	fin := time.Now().UTC()
	j.FinishedAt = &fin
	if err := m.jobEng.Save(r.Context(), j); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if err := m.audit.AppendContext(r.Context(), domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: auth.Actor(r.Context()), Action: "job.rollback", Target: j.ID, Result: "target_isolated", Detail: "Target isolated and stopped; an authorized VMware operator must start and validate the retained source manually."}); err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "audit store unavailable", Code: "store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, rollbackResponse{Success: res.Success, ManualActionRequired: res.ManualActionRequired, Steps: res.Steps, Warning: res.Warning})
}

func (m *MigrationHandlers) getReport(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok, err := m.jobEng.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "job store unavailable", Code: "store_unavailable"})
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, _, err := m.plans.GetContext(r.Context(), j.PlanID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, errorBody{Error: "plan store unavailable", Code: "store_unavailable"})
		return
	}
	steps := make([]reports.ReportStep, len(j.Steps))
	for i, s := range j.Steps {
		steps[i] = reports.ReportStep{Name: s.Name, Status: string(s.State), StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, Message: s.Message}
	}
	report := reports.NewMigrationReport(plan, &j.Job, steps, nil, nil, nil)
	writeJSON(w, http.StatusOK, report)
}

func NewMockFactory() platform.AdapterFactory { return mockplatform.NewMockFactory() }

func (m *MigrationHandlers) resolveAdapters(source, target domain.Connection) (platform.SourceAdapter, platform.TargetAdapter, error) {
	if m.factory == nil {
		return nil, nil, fmt.Errorf("adapter factory is not configured")
	}
	sourceAdapter, err := m.factory.Source(source)
	if err != nil {
		return nil, nil, fmt.Errorf("source adapter: %w", err)
	}
	targetAdapter, err := m.factory.Target(target)
	if err != nil {
		return nil, nil, fmt.Errorf("target adapter: %w", err)
	}
	return sourceAdapter, targetAdapter, nil
}

func (m *MigrationHandlers) writeExecutionDisabled(w http.ResponseWriter) {
	writeErr(w, http.StatusNotImplemented, errorBody{
		Error:  "migration execution is disabled",
		Code:   "execution_disabled",
		Detail: "Execution, cutover, and rollback are available only in explicitly enabled lab mode; this runtime remains locked.",
	})
}

func ensureWorkDir(dir string) string {
	d := filepath.Join(dir, "jobs")
	_ = os.MkdirAll(d, 0o755)
	return d
}
