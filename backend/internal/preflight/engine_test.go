package preflight

import (
	"testing"

	"github.com/drishti/hypershift/internal/domain"
)

func TestPowerStatePreflight(t *testing.T) {
	if got := checkPowerState(domain.VM{Name: "running", PowerState: domain.PowerOn}); got.Status != domain.CheckWarn {
		t.Fatalf("running VM status = %s, want warning", got.Status)
	}
	if got := checkPowerState(domain.VM{Name: "off", PowerState: domain.PowerOff}); got.Status != domain.CheckPass {
		t.Fatalf("powered-off VM status = %s, want pass", got.Status)
	}
	if got := checkPowerState(domain.VM{Name: "suspended", PowerState: domain.PowerSuspended}); got.Status != domain.CheckFail {
		t.Fatalf("suspended VM status = %s, want fail", got.Status)
	}
}

func TestSnapshotPreflightBlocksExport(t *testing.T) {
	got := checkSnapshots(domain.VM{Snapshots: []domain.Snapshot{{ID: "snapshot-1"}}})
	if got.Status != domain.CheckFail {
		t.Fatalf("snapshot status = %s, want fail", got.Status)
	}
}
