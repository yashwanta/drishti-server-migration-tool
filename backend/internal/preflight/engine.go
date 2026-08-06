// Package preflight runs compatibility and safety checks against a migration
// plan before it can be approved or executed. It never mutates anything.
package preflight

import (
	"fmt"

	"github.com/drishti/hypershift/internal/domain"
)

// Result is the outcome of running all preflight checks for a plan.
type Result struct {
	PlanID  string
	Pass    bool
	Checks  []domain.PreflightCheck
	Blocked []string
}

// Engine runs the full preflight check set.
type Engine struct {
	maxDiskGB int64
}

func New() *Engine { return &Engine{maxDiskGB: 2048} }

// Run executes every check and returns a combined result. The VM and target
// node are passed as resolved domain objects.
func (e *Engine) Run(plan domain.Plan, vm domain.VM, node domain.TargetNode) Result {
	var checks []domain.PreflightCheck
	var blocked []string

	// Check 1: a running source is allowed to reach approval only as a warning;
	// execution still requires a separate, VM-bound power-off approval.
	power := checkPowerState(vm)
	checks = append(checks, power)
	if power.Status == domain.CheckFail {
		blocked = append(blocked, power.Message)
	}

	// Check 2: target capacity.
	cap := checkCapacity(vm, node)
	checks = append(checks, cap)
	if cap.Status == domain.CheckFail {
		blocked = append(blocked, cap.Message)
	}

	// Check 3: storage availability for each mapped disk.
	for _, sm := range plan.StorageMaps {
		storage := checkStorageAvailable(sm, vm, node)
		checks = append(checks, storage)
		if storage.Status == domain.CheckFail {
			blocked = append(blocked, storage.Message)
		}
	}

	// Check 4: unsupported features (RDM, passthrough, etc).
	unsup := checkUnsupportedFeatures(vm)
	checks = append(checks, unsup)
	if unsup.Status == domain.CheckFail {
		blocked = append(blocked, unsup.Message)
	}

	// Check 5: firmware compatibility.
	fw := checkFirmware(vm, plan)
	checks = append(checks, fw)

	// Check 6: VMware tools status.
	tools := checkToolsStatus(vm)
	checks = append(checks, tools)

	// Check 7: snapshot presence. Export refuses delta chains and DRISHTI never
	// consolidates VMware snapshots automatically.
	snap := checkSnapshots(vm)
	checks = append(checks, snap)
	if snap.Status == domain.CheckFail {
		blocked = append(blocked, snap.Message)
	}

	// Check 8: target node online.
	online := checkNodeOnline(node)
	checks = append(checks, online)
	if online.Status == domain.CheckFail {
		blocked = append(blocked, online.Message)
	}

	pass := len(blocked) == 0
	return Result{PlanID: plan.ID, Pass: pass, Checks: checks, Blocked: blocked}
}

func checkPowerState(vm domain.VM) domain.PreflightCheck {
	switch vm.PowerState {
	case domain.PowerOff:
		return domain.PreflightCheck{Code: "POWER_STATE", Severity: domain.SeverityInfo, Status: domain.CheckPass,
			Message: "Source VM is powered off and ready for cold migration."}
	case domain.PowerOn:
		return domain.PreflightCheck{
			Code: "POWER_STATE", Severity: domain.SeverityWarning, Status: domain.CheckWarn,
			Message: fmt.Sprintf("VM %s is powered on. Execution requires separate approval to power off this exact source VM.", vm.Name),
		}
	default:
		return domain.PreflightCheck{
			Code: "POWER_STATE", Severity: domain.SeverityError, Status: domain.CheckFail,
			Message: fmt.Sprintf("VM %s is %s. Cold migration cannot proceed from this power state.", vm.Name, vm.PowerState),
		}
	}
}

func checkCapacity(vm domain.VM, node domain.TargetNode) domain.PreflightCheck {
	freeMemMB := node.MemoryTotalMB - node.MemoryUsedMB
	if int64(vm.MemoryMB) > freeMemMB {
		return domain.PreflightCheck{
			Code: "CAPACITY", Severity: domain.SeverityError, Status: domain.CheckFail,
			Message: fmt.Sprintf("Target node has %d MB free memory but VM needs %d MB.", freeMemMB, vm.MemoryMB),
		}
	}
	return domain.PreflightCheck{Code: "CAPACITY", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: fmt.Sprintf("Target node has sufficient CPU and %d MB free memory.", freeMemMB)}
}

func checkStorageAvailable(sm domain.StorageMap, vm domain.VM, node domain.TargetNode) domain.PreflightCheck {
	var disk domain.Disk
	for _, d := range vm.Disks {
		if d.ID == sm.SourceDiskID {
			disk = d
			break
		}
	}
	for _, s := range node.Storage {
		if s.ID == sm.TargetStorageID {
			if disk.CapacityBytes > s.FreeBytes {
				return domain.PreflightCheck{
					Code: "STORAGE_" + sm.SourceDiskID, Severity: domain.SeverityError, Status: domain.CheckFail,
					Message: fmt.Sprintf("Storage %s has %d bytes free but disk %s needs %d.", s.Name, s.FreeBytes, disk.ID, disk.CapacityBytes),
				}
			}
			return domain.PreflightCheck{Code: "STORAGE_" + sm.SourceDiskID, Severity: domain.SeverityInfo, Status: domain.CheckPass,
				Message: fmt.Sprintf("Disk %s (%d bytes) fits in storage %s (%d bytes free).", disk.ID, disk.CapacityBytes, s.Name, s.FreeBytes)}
		}
	}
	return domain.PreflightCheck{
		Code: "STORAGE_" + sm.SourceDiskID, Severity: domain.SeverityError, Status: domain.CheckFail,
		Message: fmt.Sprintf("Target storage %s not found on node %s.", sm.TargetStorageID, node.Name),
	}
}

func checkUnsupportedFeatures(vm domain.VM) domain.PreflightCheck {
	for _, d := range vm.Disks {
		if d.Format == domain.DiskRaw {
			return domain.PreflightCheck{
				Code: "UNSUPPORTED", Severity: domain.SeverityError, Status: domain.CheckFail,
				Message: fmt.Sprintf("Disk %s uses raw/RDM format which is not supported in the MVP.", d.ID),
			}
		}
	}
	return domain.PreflightCheck{Code: "UNSUPPORTED", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: "No unsupported features detected (no RDM, passthrough, or encryption)."}
}

func checkFirmware(vm domain.VM, plan domain.Plan) domain.PreflightCheck {
	if vm.Firmware != plan.Firmware {
		return domain.PreflightCheck{Code: "FIRMWARE", Severity: domain.SeverityWarning, Status: domain.CheckWarn,
			Message: fmt.Sprintf("Plan firmware %s differs from source %s. Review carefully.", plan.Firmware, vm.Firmware)}
	}
	return domain.PreflightCheck{Code: "FIRMWARE", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: fmt.Sprintf("Firmware %s preserved.", vm.Firmware)}
}

func checkToolsStatus(vm domain.VM) domain.PreflightCheck {
	if vm.ToolsStatus != "toolsOk" && vm.ToolsStatus != "toolsOld" {
		return domain.PreflightCheck{Code: "VMWARE_TOOLS", Severity: domain.SeverityWarning, Status: domain.CheckWarn,
			Message: fmt.Sprintf("VMware Tools status is %s. Quiesced snapshots may be unavailable.", vm.ToolsStatus)}
	}
	return domain.PreflightCheck{Code: "VMWARE_TOOLS", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: "VMware Tools running."}
}

func checkSnapshots(vm domain.VM) domain.PreflightCheck {
	if len(vm.Snapshots) > 0 {
		return domain.PreflightCheck{Code: "SNAPSHOTS", Severity: domain.SeverityError, Status: domain.CheckFail,
			Message: fmt.Sprintf("VM has %d snapshot(s). Consolidate them manually before export; DRISHTI never removes or consolidates VMware snapshots.", len(vm.Snapshots))}
	}
	return domain.PreflightCheck{Code: "SNAPSHOTS", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: "No snapshots; disk export is straightforward."}
}

func checkNodeOnline(node domain.TargetNode) domain.PreflightCheck {
	if !node.Online {
		return domain.PreflightCheck{Code: "NODE_ONLINE", Severity: domain.SeverityError, Status: domain.CheckFail,
			Message: fmt.Sprintf("Target node %s is offline.", node.Name)}
	}
	return domain.PreflightCheck{Code: "NODE_ONLINE", Severity: domain.SeverityInfo, Status: domain.CheckPass,
		Message: fmt.Sprintf("Target node %s is online.", node.Name)}
}
