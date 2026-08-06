package job_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/drishti/hypershift/internal/converter"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/job"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
	"github.com/drishti/hypershift/internal/store"
)

type retryConverter struct {
	fail atomic.Bool
}

func (c *retryConverter) Convert(_ context.Context, inputPath, outputPath, format string) (converter.Result, error) {
	if c.fail.Load() {
		return converter.Result{}, fmt.Errorf("fixture qemu-img failure")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return converter.Result{}, err
	}
	if err := os.WriteFile(outputPath, append([]byte("converted:"), data...), 0o600); err != nil {
		return converter.Result{}, err
	}
	hash := sha256.Sum256(append([]byte("converted:"), data...))
	return converter.Result{InputPath: inputPath, OutputPath: outputPath, Format: format,
		SizeBytes: int64(len(data) + len("converted:")), SHA256: hex.EncodeToString(hash[:])}, nil
}

func TestConversionFailureAndRetryPersistThroughPostgres(t *testing.T) {
	databaseURL := os.Getenv("DRISHTI_TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("DRISHTI_TEST_DB_URL is not set")
	}
	ctx := context.Background()
	database, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	provider := mock.New()
	sourceConnection, _ := provider.Connection("conn-vmware-lab")
	targetConnection, _ := provider.Connection("conn-proxmox-lab")
	factory := mockplatform.NewMockFactory()
	source, err := factory.Source(sourceConnection)
	if err != nil {
		t.Fatal(err)
	}
	target, err := factory.Target(targetConnection)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := source.VM(ctx, "vm-web-01")
	if err != nil {
		t.Fatal(err)
	}
	node, err := target.Node(ctx, "node-pve-01")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plan := domain.Plan{ID: fmt.Sprintf("plan-item3-conversion-%d", time.Now().UnixNano()), Name: "item3 conversion persistence",
		SourceVMID: vm.ID, SourceConnID: sourceConnection.ID, TargetNodeID: node.ID,
		TargetConnID: targetConnection.ID, TargetVMName: "item3-target", CPU: vm.CPUs,
		MemoryMB: vm.MemoryMB, Firmware: vm.Firmware, DiskFormat: domain.DiskQCOW2,
		StorageMaps: []domain.StorageMap{{SourceDiskID: vm.Disks[0].ID, TargetStorageID: node.Storage[0].ID, TargetFormat: domain.DiskQCOW2}},
		Status:      domain.PlanApproved, PreflightPassed: true, Strategy: domain.MigrationStrategyCold,
		CreatedBy: "test", CreatedAt: now, UpdatedAt: now}
	if err := database.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}

	diskConverter := &retryConverter{}
	diskConverter.fail.Store(true)
	engine := job.NewEngine(job.NewPersistentState(database), factory, t.TempDir(), database.AppendAudit)
	engine.SetPlanSaver(database.PutPlan)
	engine.SetDiskConverter(diskConverter)
	if _, err := engine.ExecutePlan(ctx, plan, vm, node, sourceConnection, targetConnection); err == nil {
		t.Fatal("expected conversion failure")
	}
	persistedFailure, found, err := database.GetJob(ctx, "job-"+plan.ID)
	if err != nil || !found {
		t.Fatalf("load failed job: found=%v err=%v", found, err)
	}
	failedStep := findStep(persistedFailure, "convert_disks")
	if failedStep == nil || failedStep.State != domain.JobFailed || !strings.Contains(failedStep.Message, "fixture qemu-img failure") {
		t.Fatalf("conversion failure not persisted: %#v", failedStep)
	}

	persistedPlan, found, err := database.GetPlan(ctx, plan.ID)
	if err != nil || !found || persistedPlan.TargetVMID == nil {
		t.Fatalf("load retry plan: found=%v plan=%#v err=%v", found, persistedPlan, err)
	}
	diskConverter.fail.Store(false)
	if _, err := engine.ExecutePlan(ctx, persistedPlan, vm, node, sourceConnection, targetConnection); err != nil {
		t.Fatalf("retry: %v", err)
	}
	persistedSuccess, found, err := database.GetJob(ctx, "job-"+plan.ID)
	if err != nil || !found {
		t.Fatalf("load retried job: found=%v err=%v", found, err)
	}
	succeededStep := findStep(persistedSuccess, "convert_disks")
	if succeededStep == nil || succeededStep.State != domain.JobSucceeded || !strings.Contains(succeededStep.Message, "qcow2") {
		t.Fatalf("conversion retry evidence not persisted: %#v", succeededStep)
	}
}

func findStep(value *job.Job, name string) *job.Step {
	for i := range value.Steps {
		if value.Steps[i].Name == name {
			return &value.Steps[i]
		}
	}
	return nil
}
