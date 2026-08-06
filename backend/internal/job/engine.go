// Package job implements the durable migration job state machine.
package job

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/auth"
	"github.com/drishti/hypershift/internal/converter"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
)

// Backend is implemented by the PostgreSQL repository. Implementations must
// atomically persist a job and its ordered steps.
type Backend interface {
	SaveJob(context.Context, *Job) error
	GetJob(context.Context, string) (*Job, bool, error)
	ListJobs(context.Context) ([]*Job, error)
}

// State is the concurrency-safe in-memory backend used only in mock mode.
type State struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	backend Backend
}

func NewState() *State { return &State{jobs: map[string]*Job{}} }

func NewPersistentState(backend Backend) *State { return &State{backend: backend} }

type Job struct {
	domain.Job
	Steps []Step   `json:"steps"`
	log   []string `json:"-"`
}

type Step struct {
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name"`
	Actor      string          `json:"actor"`
	State      domain.JobState `json:"state"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	ExternalID string          `json:"external_id,omitempty"`
	Message    string          `json:"message"`
}

func cloneJob(value *Job) *Job {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Steps = append([]Step(nil), value.Steps...)
	clone.log = append([]string(nil), value.log...)
	return &clone
}

func (s *State) save(ctx context.Context, value *Job) error {
	if s.backend != nil {
		return s.backend.SaveJob(ctx, cloneJob(value))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[value.ID] = cloneJob(value)
	return nil
}

func (s *State) get(ctx context.Context, id string) (*Job, bool, error) {
	if s.backend != nil {
		return s.backend.GetJob(ctx, id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.jobs[id]
	return cloneJob(value), ok, nil
}

func (s *State) list(ctx context.Context) ([]*Job, error) {
	if s.backend != nil {
		return s.backend.ListJobs(ctx)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, value := range s.jobs {
		out = append(out, cloneJob(value))
	}
	return out, nil
}

type Engine struct {
	mu        sync.Mutex
	state     *State
	factory   platform.AdapterFactory
	workDir   string
	audit     AuditWriter
	savePlan  PlanSaver
	converter converter.DiskConverter
}

type AuditWriter func(context.Context, domain.AuditEvent) error

// PlanSaver persists plan updates (for example, a TargetVMID assignment).
type PlanSaver func(context.Context, domain.Plan) error

func NewEngine(state *State, factory platform.AdapterFactory, workDir string, audit AuditWriter) *Engine {
	return &Engine{state: state, factory: factory, workDir: workDir, audit: audit, converter: converter.NewMock()}
}

func (e *Engine) SetPlanSaver(fn PlanSaver) { e.savePlan = fn }

func (e *Engine) SetDiskConverter(diskConverter converter.DiskConverter) {
	if diskConverter != nil {
		e.converter = diskConverter
	}
}

func (e *Engine) create(ctx context.Context, plan domain.Plan) (*Job, error) {
	actor := requestActor(ctx)
	value := &Job{
		Job:   domain.Job{ID: "job-" + plan.ID, PlanID: plan.ID, Actor: actor, State: domain.JobPending, IdempotencyKey: plan.ID},
		Steps: defaultSteps(actor),
	}
	if err := e.state.save(ctx, value); err != nil {
		return nil, fmt.Errorf("persist new job: %w", err)
	}
	return cloneJob(value), nil
}

func (e *Engine) Create(ctx context.Context, plan domain.Plan) (*Job, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.create(ctx, plan)
}

func requestActor(ctx context.Context) string {
	actor := auth.Actor(ctx)
	if actor == "unauthenticated" {
		return "system"
	}
	return actor
}

func defaultSteps(actor string) []Step {
	names := []string{"preflight", "source_poweroff", "export_disks", "convert_disks", "create_target_vm", "attach_disks", "isolated_boot", "validate", "cutover"}
	steps := make([]Step, len(names))
	for i, name := range names {
		steps[i] = Step{Name: name, Actor: actor, State: domain.JobPending}
	}
	return steps
}

func (e *Engine) Get(ctx context.Context, id string) (*Job, bool, error) {
	return e.state.get(ctx, id)
}

func (e *Engine) List(ctx context.Context) ([]*Job, error) { return e.state.list(ctx) }

func (e *Engine) Save(ctx context.Context, value *Job) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.save(ctx, value)
}

func (e *Engine) writeAudit(ctx context.Context, event domain.AuditEvent) error {
	if e.audit == nil {
		return nil
	}
	return e.audit(ctx, event)
}

func (e *Engine) ExecutePlan(ctx context.Context, plan domain.Plan, vm domain.VM, node domain.TargetNode, connSource, connTarget domain.Connection) (*Job, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	value, ok, err := e.state.get(ctx, "job-"+plan.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		value, err = e.create(ctx, plan)
		if err != nil {
			return nil, err
		}
	}

	srcAdapter, err := e.factory.Source(connSource)
	if err != nil {
		return value, fmt.Errorf("source adapter: %w", err)
	}
	tgtAdapter, err := e.factory.Target(connTarget)
	if err != nil {
		return value, fmt.Errorf("target adapter: %w", err)
	}

	started := time.Now().UTC()
	value.State = domain.JobRunning
	value.StartedAt = &started
	value.FinishedAt = nil
	if err := e.state.save(ctx, value); err != nil {
		return value, err
	}
	if err := e.writeAudit(ctx, domain.AuditEvent{ID: "evt-" + plan.ID, Timestamp: started, Actor: value.Actor, Action: "job.start", Target: value.ID, Result: "running", Detail: "Migration execution started."}); err != nil {
		return value, fmt.Errorf("persist job start audit: %w", err)
	}

	if err := e.runStep(ctx, value, "source_poweroff", func() error {
		state, err := srcAdapter.PowerState(ctx, plan.SourceVMID)
		if err != nil {
			return fmt.Errorf("read source power state: %w", err)
		}
		if state == domain.PowerOn && !plan.SourcePowerOffApproved {
			return fmt.Errorf("running source requires explicit source power-off approval")
		}
		if state != domain.PowerOn && state != domain.PowerOff {
			return fmt.Errorf("source power state %q is not safe for cold migration", state)
		}
		if err := srcAdapter.PowerOff(ctx, plan.SourceVMID, platform.PowerOffApproval{PlanID: plan.ID, VMID: plan.SourceVMID, Approved: plan.SourcePowerOffApproved}); err != nil {
			return err
		}
		state, err = srcAdapter.PowerState(ctx, plan.SourceVMID)
		if err != nil {
			return fmt.Errorf("verify source power state: %w", err)
		}
		if state != domain.PowerOff {
			return fmt.Errorf("source VM did not reach powered-off state")
		}
		return nil
	}); err != nil {
		return value, err
	}

	vmid := 0
	if plan.TargetVMID != nil {
		vmid = *plan.TargetVMID
	} else {
		vmid, err = tgtAdapter.ReserveVMID(ctx, node.ID)
		if err != nil {
			return value, e.fail(ctx, value, fmt.Errorf("reserve vmid: %w", err))
		}
		plan.TargetVMID = &vmid
		if e.savePlan != nil {
			if err := e.savePlan(ctx, plan); err != nil {
				return value, e.fail(ctx, value, fmt.Errorf("persist target VMID: %w", err))
			}
		}
	}

	if err := e.runStep(ctx, value, "create_target_vm", func() error {
		storageID := ""
		if len(plan.StorageMaps) > 0 {
			storageID = plan.StorageMaps[0].TargetStorageID
		}
		_, err := tgtAdapter.CreateVM(ctx, platform.CreateVMSpec{Name: plan.TargetVMName, NodeID: plan.TargetNodeID, VMID: vmid,
			CPU: plan.CPU, MemoryMB: plan.MemoryMB, Firmware: plan.Firmware, IdempotencyKey: plan.ID,
			IsolatedBridge: "vmbr1", StorageID: storageID})
		return err
	}); err != nil {
		return value, err
	}

	jobDir := filepath.Join(e.workDir, value.ID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return value, e.fail(ctx, value, fmt.Errorf("create job workspace: %w", err))
	}
	type diskWork struct {
		index         int
		storageMap    domain.StorageMap
		exportPath    string
		convertedPath string
		sizeBytes     int64
	}
	disks := make([]diskWork, 0, len(plan.StorageMaps))
	for diskIndex, storageMap := range plan.StorageMaps {
		var diskSize int64
		for _, disk := range vm.Disks {
			if disk.ID == storageMap.SourceDiskID {
				diskSize = disk.CapacityBytes
			}
		}
		disks = append(disks, diskWork{index: diskIndex, storageMap: storageMap, sizeBytes: diskSize,
			exportPath:    filepath.Join(jobDir, storageMap.SourceDiskID+".vmdk"),
			convertedPath: filepath.Join(jobDir, storageMap.SourceDiskID+"."+string(storageMap.TargetFormat))})
	}
	if err := e.runStep(ctx, value, "export_disks", func() error {
		for _, disk := range disks {
			if _, err := srcAdapter.ExportDisk(ctx, plan.SourceVMID, disk.storageMap.SourceDiskID, disk.exportPath); err != nil {
				return fmt.Errorf("disk %s: %w", disk.storageMap.SourceDiskID, err)
			}
		}
		return nil
	}); err != nil {
		return value, err
	}
	var conversionEvidence []string
	if err := e.runStep(ctx, value, "convert_disks", func() error {
		for _, disk := range disks {
			result, err := e.converter.Convert(ctx, disk.exportPath, disk.convertedPath, string(disk.storageMap.TargetFormat))
			if err != nil {
				return fmt.Errorf("disk %s: %w", disk.storageMap.SourceDiskID, err)
			}
			conversionEvidence = append(conversionEvidence, fmt.Sprintf("%s:%s:%d:%s:reused=%t", disk.storageMap.SourceDiskID, result.Format, result.SizeBytes, result.SHA256, result.Reused))
		}
		return nil
	}); err != nil {
		return value, err
	}
	if len(conversionEvidence) > 0 {
		e.setStepMessage(value, "convert_disks", "completed; "+strings.Join(conversionEvidence, ","))
		if err := e.state.save(ctx, value); err != nil {
			return value, fmt.Errorf("persist conversion evidence: %w", err)
		}
	}
	if err := e.runStep(ctx, value, "attach_disks", func() error {
		for _, disk := range disks {
			if err := tgtAdapter.AttachDisk(ctx, vmid, platform.AttachDiskSpec{Path: disk.convertedPath,
				Format: disk.storageMap.TargetFormat, SizeBytes: disk.sizeBytes, Boot: disk.index == 0,
				Controller: "virtio-scsi", StorageID: disk.storageMap.TargetStorageID, DeviceIndex: disk.index,
				IdempotencyKey: plan.ID + ":" + disk.storageMap.SourceDiskID}); err != nil {
				return fmt.Errorf("disk %s: %w", disk.storageMap.SourceDiskID, err)
			}
		}
		return nil
	}); err != nil {
		return value, err
	}

	if err := e.runStep(ctx, value, "isolated_boot", func() error {
		if err := tgtAdapter.SetNetwork(ctx, vmid, "vmbr1", 0, true); err != nil {
			return err
		}
		return tgtAdapter.StartVM(ctx, vmid)
	}); err != nil {
		return value, err
	}

	value.State = domain.JobPending
	value.Steps[7].Message = "Awaiting validation and operator cutover approval."
	if err := e.state.save(ctx, value); err != nil {
		return value, err
	}
	if err := e.writeAudit(ctx, domain.AuditEvent{ID: "evt-" + plan.ID + "-wait", Timestamp: time.Now().UTC(), Actor: "system", Action: "job.waiting_validation", Target: value.ID, Result: "pending", Detail: "VM migrated to isolated target; awaiting validation and cutover approval."}); err != nil {
		return value, err
	}
	return cloneJob(value), nil
}

func (e *Engine) setStepMessage(value *Job, name, message string) {
	for i := range value.Steps {
		if value.Steps[i].Name == name {
			value.Steps[i].Message = message
			return
		}
	}
}

func (e *Engine) fail(ctx context.Context, value *Job, cause error) error {
	timestamp := time.Now().UTC()
	value.State = domain.JobFailed
	value.FinishedAt = &timestamp
	if err := e.state.save(ctx, value); err != nil {
		return fmt.Errorf("%v; persist failed job: %w", cause, err)
	}
	if err := e.writeAudit(ctx, domain.AuditEvent{ID: "evt-" + value.ID + "-fail", Timestamp: timestamp, Actor: "system", Action: "job.fail", Target: value.ID, Result: "failed", Detail: cause.Error()}); err != nil {
		return fmt.Errorf("%v; persist failure audit: %w", cause, err)
	}
	return cause
}

func (e *Engine) runStep(ctx context.Context, value *Job, name string, fn func() error) error {
	for i := range value.Steps {
		if value.Steps[i].Name != name {
			continue
		}
		if value.Steps[i].State == domain.JobSucceeded {
			return nil
		}
		started := time.Now().UTC()
		value.Steps[i].State = domain.JobRunning
		value.Steps[i].StartedAt = &started
		value.Steps[i].FinishedAt = nil
		value.Steps[i].Message = "running"
		if err := e.state.save(ctx, value); err != nil {
			return fmt.Errorf("persist running step %q: %w", name, err)
		}
		if err := fn(); err != nil {
			finished := time.Now().UTC()
			value.Steps[i].State = domain.JobFailed
			value.Steps[i].Message = err.Error()
			value.Steps[i].FinishedAt = &finished
			return e.fail(ctx, value, fmt.Errorf("%s: %w", name, err))
		}
		finished := time.Now().UTC()
		value.Steps[i].State = domain.JobSucceeded
		value.Steps[i].FinishedAt = &finished
		value.Steps[i].Message = "completed"
		if err := e.state.save(ctx, value); err != nil {
			return fmt.Errorf("persist completed step %q: %w", name, err)
		}
		return nil
	}
	return fmt.Errorf("job step %q not found", name)
}

// StartExternal records a Proxmox-managed asynchronous migration task.
func (e *Engine) StartExternal(ctx context.Context, plan domain.Plan, externalID string) (*Job, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UTC()
	stepName := "proxmox_remote_migration"
	detail := "Offline Proxmox remote migration started; source deletion disabled."
	if plan.Strategy == domain.MigrationStrategyPVELive {
		stepName = "proxmox_live_migration"
		detail = "Live Proxmox remote migration started in lab mode; source deletion disabled."
	}
	actor := requestActor(ctx)
	value := &Job{Job: domain.Job{ID: "job-" + plan.ID, PlanID: plan.ID, Actor: actor, State: domain.JobRunning, StartedAt: &now, IdempotencyKey: plan.ID},
		Steps: []Step{{Name: stepName, Actor: actor, State: domain.JobRunning, StartedAt: &now, ExternalID: externalID, Message: "Proxmox task " + externalID}}}
	if err := e.state.save(ctx, value); err != nil {
		return nil, err
	}
	if err := e.writeAudit(ctx, domain.AuditEvent{ID: "evt-" + plan.ID + "-remote", Timestamp: now, Actor: value.Actor, Action: "job.remote_migration", Target: value.ID, Result: "running", Detail: detail}); err != nil {
		return nil, err
	}
	return cloneJob(value), nil
}

// FinishExternal records the terminal state of an asynchronous Proxmox task.
func (e *Engine) FinishExternal(ctx context.Context, jobID string, taskErr error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok, err := e.state.get(ctx, jobID)
	if err != nil || !ok {
		return err
	}
	now := time.Now().UTC()
	value.FinishedAt = &now
	if taskErr != nil {
		value.State = domain.JobFailed
		value.Steps[0].State = domain.JobFailed
		value.Steps[0].Message = taskErr.Error()
	} else {
		value.State = domain.JobSucceeded
		value.Steps[0].State = domain.JobSucceeded
		if value.Steps[0].Name == "proxmox_live_migration" {
			value.Steps[0].Message = "live migration completed; target is running and retained source must remain stopped"
		} else {
			value.Steps[0].Message = "completed; source retained and target remains powered off"
		}
	}
	value.Steps[0].FinishedAt = &now
	return e.state.save(ctx, value)
}

// UpdateExternal records safe, user-facing progress for an active platform task.
func (e *Engine) UpdateExternal(ctx context.Context, jobID, message string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok, err := e.state.get(ctx, jobID)
	if err != nil || !ok || value.State != domain.JobRunning || len(value.Steps) == 0 {
		return err
	}
	value.Steps[0].Message = message
	return e.state.save(ctx, value)
}
