package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
)

func TestPostgresPersistsPlansJobsStepsAuditAndSerializesApproval(t *testing.T) {
	databaseURL := os.Getenv("DRISHTI_TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("DRISHTI_TEST_DB_URL is not set")
	}
	ctx := context.Background()
	database, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.db.ExecContext(ctx, `TRUNCATE job_steps, jobs, audit_events, plans RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	plan := domain.Plan{
		ID: "plan-persistence", Name: "persistence", SourceVMID: "vm-1", SourceConnID: "source-1",
		TargetNodeID: "node-1", TargetConnID: "target-1", TargetVMName: "target-vm",
		CPU: 4, MemoryMB: 8192, Firmware: domain.FirmwareUEFI, DiskFormat: domain.DiskQCOW2,
		StorageMaps: []domain.StorageMap{{SourceDiskID: "disk-1", TargetStorageID: "local-lvm", TargetFormat: domain.DiskRaw}},
		NetworkMaps: []domain.NetworkMap{{SourceNICID: "nic-1", TargetBridge: "vmbr-lab", VLANID: 42}},
		Status:      domain.PlanPreflight, PreflightPassed: true, Strategy: domain.MigrationStrategyCold,
		CreatedBy: "test", CreatedAt: now, UpdatedAt: now,
	}
	if err := database.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			event := domain.AuditEvent{ID: fmt.Sprintf("evt-approval-%02d", i), Timestamp: now.Add(time.Duration(i) * time.Microsecond), Actor: "operator", Action: "plan.approve", Target: plan.ID, Result: "approved", Detail: "concurrent approval test"}
			if _, found, err := database.ApprovePlan(ctx, plan.ID, true, event); err == nil && found {
				successes.Add(1)
			}
		}(i)
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful transactional approvals = %d, want 1", successes.Load())
	}

	persistedPlan, found, err := database.GetPlan(ctx, plan.ID)
	if err != nil || !found {
		t.Fatalf("GetPlan: found=%v err=%v", found, err)
	}
	if persistedPlan.Status != domain.PlanApproved || !persistedPlan.SourcePowerOffApproved || len(persistedPlan.StorageMaps) != 1 || len(persistedPlan.NetworkMaps) != 1 {
		t.Fatalf("incomplete persisted plan: %#v", persistedPlan)
	}
	events, err := database.ListAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "plan.approve" {
		t.Fatalf("approval audit events = %#v", events)
	}

	started := now.Add(time.Second)
	finished := started.Add(time.Second)
	value := &job.Job{
		Job: domain.Job{ID: "job-persistence", PlanID: plan.ID, State: domain.JobSucceeded, IdempotencyKey: plan.ID, StartedAt: &started, FinishedAt: &finished},
		Steps: []job.Step{
			{Name: "source_poweroff", State: domain.JobSucceeded, StartedAt: &started, FinishedAt: &finished, Message: "completed"},
			{Name: "create_target_vm", State: domain.JobRunning, ExternalID: "UPID:fixture", StartedAt: &finished, Message: "running"},
		},
	}
	if err := database.SaveJob(ctx, value); err != nil {
		t.Fatal(err)
	}

	secondConnection, err := OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConnection.Close()
	reloaded, found, err := secondConnection.GetJob(ctx, value.ID)
	if err != nil || !found {
		t.Fatalf("GetJob after reopen: found=%v err=%v", found, err)
	}
	if reloaded.State != domain.JobSucceeded || len(reloaded.Steps) != 2 || reloaded.Steps[1].ExternalID != "UPID:fixture" {
		t.Fatalf("incomplete persisted job: %#v", reloaded)
	}
}
