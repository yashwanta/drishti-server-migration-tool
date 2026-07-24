// Package mock provides a development-only inventory provider that returns
// realistic VMware and Proxmox data without contacting any real platform. It is
// the only provider wired in during Phase 0 and Phase 1.
package mock

import (
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

// Provider returns deterministic sample inventory. It is safe to share.
type Provider struct{}

func New() *Provider { return &Provider{} }

// Connections returns the two default lab connections.
func (p *Provider) Connections() []domain.Connection {
	now := time.Now().UTC()
	return []domain.Connection{
		{
			ID: "conn-vmware-lab", Name: "vCenter Lab", Kind: domain.PlatformVMware, Role: domain.RoleSource,
			Endpoint: "vcenter-lab.example.local", Status: domain.ConnConnected, SecretRef: "vmw/lab-admin",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "conn-proxmox-lab", Name: "Proxmox Lab", Kind: domain.PlatformProxmox, Role: domain.RoleTarget,
			Endpoint: "pve-lab.example.local", Status: domain.ConnConnected, SecretRef: "pve/lab-token",
			CreatedAt: now, UpdatedAt: now,
		},
	}
}

// Inventory returns normalized inventory for the given connection id.
func (p *Provider) Inventory(connID string) (domain.InventoryRoot, bool) {
	switch connID {
	case "conn-vmware-lab":
		return vmwareInventory(connID), true
	case "conn-proxmox-lab":
		return proxmoxInventory(connID), true
	default:
		return domain.InventoryRoot{}, false
	}
}

func vmwareInventory(connID string) domain.InventoryRoot {
	return domain.InventoryRoot{
		ConnectionID: connID,
		GeneratedAt:  time.Now().UTC(),
		Datacenters: []domain.Datacenter{
			{
				ID:   "dc-lab",
				Name: "Lab-DC",
				Clusters: []domain.Cluster{
					{
						ID:   "cluster-lab",
						Name: "Lab-Cluster",
						Hosts: []domain.Host{
							{
								ID:            "host-esxi-01",
								Name:          "esxi-01.lab.local",
								PowerState:    "poweredOn",
								CPUTotalMHz:   64000,
								CPUUsedMHz:    18000,
								MemoryTotalMB: 131072,
								MemoryUsedMB:  49152,
								VMs: []domain.VM{
									webServerVM(),
									dbServerVM(),
									unsupportedVM(),
								},
							},
						},
					},
				},
			},
		},
	}
}

func webServerVM() domain.VM {
	return domain.VM{
		ID: "vm-web-01", Name: "web-01", HostID: "host-esxi-01",
		PowerState: domain.PowerOff, CPUs: 2, MemoryMB: 4096,
		Firmware: domain.FirmwareBIOS, GuestOS: "ubuntu64Guest",
		GuestFamily: domain.GuestLinux, ToolsStatus: "toolsOk",
		Disks: []domain.Disk{
			{ID: "disk-web-0", Label: "Hard disk 1", CapacityBytes: 40 * giga, Format: domain.DiskVMDK, Controller: "lsilogic", Thin: true, DatastoreID: "ds-local-01"},
		},
		NICs: []domain.NIC{
			{ID: "nic-web-0", Label: "Network adapter 1", MACAddress: "00:50:56:a1:00:01", NetworkID: "net-vlan-10", Connected: true, Model: "vmxnet3"},
		},
		DatastoreIDs: []string{"ds-local-01"},
		NetworkIDs:   []string{"net-vlan-10"},
		Notes:        "Static IP 10.10.10.21 (VLAN 10). Cold migration candidate.",
	}
}

func dbServerVM() domain.VM {
	return domain.VM{
		ID: "vm-db-01", Name: "db-01", HostID: "host-esxi-01",
		PowerState: domain.PowerOff, CPUs: 4, MemoryMB: 16384,
		Firmware: domain.FirmwareUEFI, GuestOS: "windows2019srv_64",
		GuestFamily: domain.GuestWindows, ToolsStatus: "toolsOk",
		Disks: []domain.Disk{
			{ID: "disk-db-0", Label: "Hard disk 1", CapacityBytes: 60 * giga, Format: domain.DiskVMDK, Controller: "lsisas", Thin: true, DatastoreID: "ds-local-01"},
			{ID: "disk-db-1", Label: "Hard disk 2", CapacityBytes: 200 * giga, Format: domain.DiskVMDK, Controller: "lsisas", Thin: true, DatastoreID: "ds-shared-01"},
		},
		NICs: []domain.NIC{
			{ID: "nic-db-0", Label: "Network adapter 1", MACAddress: "00:50:56:a1:00:02", NetworkID: "net-vlan-20", Connected: true, Model: "vmxnet3"},
		},
		Snapshots: []domain.Snapshot{
			{ID: "snap-db-1", Name: "pre-patch-2026-06", Description: "Before June patches", CreatedAt: time.Now().UTC().AddDate(0, -1, 0), Current: true},
		},
		DatastoreIDs: []string{"ds-local-01", "ds-shared-01"},
		NetworkIDs:   []string{"net-vlan-20"},
		Notes:        "Static IP 10.10.20.31 (VLAN 20). Has snapshot requiring review.",
	}
}

func unsupportedVM() domain.VM {
	return domain.VM{
		ID: "vm-rdm-01", Name: "legacy-rdm", HostID: "host-esxi-01",
		PowerState: domain.PowerOn, CPUs: 2, MemoryMB: 8192,
		Firmware: domain.FirmwareBIOS, GuestOS: "rhel7_64Guest",
		GuestFamily: domain.GuestLinux, ToolsStatus: "toolsNotRunning",
		Disks: []domain.Disk{
			{ID: "disk-rdm-0", Label: "Hard disk 1", CapacityBytes: 500 * giga, Format: domain.DiskRaw, Controller: "pvscsi", Thin: false, DatastoreID: "ds-rdm"},
		},
		NICs: []domain.NIC{
			{ID: "nic-rdm-0", Label: "Network adapter 1", MACAddress: "00:50:56:a1:00:03", NetworkID: "net-vlan-30", Connected: true, Model: "vmxnet3"},
		},
		DatastoreIDs: []string{"ds-rdm"},
		NetworkIDs:   []string{"net-vlan-30"},
		Notes:        "Uses RDM/shared disk and is powered on. Blocked by preflight.",
	}
}

func proxmoxInventory(connID string) domain.InventoryRoot {
	next := 102
	return domain.InventoryRoot{
		ConnectionID: connID,
		GeneratedAt:  time.Now().UTC(),
		Nodes: []domain.TargetNode{
			{
				ID: "node-pve-01", Name: "pve-01", Online: true,
				CPUTotalMHz: 64000, CPUUsedMHz: 12000,
				MemoryTotalMB: 131072, MemoryUsedMB: 40960,
				VLANAware: true, NextVMID: next,
				Storage: []domain.TargetStorage{
					{ID: "local-lvm", Name: "local-lvm", Type: "lvm", ContentTypes: []string{"rootdir", "images"}, CapacityBytes: 400 * giga, FreeBytes: 250 * giga, Shared: false},
					{ID: "nfs-shared", Name: "nfs-shared", Type: "nfs", ContentTypes: []string{"rootdir", "images", "iso"}, CapacityBytes: 2 * tera, FreeBytes: 1 * tera, Shared: true},
				},
				Bridges: []domain.Bridge{
					{ID: "vmbr0", Name: "vmbr0", VLANAware: true, Ports: []string{"eno1"}},
					{ID: "vmbr1", Name: "vmbr1", VLANAware: true, Ports: []string{"eno2"}},
				},
				VMs: []domain.TargetVM{
					{ID: 100, Name: "monitor", NodeID: "node-pve-01", Status: domain.PowerOn, CPUs: 2, MemoryMB: 4096},
					{ID: 101, Name: "bastion", NodeID: "node-pve-01", Status: domain.PowerOff, CPUs: 1, MemoryMB: 2048},
				},
			},
		},
	}
}

const (
	giga = 1024 * 1024 * 1024
	tera = 1024 * giga
)
