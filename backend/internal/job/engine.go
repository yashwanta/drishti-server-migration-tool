// Package job implements the durable migration job state machine.
package job

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/drishti/hypershift/internal/converter"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
)

type State struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewState() *State { return &State{jobs: map[string]*Job{}} }

type Job struct {
	domain.Job
	Steps []Step   `json:"steps"`
	log   []string `json:"-"`
}

type Step struct {
	Name       string          `json:"name"`
	State      domain.JobState `json:"state"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Message    string          `json:"message"`
}

type Engine struct {
	state    *State
	factory  platform.AdapterFactory
	workDir  string
	audit    func(domain.AuditEvent)
	savePlan PlanSaver
}

// PlanSaver persists plan updates (e.g. TargetVMID assignment).
type PlanSaver func(domain.Plan)

func NewEngine(state *State, factory platform.AdapterFactory, workDir string, audit func(domain.AuditEvent)) *Engine {
	return &Engine{state: state, factory: factory, workDir: workDir, audit: audit}
}

func (e *Engine) SetPlanSaver(fn PlanSaver) { e.savePlan = fn }

func (e *Engine) Create(plan domain.Plan) *Job {
	j := &Job{
		Job: domain.Job{
			ID:             "job-" + plan.ID,
			PlanID:         plan.ID,
			State:          domain.JobPending,
			IdempotencyKey: plan.ID,
		},
		Steps: defaultSteps(),
	}
	e.state.mu.Lock()
	e.state.jobs[j.ID] = j
	e.state.mu.Unlock()
	return j
}

func defaultSteps() []Step {
	names := []string{"preflight", "source_poweroff", "export_disks", "convert_disks", "create_target_vm", "attach_disks", "isolated_boot", "validate", "cutover"}
	steps := make([]Step, len(names))
	for i, n := range names {
		steps[i] = Step{Name: n, State: domain.JobPending}
	}
	return steps
}

func (e *Engine) Get(id string) (*Job, bool) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	j, ok := e.state.jobs[id]
	return j, ok
}

func (e *Engine) List() []*Job {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	out := make([]*Job, 0, len(e.state.jobs))
	for _, j := range e.state.jobs {
		out = append(out, j)
	}
	return out
}

func (e *Engine) ExecutePlan(ctx context.Context, plan domain.Plan, vm domain.VM, node domain.TargetNode, connSource, connTarget domain.Connection) (*Job, error) {
	j, _ := e.Get("job-" + plan.ID)
	if j == nil {
		j = e.Create(plan)
	}

	srcAdapter, err := e.factory.Source(connSource)
	if err != nil {
		return j, fmt.Errorf("source adapter: %w", err)
	}
	tgtAdapter, err := e.factory.Target(connTarget)
	if err != nil {
		return j, fmt.Errorf("target adapter: %w", err)
	}

	started := time.Now().UTC()
	j.State = domain.JobRunning
	j.StartedAt = &started
	e.audit(domain.AuditEvent{ID: "evt-" + plan.ID, Timestamp: started, Actor: "system", Action: "job.start", Target: j.ID, Result: "running", Detail: "Migration execution started."})

	// Step: source power off
	e.runStep(j, "source_poweroff", func() error {
		if err := srcAdapter.PowerOff(ctx, plan.SourceVMID); err != nil {
			return err
		}
		state, _ := srcAdapter.PowerState(ctx, plan.SourceVMID)
		if state != domain.PowerOff {
			return fmt.Errorf("source VM did not reach powered-off state")
		}
		return nil
	})
	if j.State == domain.JobFailed {
		return j, fmt.Errorf("migration failed at source_poweroff")
	}

	// Reserve target VM ID
	vmid, err := tgtAdapter.ReserveVMID(ctx, node.ID)
	if err != nil {
		return j, e.fail(j, fmt.Errorf("reserve vmid: %w", err))
	}
	plan.TargetVMID = &vmid
	if e.savePlan != nil {
		e.savePlan(plan)
	}

	// Step: create target VM (initially isolated)
	e.runStep(j, "create_target_vm", func() error {
		_, err := tgtAdapter.CreateVM(ctx, platform.CreateVMSpec{
			Name: plan.TargetVMName, NodeID: plan.TargetNodeID, VMID: vmid,
			CPU: plan.CPU, MemoryMB: plan.MemoryMB, Firmware: plan.Firmware,
			IdempotencyKey: plan.ID, IsolatedBridge: "vmbr1",
		})
		return err
	})

	// Steps: export + convert + attach each disk
	jobDir := filepath.Join(e.workDir, j.ID)
	_ = os.MkdirAll(jobDir, 0o755)

	for _, sm := range plan.StorageMaps {
		exportPath := filepath.Join(jobDir, sm.SourceDiskID+".vmdk")
		convPath := filepath.Join(jobDir, sm.SourceDiskID+"."+string(sm.TargetFormat))

		e.runStep(j, "export_disks", func() error {
			_, err := srcAdapter.ExportDisk(ctx, plan.SourceVMID, sm.SourceDiskID, exportPath)
			return err
		})
		e.runStep(j, "convert_disks", func() error {
			_, err := converter.Convert(exportPath, convPath, string(sm.TargetFormat))
			return err
		})
		var diskSize int64
		for _, d := range vm.Disks {
			if d.ID == sm.SourceDiskID {
				diskSize = d.CapacityBytes
			}
		}
		e.runStep(j, "attach_disks", func() error {
			return tgtAdapter.AttachDisk(ctx, vmid, platform.AttachDiskSpec{
				Path: convPath, Format: sm.TargetFormat, SizeBytes: diskSize, Boot: true, Controller: "virtio-scsi",
			})
		})
	}

	// Step: isolated boot
	e.runStep(j, "isolated_boot", func() error {
		if err := tgtAdapter.SetNetwork(ctx, vmid, "vmbr1", 0, true); err != nil {
			return err
		}
		return tgtAdapter.StartVM(ctx, vmid)
	})

	// validate + cutover require operator approval
	j.State = domain.JobPending
	j.Steps[7].Message = "Awaiting validation and operator cutover approval."
	e.audit(domain.AuditEvent{ID: "evt-" + plan.ID + "-wait", Timestamp: time.Now().UTC(), Actor: "system", Action: "job.waiting_validation", Target: j.ID, Result: "pending", Detail: "VM migrated to isolated target; awaiting validation and cutover approval."})
	return j, nil
}

func (e *Engine) fail(j *Job, err error) error {
	ts := time.Now().UTC()
	j.State = domain.JobFailed
	j.FinishedAt = &ts
	e.audit(domain.AuditEvent{ID: "evt-" + j.ID + "-fail", Timestamp: ts, Actor: "system", Action: "job.fail", Target: j.ID, Result: "failed", Detail: err.Error()})
	return err
}

func (e *Engine) runStep(j *Job, name string, fn func() error) {
	for i := range j.Steps {
		if j.Steps[i].Name == name {
			if j.Steps[i].State == domain.JobSucceeded {
				return
			}
			st := time.Now().UTC()
			j.Steps[i].State = domain.JobRunning
			j.Steps[i].StartedAt = &st
			if err := fn(); err != nil {
				j.Steps[i].State = domain.JobFailed
				j.Steps[i].Message = err.Error()
				ft := time.Now().UTC()
				j.Steps[i].FinishedAt = &ft
				_ = e.fail(j, fmt.Errorf("%s: %w", name, err))
				return
			}
			j.Steps[i].State = domain.JobSucceeded
			ft := time.Now().UTC()
			j.Steps[i].FinishedAt = &ft
			j.Steps[i].Message = "completed"
			return
		}
	}
}

// StartExternal records a Proxmox-managed asynchronous migration task.
func (e *Engine) StartExternal(plan domain.Plan, externalID string) *Job {
	now := time.Now().UTC()
	stepName := "proxmox_remote_migration"
	detail := "Offline Proxmox remote migration started; source deletion disabled."
	if plan.Strategy == domain.MigrationStrategyPVELive {
		stepName = "proxmox_live_migration"
		detail = "Live Proxmox remote migration started in lab mode; source deletion disabled."
	}
	j := &Job{Job: domain.Job{ID: "job-" + plan.ID, PlanID: plan.ID, State: domain.JobRunning, StartedAt: &now, IdempotencyKey: plan.ID}, Steps: []Step{{Name: stepName, State: domain.JobRunning, StartedAt: &now, Message: "Proxmox task " + externalID}}}
	e.state.mu.Lock()
	e.state.jobs[j.ID] = j
	e.state.mu.Unlock()
	e.audit(domain.AuditEvent{ID: "evt-" + plan.ID + "-remote", Timestamp: now, Actor: "operator", Action: "job.remote_migration", Target: j.ID, Result: "running", Detail: detail})
	return j
}

// FinishExternal records the terminal state of an asynchronous Proxmox task.
func (e *Engine) FinishExternal(jobID string, taskErr error) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	j := e.state.jobs[jobID]
	if j == nil {
		return
	}
	now := time.Now().UTC()
	j.FinishedAt = &now
	if taskErr != nil {
		j.State = domain.JobFailed
		j.Steps[0].State = domain.JobFailed
		j.Steps[0].Message = taskErr.Error()
	} else {
		j.State = domain.JobSucceeded
		j.Steps[0].State = domain.JobSucceeded
		if j.Steps[0].Name == "proxmox_live_migration" {
			j.Steps[0].Message = "live migration completed; target is running and retained source must remain stopped"
		} else {
			j.Steps[0].Message = "completed; source retained and target remains powered off"
		}
	}
	j.Steps[0].FinishedAt = &now
}

// UpdateExternal records safe, user-facing progress for an active platform task.
func (e *Engine) UpdateExternal(jobID, message string) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()
	j := e.state.jobs[jobID]
	if j == nil || j.State != domain.JobRunning || len(j.Steps) == 0 {
		return
	}
	j.Steps[0].Message = message
}
