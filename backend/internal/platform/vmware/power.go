package vmware

import (
	"context"
	"fmt"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/vmware/govmomi/object"
)

// PowerOff stops only the approved VM and verifies the terminal state. It is
// idempotent for an already powered-off VM.
func (s *Source) PowerOff(ctx context.Context, vmID string, approval platform.PowerOffApproval) error {
	client, err := s.client.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.Logout(context.Background()) }()

	vm, err := retrieveVM(ctx, client.Client, vmID, []string{"runtime.powerState"})
	if err != nil {
		return err
	}
	state := normalizePower(vm.Runtime.PowerState)
	if state == domain.PowerOff {
		return nil
	}
	if state != domain.PowerOn {
		return fmt.Errorf("source VM power state %q cannot be safely powered off", state)
	}
	if !approval.Approved || approval.PlanID == "" || approval.VMID != vmID {
		return fmt.Errorf("explicit source power-off approval for this plan and VM is required")
	}
	if err := s.mutations.Authorize(s.client.endpoint); err != nil {
		return fmt.Errorf("source power-off refused: %w", err)
	}

	managed := object.NewVirtualMachine(client.Client, vm.Self)
	task, err := managed.PowerOff(ctx)
	if err != nil {
		return fmt.Errorf("request source power-off: %w", err)
	}
	if err := task.Wait(ctx); err != nil {
		return fmt.Errorf("wait for source power-off: %w", err)
	}
	verified, err := retrieveVM(ctx, client.Client, vmID, []string{"runtime.powerState"})
	if err != nil {
		return fmt.Errorf("verify source power-off: %w", err)
	}
	if normalizePower(verified.Runtime.PowerState) != domain.PowerOff {
		return fmt.Errorf("source VM did not reach powered-off state")
	}
	return nil
}
