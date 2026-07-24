package remediation

import "github.com/drishti/hypershift/internal/domain"

// GuestFacts captures the network identity and configuration collected before
// migration.
type GuestFacts struct {
	VMID        string
	Hostname    string
	Interfaces  []NetInterface
	DNS         []string
	Gateways    []string
	Routes      []string
	Services    []string
	VirtIOReady bool
	QemuAgent   bool
}

type NetInterface struct {
	Name      string
	MAC       string
	IPAddress string
	Netmask   string
	Gateway   string
	DHCP      bool
	NetworkID string
	VLANID    int
}

func CollectFacts(vm domain.VM) GuestFacts {
	facts := GuestFacts{VMID: vm.ID, Hostname: vm.Name}
	for _, nic := range vm.NICs {
		facts.Interfaces = append(facts.Interfaces, NetInterface{
			Name: nic.Label, MAC: nic.MACAddress, NetworkID: nic.NetworkID, DHCP: false,
		})
	}
	facts.VirtIOReady = vm.GuestFamily == domain.GuestLinux
	facts.QemuAgent = vm.GuestFamily == domain.GuestLinux
	if vm.GuestFamily == domain.GuestWindows {
		facts.Services = []string{"Requires VirtIO driver injection (Phase 7)"}
	}
	return facts
}

type RestoreNetworkPlan struct {
	Safe    bool
	Steps   []string
	Warning string
}

func PlanRestore(facts GuestFacts, sourceOff bool, validationPassed bool) RestoreNetworkPlan {
	if !sourceOff {
		return RestoreNetworkPlan{Safe: false, Warning: "Source VM is not confirmed powered off. Refusing to restore production network to avoid duplicate identity."}
	}
	if !validationPassed {
		return RestoreNetworkPlan{Safe: false, Warning: "Target validation has not passed. Network restore blocked."}
	}
	if !facts.VirtIOReady {
		return RestoreNetworkPlan{Safe: false, Warning: "VirtIO drivers not ready. Manual remediation required before cutover."}
	}
	return RestoreNetworkPlan{Safe: true, Steps: []string{
		"Source confirmed powered off.",
		"Target validation passed.",
		"Attaching target to approved production network.",
		"Restoring static IP configuration from encrypted guest facts.",
		"Verifying gateway, DNS, and required TCP ports.",
	}}
}