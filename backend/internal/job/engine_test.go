package job

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
)

func TestExecutePlanRefusesUnapprovedRunningSource(t *testing.T) {
	provider := mock.New()
	sourceConn, _ := provider.Connection("conn-vmware-lab")
	targetConn, _ := provider.Connection("conn-proxmox-lab")
	factory := mockplatform.NewMockFactory()
	source, err := factory.Source(sourceConn)
	if err != nil {
		t.Fatal(err)
	}
	target, err := factory.Target(targetConn)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := source.VM(context.Background(), "vm-rdm-01")
	if err != nil {
		t.Fatal(err)
	}
	node, err := target.Node(context.Background(), "node-pve-01")
	if err != nil {
		t.Fatal(err)
	}
	plan := domain.Plan{
		ID: "plan-no-power-approval", SourceVMID: vm.ID,
		SourceConnID: sourceConn.ID, TargetConnID: targetConn.ID,
		TargetNodeID: node.ID, TargetVMName: "should-not-exist",
		CPU: 1, MemoryMB: 512, Firmware: domain.FirmwareBIOS,
	}
	engine := NewEngine(NewState(), factory, t.TempDir(), func(context.Context, domain.AuditEvent) error { return nil })
	job, err := engine.ExecutePlan(context.Background(), plan, vm, node, sourceConn, targetConn)
	if err == nil {
		t.Fatal("expected execution to refuse an unapproved source power-off")
	}
	if job.State != domain.JobFailed || job.Steps[1].State != domain.JobFailed {
		t.Fatalf("job did not fail at source_poweroff: %#v", job)
	}
	state, err := source.PowerState(context.Background(), vm.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state != domain.PowerOn {
		t.Fatalf("source state changed despite refusal: %s", state)
	}
}

func TestConcurrentExternalStateTransitions(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(NewState(), mockplatform.NewMockFactory(), t.TempDir(), func(context.Context, domain.AuditEvent) error { return nil })
	plan := domain.Plan{ID: "plan-concurrent", Strategy: domain.MigrationStrategyCold}
	value, err := engine.StartExternal(ctx, plan, "UPID:fixture")
	if err != nil {
		t.Fatal(err)
	}

	var workers sync.WaitGroup
	for i := 0; i < 64; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			_ = engine.UpdateExternal(ctx, value.ID, fmt.Sprintf("progress-%d", i))
			_, _, _ = engine.Get(ctx, value.ID)
			_, _ = engine.List(ctx)
		}(i)
	}
	workers.Wait()
	if err := engine.FinishExternal(ctx, value.ID, nil); err != nil {
		t.Fatal(err)
	}

	finished, ok, err := engine.Get(ctx, value.ID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if finished.State != domain.JobSucceeded || len(finished.Steps) != 1 || finished.Steps[0].State != domain.JobSucceeded {
		t.Fatalf("unexpected terminal job: %#v", finished)
	}
}
