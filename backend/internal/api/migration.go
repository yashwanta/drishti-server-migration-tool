package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	real              *proxmox.Probe
}

func NewMigrationHandlers(h *Handlers, jobEng *job.Engine, factory platform.AdapterFactory, workDir string) *MigrationHandlers {
	return &MigrationHandlers{Handlers: h, jobEng: jobEng, pre: preflight.New(), cut: cutover.New(), factory: factory, workDir: workDir}
}

// DisableExecution prevents a real-mode deployment from invoking the mock job
// engine. Real execution is enabled only after a production adapter exists.
func (m *MigrationHandlers) DisableExecution() { m.executionDisabled = true }

// EnableLabRemoteMigration installs the real Proxmox migration path. Callers
// must only invoke this after the explicit lab mutation interlock is enabled.
func (m *MigrationHandlers) EnableLabRemoteMigration(real *proxmox.Probe, enableExecution bool) {
	m.real = real
	m.executionDisabled = !enableExecution
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
	plan, ok := m.plans.Get(id)
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
		m.plans.Put(plan)
		writeJSON(w, http.StatusOK, preflightResponse{PlanID: plan.ID, Pass: pass, Checks: checks, Blocked: blocked})
		return
	}
	srcAdapter, _ := m.factory.Source(srcConn)
	tgtAdapter, _ := m.factory.Target(tgtConn)
	ctx := context.Background()
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
		m.plans.Put(plan)
	}
	result := m.pre.Run(plan, vm, node)
	plan.Status = domain.PlanPreflight
	m.plans.Put(plan)
	m.audit.Append(domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: "operator", Action: "plan.preflight", Target: plan.ID, Result: boolStr(result.Pass), Detail: "Preflight checks ran."})
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
	plan, ok := m.plans.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	plan.Status = domain.PlanApproved
	plan.UpdatedAt = time.Now().UTC()
	m.plans.Put(plan)
	m.audit.Append(domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: "operator", Action: "plan.approve", Target: plan.ID, Result: "approved", Detail: "Migration plan approved."})
	writeJSON(w, http.StatusOK, plan)
}

func (m *MigrationHandlers) executeMigration(w http.ResponseWriter, r *http.Request) {
	if m.executionDisabled {
		writeErr(w, http.StatusNotImplemented, errorBody{
			Error:  "real migration execution is not implemented",
			Code:   "not_implemented",
			Detail: "Connection testing is real in lab mode, but inventory, transfer, cutover, and rollback remain disabled.",
		})
		return
	}
	id := normalizeID(r.PathValue("id"))
	plan, ok := m.plans.Get(id)
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
		m.plans.Put(plan)
		j := m.jobEng.StartExternal(plan, result.UPID)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Hour)
			defer cancel()
			err := m.real.WaitRemoteMigration(ctx, srcConn, result, func(progress proxmox.RemoteMigrationProgress) {
				m.jobEng.UpdateExternal(j.ID, progress.Message)
			})
			if err == nil {
				err = m.real.VerifyRemoteMigration(ctx, srcConn, tgtConn, result)
			}
			m.jobEng.FinishExternal(j.ID, err)
		}()
		writeJSON(w, http.StatusAccepted, j)
		return
	}
	srcAdapter, _ := m.factory.Source(srcConn)
	tgtAdapter, _ := m.factory.Target(tgtConn)
	ctx := context.Background()
	vm, _ := srcAdapter.VM(ctx, plan.SourceVMID)
	node, _ := tgtAdapter.Node(ctx, plan.TargetNodeID)
	if len(plan.StorageMaps) == 0 {
		plan.StorageMaps = autoStorageMaps(vm, node)
		m.plans.Put(plan)
	}
	j, err := m.jobEng.ExecutePlan(ctx, plan, vm, node, srcConn, tgtConn)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errorBody{Error: err.Error(), Code: "execution_failed"})
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (m *MigrationHandlers) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs := m.jobEng.List()
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (m *MigrationHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok := m.jobEng.Get(id)
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
	j, ok := m.jobEng.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok := m.plans.Get(j.PlanID)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, _ := m.factory.Source(srcConn)
	tgtAdapter, _ := m.factory.Target(tgtConn)
	ctx := context.Background()
	vm, _ := srcAdapter.VM(ctx, plan.SourceVMID)
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
	m.audit.Append(domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: "operator", Action: "job.validate", Target: j.ID, Result: boolStr(result.Passed), Detail: "Validation ran."})
	writeJSON(w, http.StatusOK, validateResponse{Passed: result.Passed, Checks: checks})
}

type cutoverResponse struct {
	Success           bool     `json:"success"`
	Steps             []string `json:"steps"`
	Warning           string   `json:"warning,omitempty"`
	RetentionDeadline string   `json:"retention_deadline,omitempty"`
}

func (m *MigrationHandlers) cutoverJob(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok := m.jobEng.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok := m.plans.Get(j.PlanID)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, _ := m.factory.Source(srcConn)
	tgtAdapter, _ := m.factory.Target(tgtConn)
	ctx := context.Background()
	vm, _ := srcAdapter.VM(ctx, plan.SourceVMID)
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
	m.audit.Append(domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: "operator", Action: "job.cutover", Target: j.ID, Result: "succeeded", Detail: "Cutover completed."})
	writeJSON(w, http.StatusOK, cutoverResponse{Success: res.Success, Steps: res.Steps, RetentionDeadline: res.RetentionDeadline.Format(time.RFC3339)})
}

type rollbackResponse struct {
	Success bool     `json:"success"`
	Steps   []string `json:"steps"`
	Warning string   `json:"warning,omitempty"`
}

func (m *MigrationHandlers) rollbackJob(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok := m.jobEng.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, ok := m.plans.Get(j.PlanID)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "plan not found", Code: "not_found"})
		return
	}
	srcConn, tgtConn, _ := m.resolveSourceTarget(plan)
	srcAdapter, _ := m.factory.Source(srcConn)
	tgtAdapter, _ := m.factory.Target(tgtConn)
	ctx := context.Background()
	res, err := m.cut.Rollback(ctx, srcAdapter, tgtAdapter, plan)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, errorBody{Error: res.Warning, Code: "rollback_failed"})
		return
	}
	j.State = domain.JobRolledBack
	fin := time.Now().UTC()
	j.FinishedAt = &fin
	m.audit.Append(domain.AuditEvent{ID: "evt-" + newID(), Timestamp: nowUTC(), Actor: "operator", Action: "job.rollback", Target: j.ID, Result: "rolled_back", Detail: "Rollback completed; source restored."})
	writeJSON(w, http.StatusOK, rollbackResponse{Success: res.Success, Steps: res.Steps, Warning: res.Warning})
}

func (m *MigrationHandlers) getReport(w http.ResponseWriter, r *http.Request) {
	id := normalizeID(r.PathValue("id"))
	j, ok := m.jobEng.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errorBody{Error: "job not found", Code: "not_found"})
		return
	}
	plan, _ := m.plans.Get(j.PlanID)
	steps := make([]reports.ReportStep, len(j.Steps))
	for i, s := range j.Steps {
		steps[i] = reports.ReportStep{Name: s.Name, Status: string(s.State), StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, Message: s.Message}
	}
	report := reports.NewMigrationReport(plan, &j.Job, steps, nil, nil, nil)
	writeJSON(w, http.StatusOK, report)
}

func NewMockFactory() platform.AdapterFactory { return mockplatform.NewMockFactory() }
func ensureWorkDir(dir string) string {
	d := filepath.Join(dir, "jobs")
	_ = os.MkdirAll(d, 0o755)
	return d
}
