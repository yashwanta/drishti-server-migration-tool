package vmware

import (
	"context"
	"fmt"
	"strconv"

	"github.com/drishti/hypershift/internal/domain"
	"github.com/drishti/hypershift/internal/platform"
	"github.com/drishti/hypershift/internal/safety"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// Source is a VMware implementation of platform.SourceAdapter. Its only
// mutating operation is an explicitly approved power-off.
type Source struct {
	client       *Client
	connectionID string
	workspace    string
	mutations    safety.MutationPolicy
}

// NewSource creates a VMware source adapter. Construction performs no network
// access; each operation opens and closes its own authenticated API session.
func NewSource(connectionID, endpoint, username, password string, insecureTLS bool, workspace string, mutations safety.MutationPolicy) *Source {
	return &Source{
		client:       New(endpoint, username, password, insecureTLS),
		connectionID: connectionID,
		workspace:    workspace,
		mutations:    mutations,
	}
}

func (s *Source) Kind() domain.PlatformKind { return domain.PlatformVMware }

func (s *Source) Inventory(ctx context.Context) (domain.InventoryRoot, error) {
	return s.client.Inventory(ctx, s.connectionID)
}

func (s *Source) VM(ctx context.Context, vmID string) (domain.VM, error) {
	client, err := s.client.connect(ctx)
	if err != nil {
		return domain.VM{}, err
	}
	defer func() { _ = client.Logout(context.Background()) }()
	vm, err := retrieveVM(ctx, client.Client, vmID, []string{"name", "config", "runtime", "guest", "datastore", "network", "snapshot"})
	if err != nil {
		return domain.VM{}, err
	}
	normalized := normalizeVM(vm)
	if normalized.ID == "" {
		return domain.VM{}, fmt.Errorf("vm %q is a template or inaccessible", vmID)
	}
	return normalized, nil
}

func (s *Source) PowerState(ctx context.Context, vmID string) (domain.PowerState, error) {
	client, err := s.client.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Logout(context.Background()) }()
	vm, err := retrieveVM(ctx, client.Client, vmID, []string{"runtime.powerState"})
	if err != nil {
		return "", err
	}
	return normalizePower(vm.Runtime.PowerState), nil
}

func retrieveVM(ctx context.Context, client *vim25.Client, vmID string, properties []string) (*mo.VirtualMachine, error) {
	if vmID == "" {
		return nil, fmt.Errorf("VM managed-object ID is required")
	}
	ref := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmID}
	collector := property.DefaultCollector(client)
	var vm mo.VirtualMachine
	if err := collector.RetrieveOne(ctx, ref, properties, &vm); err != nil {
		return nil, fmt.Errorf("read VM %q: %w", vmID, err)
	}
	return &vm, nil
}

func findVirtualDisk(vm *mo.VirtualMachine, diskID string) (*types.VirtualDisk, error) {
	if vm.Config == nil {
		return nil, fmt.Errorf("VM configuration is unavailable")
	}
	key, err := strconv.Atoi(diskID)
	if err != nil {
		return nil, fmt.Errorf("invalid VMware disk ID %q", diskID)
	}
	for _, device := range vm.Config.Hardware.Device {
		if disk, ok := device.(*types.VirtualDisk); ok && int(disk.Key) == key {
			return disk, nil
		}
	}
	return nil, fmt.Errorf("disk %q not found on VM", diskID)
}

type objectDatastoreDownloader struct{ datastore *object.Datastore }

func (d objectDatastoreDownloader) Download(ctx context.Context, remotePath, localPath string) error {
	return d.datastore.DownloadFile(ctx, remotePath, localPath, &soap.Download{})
}

var _ platform.SourceAdapter = (*Source)(nil)
