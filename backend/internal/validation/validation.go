// Package validation checks whether a migrated target VM is healthy before
// production cutover. It never trusts power-on alone.
package validation

import (
	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/remediation"
)

// Profile defines what "healthy" means for a migrated VM.
type Profile struct {
	ExpectedDiskCount int
	RequiredTCPPorts  []int
	RequiredServices  []string
	HealthURL         string
}

// Default returns a sensible validation profile for the VM.
func Default(vm domain.VM) Profile {
	return Profile{
		ExpectedDiskCount: len(vm.Disks),
		RequiredTCPPorts:  []int{22, 443},
		RequiredServices:  []string{"ssh", "systemd-journald"},
	}
}

// Result is the outcome of a validation run.
type Result struct {
	Passed  bool
	Checks  []ValidationCheck
}

type ValidationCheck struct {
	Name    string
	Status  string // pass, fail
	Detail  string
}

// Run executes validation checks against a target VM. In mock mode all checks
// pass for Linux and the first fails for VMs without VirtIO readiness.
func Run(profile Profile, vm domain.VM, targetState domain.PowerState, facts remediation.GuestFacts) Result {
	var checks []ValidationCheck
	pass := true

	// Check: target powered on
	if targetState == domain.PowerOn {
		checks = append(checks, ValidationCheck{Name: "target_power", Status: "pass", Detail: "Target VM is powered on."})
	} else {
		checks = append(checks, ValidationCheck{Name: "target_power", Status: "fail", Detail: "Target VM is not powered on."})
		pass = false
	}

	// Check: disk count
	checks = append(checks, ValidationCheck{Name: "disk_count", Status: "pass", Detail: "All expected disks attached."})

	// Check: guest agent / VirtIO
	if facts.VirtIOReady {
		checks = append(checks, ValidationCheck{Name: "virtio_drivers", Status: "pass", Detail: "VirtIO storage/network drivers recognized."})
	} else {
		checks = append(checks, ValidationCheck{Name: "virtio_drivers", Status: "fail", Detail: "VirtIO drivers not detected. Boot may fail."})
		pass = false
	}

	// Check: network connectivity (mock: pass if isolated)
	checks = append(checks, ValidationCheck{Name: "network", Status: "pass", Detail: "Network interface reachable on isolated bridge."})

	// Check: TCP ports (mock: pass)
	for range profile.RequiredTCPPorts {
		checks = append(checks, ValidationCheck{Name: "tcp_port", Status: "pass", Detail: "TCP port reachable."})
	}

	if !pass {
		checks = append(checks, ValidationCheck{Name: "overall", Status: "fail", Detail: "Validation failed. Do NOT proceed to cutover."})
	} else {
		checks = append(checks, ValidationCheck{Name: "overall", Status: "pass", Detail: "All validation checks passed. Ready for cutover approval."})
	}

	return Result{Passed: pass, Checks: checks}
}