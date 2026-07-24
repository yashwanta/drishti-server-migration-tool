package mock

import (
	"time"

	"github.com/drishti/hypershift/internal/domain"
)

const (
	giga = 1024 * 1024 * 1024
	tera = 1024 * giga
)

func vmwareInventory(connID string) domain.InventoryRoot {
	return domain.InventoryRoot{
		ConnectionID: connID,
		GeneratedAt:  time.Now().UTC(),
		Datacenters: []domain.Datacenter{
			{
				ID:   "dc-" + connID,
				Name: "Datacenter",
				Clusters: []domain.Cluster{
					{
						ID:   "cluster-" + connID,
						Name: "Cluster",
						Hosts: []domain.Host{
							{
								ID:            "host-esxi-01",
								Name:          "esxi-01",
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
		Snapshots:    []domain.Snapshot{},
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
		Snapshots:    []domain.Snapshot{},
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

// hypervInventory generates sample Hyper-V inventory as a source. Hyper-V support
// is beyond the MVP but the UI allows registering it; generated inventory lets
// operators see how it would appear.
func hypervInventory(connID string) domain.InventoryRoot {
	return domain.InventoryRoot{
		ConnectionID: connID,
		GeneratedAt:  time.Now().UTC(),
		Datacenters: []domain.Datacenter{
			{
				ID:   "dc-hyperv",
				Name: "Hyper-V Site",
				Clusters: []domain.Cluster{
					{
						ID:   "cluster-hyperv",
						Name: "Hyper-V Nodes",
						Hosts: []domain.Host{
							{
								ID:            "host-hv-01",
								Name:          "hv-01",
								PowerState:    "poweredOn",
								CPUTotalMHz:   48000,
								CPUUsedMHz:    21000,
								MemoryTotalMB: 65536,
								MemoryUsedMB:  32768,
								VMs: []domain.VM{
									{
										ID: "vm-hv-app-01", Name: "app-01", HostID: "host-hv-01",
										PowerState: domain.PowerOff, CPUs: 4, MemoryMB: 8192,
										Firmware: domain.FirmwareUEFI, GuestOS: "Windows Server 2022",
										GuestFamily: domain.GuestWindows, ToolsStatus: "integration-services-ok",
										Disks: []domain.Disk{
											{ID: "disk-hv-0", Label: "Virtual disk 1", CapacityBytes: 80 * giga, Format: domain.DiskRaw, Controller: "scsi", Thin: false, DatastoreID: "vhdx-store"},
										},
										NICs: []domain.NIC{
											{ID: "nic-hv-0", Label: "Network adapter", MACAddress: "00:15:5d:a1:00:10", NetworkID: "net-hv-vlan-100", Connected: true, Model: "hyper-v"},
										},
										Snapshots:    []domain.Snapshot{},
										DatastoreIDs: []string{"vhdx-store"},
										NetworkIDs:   []string{"net-hv-vlan-100"},
										Notes:        "Hyper-V source VM. Note: Hyper-V migration is on the future roadmap (12B), not the current MVP.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}