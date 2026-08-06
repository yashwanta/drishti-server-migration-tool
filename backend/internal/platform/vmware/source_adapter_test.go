package vmware

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drishti/hypershift/internal/config"
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/safety"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
)

func TestSourcePowerOffRequiresApprovalAndRejectsProduction(t *testing.T) {
	model := simulator.ESX()
	model.Machine = 1
	defer model.Remove()
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	server := model.Service.NewServer()
	defer server.Close()

	username := server.URL.User.Username()
	password, _ := server.URL.User.Password()
	endpointURL := *server.URL
	endpointURL.User = nil
	endpoint := endpointURL.String()
	workspace := localTempDir(t)
	source := NewSource("conn-simulator", endpoint, username, password, true, workspace, safety.MutationPolicy{Mode: config.ModeLab, Enabled: true})
	vmID := firstVMID(t, source)
	setSimulatorPower(t, source, vmID, true)
	vm, err := source.VM(context.Background(), vmID)
	if err != nil || len(vm.Disks) == 0 {
		t.Fatalf("read simulator VM disks: vm=%#v err=%v", vm, err)
	}
	if _, err := source.ExportDisk(context.Background(), vmID, vm.Disks[0].ID, filepath.Join(workspace, "job", "disk.vmdk")); err == nil || !strings.Contains(err.Error(), "powered off") {
		t.Fatalf("expected powered-on export refusal, got %v", err)
	}

	err = source.PowerOff(context.Background(), vmID, platform.PowerOffApproval{PlanID: "plan-1", VMID: vmID})
	if err == nil {
		t.Fatal("expected missing explicit approval to be rejected")
	}
	if state, _ := source.PowerState(context.Background(), vmID); state != domain.PowerOn {
		t.Fatalf("source state changed after rejected operation: %s", state)
	}
	if err := source.PowerOff(context.Background(), vmID, platform.PowerOffApproval{PlanID: "plan-1", VMID: vmID, Approved: true}); err != nil {
		t.Fatalf("approved lab power-off failed: %v", err)
	}
	if state, _ := source.PowerState(context.Background(), vmID); state != domain.PowerOff {
		t.Fatalf("source state = %s, want off", state)
	}

	setSimulatorPower(t, source, vmID, true)
	productionSource := NewSource("conn-simulator", endpoint, username, password, true, localTempDir(t), safety.MutationPolicy{Mode: config.ModeProduction, Enabled: true})
	err = productionSource.PowerOff(context.Background(), vmID, platform.PowerOffApproval{PlanID: "plan-2", VMID: vmID, Approved: true})
	if err == nil {
		t.Fatal("expected production mutation to be rejected")
	}
	if state, _ := source.PowerState(context.Background(), vmID); state != domain.PowerOn {
		t.Fatalf("production refusal changed source state: %s", state)
	}
}

func TestSourcePowerOffIsIdempotentWhenAlreadyOff(t *testing.T) {
	model := simulator.ESX()
	model.Machine = 1
	defer model.Remove()
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	server := model.Service.NewServer()
	defer server.Close()
	username := server.URL.User.Username()
	password, _ := server.URL.User.Password()
	u := *server.URL
	u.User = nil
	source := NewSource("conn-simulator", u.String(), username, password, true, localTempDir(t), safety.MutationPolicy{Mode: config.ModeMock})
	vmID := firstVMID(t, source)
	setSimulatorPower(t, source, vmID, false)
	if err := source.PowerOff(context.Background(), vmID, platform.PowerOffApproval{}); err != nil {
		t.Fatalf("already-off VM should require no mutation authorization: %v", err)
	}
}

func firstVMID(t *testing.T, source *Source) string {
	t.Helper()
	inv, err := source.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, dc := range inv.Datacenters {
		for _, cluster := range dc.Clusters {
			for _, host := range cluster.Hosts {
				if len(host.VMs) > 0 {
					return host.VMs[0].ID
				}
			}
		}
	}
	t.Fatal("simulator has no VM")
	return ""
}

func setSimulatorPower(t *testing.T, source *Source, vmID string, on bool) {
	t.Helper()
	ctx := context.Background()
	state, err := source.PowerState(ctx, vmID)
	if err != nil {
		t.Fatal(err)
	}
	if (on && state == domain.PowerOn) || (!on && state == domain.PowerOff) {
		return
	}
	client, err := source.client.connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Logout(ctx)
	vm := object.NewVirtualMachine(client.Client, types.ManagedObjectReference{Type: "VirtualMachine", Value: vmID})
	var task *object.Task
	if on {
		task, err = vm.PowerOn(ctx)
	} else {
		task, err = vm.PowerOff(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := task.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
