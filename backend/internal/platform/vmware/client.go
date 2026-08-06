// Package vmware implements vCenter and standalone ESXi discovery plus the
// narrowly scoped source operations required for an approved cold migration.
package vmware

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// Client contains only the information required for read-only API calls.
type Client struct {
	endpoint    string
	username    string
	password    string
	insecureTLS bool
}

func New(endpoint, username, password string, insecureTLS bool) *Client {
	return &Client{endpoint: endpoint, username: username, password: password, insecureTLS: insecureTLS}
}

// Test authenticates and logs out without making a mutating API call.
func (c *Client) Test(ctx context.Context) error {
	client, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = client.Logout(context.Background()) }()
	return nil
}

// Inventory reads and normalizes VMware inventory. It never changes VMware state.
func (c *Client) Inventory(ctx context.Context, connectionID string) (domain.InventoryRoot, error) {
	client, err := c.connect(ctx)
	if err != nil {
		return domain.InventoryRoot{}, err
	}
	defer func() { _ = client.Logout(context.Background()) }()

	finder := find.NewFinder(client.Client, true)
	datacenters, err := finder.DatacenterList(ctx, "*")
	if err != nil {
		return domain.InventoryRoot{}, fmt.Errorf("discover datacenters: %w", err)
	}
	result := domain.InventoryRoot{
		ConnectionID: connectionID,
		GeneratedAt:  time.Now().UTC(),
		Datacenters:  make([]domain.Datacenter, 0, len(datacenters)),
	}

	collector := property.DefaultCollector(client.Client)
	for _, dcObject := range datacenters {
		finder.SetDatacenter(dcObject)
		hostObjects, err := finder.HostSystemList(ctx, "*")
		if err != nil {
			return domain.InventoryRoot{}, fmt.Errorf("discover hosts in %s: %w", dcObject.Name(), err)
		}
		vmObjects, err := finder.VirtualMachineList(ctx, "*")
		if err != nil && !isNotFound(err) {
			return domain.InventoryRoot{}, fmt.Errorf("discover virtual machines in %s: %w", dcObject.Name(), err)
		}

		hostRefs := make([]types.ManagedObjectReference, 0, len(hostObjects))
		for _, object := range hostObjects {
			hostRefs = append(hostRefs, object.Reference())
		}
		var hostProperties []mo.HostSystem
		if len(hostRefs) > 0 {
			if err := collector.Retrieve(ctx, hostRefs, []string{"name", "parent", "runtime", "summary"}, &hostProperties); err != nil {
				return domain.InventoryRoot{}, fmt.Errorf("read host properties in %s: %w", dcObject.Name(), err)
			}
		}

		vmRefs := make([]types.ManagedObjectReference, 0, len(vmObjects))
		for _, object := range vmObjects {
			vmRefs = append(vmRefs, object.Reference())
		}
		var vmProperties []mo.VirtualMachine
		if len(vmRefs) > 0 {
			properties := []string{"name", "config", "runtime", "guest", "datastore", "network", "snapshot"}
			if err := collector.Retrieve(ctx, vmRefs, properties, &vmProperties); err != nil {
				return domain.InventoryRoot{}, fmt.Errorf("read virtual machine properties in %s: %w", dcObject.Name(), err)
			}
		}

		vmsByHost := make(map[string][]domain.VM)
		for i := range vmProperties {
			vm := normalizeVM(&vmProperties[i])
			if vm.ID == "" { // Templates and inaccessible VMs are intentionally skipped.
				continue
			}
			vmsByHost[vm.HostID] = append(vmsByHost[vm.HostID], vm)
		}

		clusters := make([]domain.Cluster, 0, len(hostProperties))
		for i := range hostProperties {
			host := normalizeHost(&hostProperties[i])
			host.VMs = vmsByHost[host.ID]
			clusters = append(clusters, domain.Cluster{
				ID:    "compute-" + host.ID,
				Name:  host.Name,
				Hosts: []domain.Host{host},
			})
		}
		result.Datacenters = append(result.Datacenters, domain.Datacenter{
			ID: dcObject.Reference().Value, Name: dcObject.Name(), Clusters: clusters,
		})
	}
	return result, nil
}

func (c *Client) connect(ctx context.Context) (*govmomi.Client, error) {
	endpoint := strings.TrimSpace(c.endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := soap.ParseURL(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid VMware endpoint: %w", err)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/sdk"
	}
	u.User = url.UserPassword(c.username, c.password)
	client, err := govmomi.NewClient(ctx, u, c.insecureTLS)
	if err != nil {
		return nil, fmt.Errorf("VMware authentication or connection failed: %w", err)
	}
	return client, nil
}

func normalizeHost(src *mo.HostSystem) domain.Host {
	host := domain.Host{ID: src.Self.Value, Name: src.Name, PowerState: string(src.Runtime.PowerState)}
	if src.Summary.Hardware != nil {
		host.CPUTotalMHz = int64(src.Summary.Hardware.CpuMhz) * int64(src.Summary.Hardware.NumCpuCores)
		host.MemoryTotalMB = src.Summary.Hardware.MemorySize / (1024 * 1024)
	}
	host.CPUUsedMHz = int64(src.Summary.QuickStats.OverallCpuUsage)
	host.MemoryUsedMB = int64(src.Summary.QuickStats.OverallMemoryUsage)
	return host
}

func normalizeVM(src *mo.VirtualMachine) domain.VM {
	if src.Config == nil || src.Config.Template {
		return domain.VM{}
	}
	vm := domain.VM{
		ID: src.Self.Value, Name: src.Name, PowerState: normalizePower(src.Runtime.PowerState),
		CPUs: int(src.Config.Hardware.NumCPU), MemoryMB: int64(src.Config.Hardware.MemoryMB),
		Firmware: domain.FirmwareBIOS, GuestOS: src.Config.GuestFullName,
		GuestFamily: domain.GuestOther, Notes: src.Config.Annotation,
	}
	if src.Runtime.Host != nil {
		vm.HostID = src.Runtime.Host.Value
	}
	if strings.EqualFold(src.Config.Firmware, "efi") {
		vm.Firmware = domain.FirmwareUEFI
	}
	if src.Guest != nil {
		if src.Guest.GuestFullName != "" {
			vm.GuestOS = src.Guest.GuestFullName
		}
		vm.GuestFamily = normalizeGuestFamily(src.Guest.GuestFamily, vm.GuestOS)
		vm.ToolsStatus = src.Guest.ToolsRunningStatus
		if vm.ToolsStatus == "" {
			vm.ToolsStatus = string(src.Guest.ToolsStatus)
		}
	}
	for _, ref := range src.Datastore {
		vm.DatastoreIDs = append(vm.DatastoreIDs, ref.Value)
	}
	for _, ref := range src.Network {
		vm.NetworkIDs = append(vm.NetworkIDs, ref.Value)
	}
	vm.Disks, vm.NICs = normalizeDevices(src.Config.Hardware.Device)
	if src.Snapshot != nil {
		current := ""
		if src.Snapshot.CurrentSnapshot != nil {
			current = src.Snapshot.CurrentSnapshot.Value
		}
		vm.Snapshots = normalizeSnapshots(src.Snapshot.RootSnapshotList, current)
	}
	return vm
}

func normalizeDevices(devices []types.BaseVirtualDevice) ([]domain.Disk, []domain.NIC) {
	var disks []domain.Disk
	var nics []domain.NIC
	for _, device := range devices {
		switch typed := device.(type) {
		case *types.VirtualDisk:
			base := typed.GetVirtualDevice()
			disk := domain.Disk{ID: strconv.Itoa(int(base.Key)), CapacityBytes: typed.CapacityInBytes, Format: domain.DiskVMDK}
			if disk.CapacityBytes == 0 {
				disk.CapacityBytes = typed.CapacityInKB * 1024
			}
			if base.DeviceInfo != nil {
				disk.Label = base.DeviceInfo.GetDescription().Label
			}
			if backing, ok := base.Backing.(types.BaseVirtualDeviceFileBackingInfo); ok {
				file := backing.GetVirtualDeviceFileBackingInfo()
				if file.Datastore != nil {
					disk.DatastoreID = file.Datastore.Value
				}
			}
			if backing, ok := base.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok && backing.ThinProvisioned != nil {
				disk.Thin = *backing.ThinProvisioned
			}
			disks = append(disks, disk)
		case types.BaseVirtualEthernetCard:
			card := typed.GetVirtualEthernetCard()
			base := card.GetVirtualDevice()
			nic := domain.NIC{ID: strconv.Itoa(int(base.Key)), MACAddress: card.MacAddress, Model: deviceModel(device)}
			if base.DeviceInfo != nil {
				nic.Label = base.DeviceInfo.GetDescription().Label
			}
			if base.Connectable != nil {
				nic.Connected = base.Connectable.Connected
			}
			nic.NetworkID = networkID(base.Backing)
			nics = append(nics, nic)
		}
	}
	return disks, nics
}

func normalizeSnapshots(roots []types.VirtualMachineSnapshotTree, current string) []domain.Snapshot {
	var out []domain.Snapshot
	var walk func([]types.VirtualMachineSnapshotTree)
	walk = func(nodes []types.VirtualMachineSnapshotTree) {
		for _, node := range nodes {
			out = append(out, domain.Snapshot{ID: node.Snapshot.Value, Name: node.Name, Description: node.Description, CreatedAt: node.CreateTime, Current: node.Snapshot.Value == current})
			walk(node.ChildSnapshotList)
		}
	}
	walk(roots)
	return out
}

func normalizePower(state types.VirtualMachinePowerState) domain.PowerState {
	switch state {
	case types.VirtualMachinePowerStatePoweredOn:
		return domain.PowerOn
	case types.VirtualMachinePowerStateSuspended:
		return domain.PowerSuspended
	default:
		return domain.PowerOff
	}
}

func normalizeGuestFamily(family, name string) domain.GuestFamily {
	value := strings.ToLower(family + " " + name)
	if strings.Contains(value, "windows") {
		return domain.GuestWindows
	}
	if strings.Contains(value, "linux") {
		return domain.GuestLinux
	}
	return domain.GuestOther
}

func deviceModel(device types.BaseVirtualDevice) string {
	name := fmt.Sprintf("%T", device)
	name = strings.TrimPrefix(name, "*types.Virtual")
	return strings.ToLower(name)
}

func networkID(backing types.BaseVirtualDeviceBackingInfo) string {
	switch value := backing.(type) {
	case *types.VirtualEthernetCardNetworkBackingInfo:
		if value.Network != nil {
			return value.Network.Value
		}
		return value.DeviceName
	case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
		return value.Port.PortgroupKey
	case *types.VirtualEthernetCardOpaqueNetworkBackingInfo:
		return value.OpaqueNetworkId
	default:
		return ""
	}
}

func isNotFound(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
