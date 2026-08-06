// Package platform defines the adapter interfaces that decouple the orchestrator
// from specific hypervisor implementations.
package platform

import (
	"context"

	"github.com/drishti/hypershift/internal/domain"
)

// SourceAdapter provides read-only inventory plus the narrowly-scoped export and
// power operations required by an approved cold migration plan.
type SourceAdapter interface {
	Kind() domain.PlatformKind
	Inventory(ctx context.Context) (domain.InventoryRoot, error)
	VM(ctx context.Context, vmID string) (domain.VM, error)
	ExportDisk(ctx context.Context, vmID, diskID, destPath string) (ExportResult, error)
	PowerOff(ctx context.Context, vmID string, approval PowerOffApproval) error
	PowerState(ctx context.Context, vmID string) (domain.PowerState, error)
}

// PowerOffApproval binds a source power-off authorization to one approved plan
// and VM. A caller cannot request a source mutation without presenting it.
type PowerOffApproval struct {
	PlanID   string
	VMID     string
	Approved bool
}

type ExportResult struct {
	Path      string
	SizeBytes int64
	Format    domain.DiskFormat
	SHA256    string
	Duration  string
}

// TargetAdapter provides read-only target inventory plus create/import ops.
type TargetAdapter interface {
	Kind() domain.PlatformKind
	Inventory(ctx context.Context) (domain.InventoryRoot, error)
	Node(ctx context.Context, nodeID string) (domain.TargetNode, error)
	ReserveVMID(ctx context.Context, nodeID string) (int, error)
	CreateVM(ctx context.Context, spec CreateVMSpec) (int, error)
	AttachDisk(ctx context.Context, vmid int, disk AttachDiskSpec) error
	StartVM(ctx context.Context, vmid int) error
	StopVM(ctx context.Context, vmid int) error
	SetNetwork(ctx context.Context, vmid int, bridge string, vlanID int, isolated bool) error
	VMState(ctx context.Context, vmid int) (domain.PowerState, error)
}

type CreateVMSpec struct {
	Name           string
	NodeID         string
	VMID           int
	CPU            int
	MemoryMB       int64
	Firmware       domain.Firmware
	IdempotencyKey string
	IsolatedBridge string
	StorageID      string
}

type AttachDiskSpec struct {
	Path           string
	Format         domain.DiskFormat
	SizeBytes      int64
	Boot           bool
	Controller     string
	StorageID      string
	DeviceIndex    int
	IdempotencyKey string
}

// AdapterFactory creates the correct adapter for a connection.
type AdapterFactory interface {
	Source(conn domain.Connection) (SourceAdapter, error)
	Target(conn domain.Connection) (TargetAdapter, error)
}
