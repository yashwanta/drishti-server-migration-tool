package cutover

import (
	"context"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/mock"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/platform/mockplatform"
)

func TestRollbackStopsTargetButNeverPowersOnSource(t *testing.T) {
	provider := mock.New()
	sourceConn, _ := provider.Connection("conn-vmware-lab")
	targetConn, _ := provider.Connection("conn-proxmox-lab")
	factory := mockplatform.NewMockFactory()
	source, _ := factory.Source(sourceConn)
	target, _ := factory.Target(targetConn)
	vmid := 900
	if _, err := target.CreateVM(context.Background(), platform.CreateVMSpec{VMID: vmid, NodeID: "node-pve-01", Name: "rollback-fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := target.StartVM(context.Background(), vmid); err != nil {
		t.Fatal(err)
	}
	plan := domain.Plan{ID: "plan-rollback", SourceVMID: "vm-web-01", TargetVMID: &vmid}
	result, err := New().Rollback(context.Background(), source, target, plan)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !result.Success || !result.ManualActionRequired || !strings.Contains(result.Warning, "Manual action required") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if state, _ := source.PowerState(context.Background(), plan.SourceVMID); state != domain.PowerOff {
		t.Fatalf("source was powered on automatically: %s", state)
	}
	if state, _ := target.VMState(context.Background(), vmid); state != domain.PowerOff {
		t.Fatalf("target state = %s, want off", state)
	}
}
