// Package domain defines the normalized, hypervisor-agnostic model used across
// the orchestrator, adapters, worker, and UI. Nothing in this package performs
// platform I/O.
package domain

import "time"

// PlatformKind identifies the source/target platform of a connection.
type PlatformKind string

const (
	PlatformVMware  PlatformKind = "vmware"
	PlatformProxmox PlatformKind = "proxmox"
	PlatformHyperV   PlatformKind = "hyperv"
)

// Role of a connection: source provides VMs to migrate; target receives them.
type Role string

const (
	RoleSource Role = "source"
	RoleTarget Role = "target"
)

// Connection is a configured, authenticated platform endpoint. Secrets are
// referenced by SecretRef and never stored inline.
type Connection struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        PlatformKind `json:"kind"`
	Role        Role       `json:"role"`
	Endpoint    string     `json:"endpoint"`
	InsecureTLS bool       `json:"insecure_tls"`
	Status      ConnStatus `json:"status"`
	SecretRef   string     `json:"secret_ref,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ConnStatus string

const (
	ConnUnknown   ConnStatus = "unknown"
	ConnConnected ConnStatus = "connected"
	ConnError     ConnStatus = "error"
)

// InventoryRoot is the top-level normalized inventory for a connection.
type InventoryRoot struct {
	ConnectionID string       `json:"connection_id"`
	GeneratedAt  time.Time    `json:"generated_at"`
	Datacenters  []Datacenter `json:"datacenters"`
	Nodes        []TargetNode `json:"nodes,omitempty"`
}

// Datacenter groups source-side inventory.
type Datacenter struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Clusters []Cluster  `json:"clusters"`
}

type Cluster struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Hosts []Host `json:"hosts"`
}

type Host struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PowerState    string `json:"power_state"`
	CPUTotalMHz   int64  `json:"cpu_total_mhz"`
	CPUUsedMHz    int64  `json:"cpu_used_mhz"`
	MemoryTotalMB int64  `json:"memory_total_mb"`
	MemoryUsedMB  int64  `json:"memory_used_mb"`
	VMs           []VM   `json:"vms"`
}

// VM is a normalized source virtual machine.
type VM struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	HostID       string      `json:"host_id"`
	PowerState   PowerState  `json:"power_state"`
	CPUs         int         `json:"cpus"`
	MemoryMB     int64       `json:"memory_mb"`
	Firmware     Firmware    `json:"firmware"`
	GuestOS      string      `json:"guest_os"`
	GuestFamily  GuestFamily `json:"guest_family"`
	ToolsStatus  string      `json:"tools_status"`
	Disks        []Disk      `json:"disks"`
	NICs         []NIC       `json:"nics"`
	Snapshots    []Snapshot  `json:"snapshots"`
	DatastoreIDs []string    `json:"datastore_ids"`
	NetworkIDs   []string    `json:"network_ids"`
	Notes        string      `json:"notes"`
}

type PowerState string

const (
	PowerOn        PowerState = "on"
	PowerOff       PowerState = "off"
	PowerSuspended PowerState = "suspended"
)

type Firmware string

const (
	FirmwareBIOS Firmware = "bios"
	FirmwareUEFI Firmware = "uefi"
)

type GuestFamily string

const (
	GuestLinux   GuestFamily = "linux"
	GuestWindows GuestFamily = "windows"
	GuestOther   GuestFamily = "other"
)

type Disk struct {
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	CapacityBytes int64      `json:"capacity_bytes"`
	Format        DiskFormat `json:"format"`
	Controller    string     `json:"controller"`
	Thin          bool       `json:"thin"`
	DatastoreID   string     `json:"datastore_id"`
}

type DiskFormat string

const (
	DiskVMDK  DiskFormat = "vmdk"
	DiskRaw   DiskFormat = "raw"
	DiskQCOW2 DiskFormat = "qcow2"
)

type NIC struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	MACAddress string `json:"mac_address"`
	NetworkID string `json:"network_id"`
	Connected bool   `json:"connected"`
	Model     string `json:"model"`
}

type Snapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Current     bool      `json:"current"`
}

type Datastore struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	CapacityBytes int64  `json:"capacity_bytes"`
	FreeBytes     int64  `json:"free_bytes"`
	Accessible    bool   `json:"accessible"`
}

type Network struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	VLANID int    `json:"vlan_id,omitempty"`
}

// TargetNode is a normalized Proxmox node.
type TargetNode struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Online        bool            `json:"online"`
	CPUTotalMHz   int64           `json:"cpu_total_mhz"`
	CPUUsedMHz    int64           `json:"cpu_used_mhz"`
	MemoryTotalMB int64           `json:"memory_total_mb"`
	MemoryUsedMB  int64           `json:"memory_used_mb"`
	Storage       []TargetStorage `json:"storage"`
	Bridges       []Bridge        `json:"bridges"`
	VLANAware     bool            `json:"vlan_aware"`
	NextVMID      int             `json:"next_vmid"`
	VMs           []TargetVM      `json:"vms"`
}

type TargetStorage struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	ContentTypes  []string `json:"content_types"`
	CapacityBytes int64    `json:"capacity_bytes"`
	FreeBytes     int64    `json:"free_bytes"`
	Shared        bool     `json:"shared"`
}

type Bridge struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	VLANAware bool     `json:"vlan_aware"`
	Ports     []string `json:"ports"`
}

type TargetVM struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	NodeID   string     `json:"node_id"`
	Status   PowerState `json:"status"`
	CPUs     int        `json:"cpus"`
	MemoryMB int64      `json:"memory_mb"`
}

// Plan is a draft or approved migration plan. It is created by a drop and
// never executes a migration by itself.
type Plan struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	SourceVMID   string       `json:"source_vm_id"`
	SourceConnID string       `json:"source_connection_id"`
	TargetNodeID string       `json:"target_node_id"`
	TargetConnID string       `json:"target_connection_id"`
	TargetVMName string       `json:"target_vm_name"`
	TargetVMID   *int         `json:"target_vmid,omitempty"`
	CPU          int          `json:"cpu"`
	MemoryMB     int64        `json:"memory_mb"`
	Firmware     Firmware     `json:"firmware"`
	StorageMaps  []StorageMap `json:"storage_maps"`
	NetworkMaps  []NetworkMap `json:"network_maps"`
	Status       PlanStatus   `json:"status"`
	DiskFormat   DiskFormat   `json:"disk_format"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	CreatedBy    string       `json:"created_by"`
}

type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanPreflight PlanStatus = "preflight"
	PlanApproved  PlanStatus = "approved"
	PlanRejected  PlanStatus = "rejected"
	PlanArchived  PlanStatus = "archived"
)

type StorageMap struct {
	SourceDiskID    string     `json:"source_disk_id"`
	TargetStorageID string     `json:"target_storage_id"`
	TargetFormat    DiskFormat `json:"target_format"`
}

type NetworkMap struct {
	SourceNICID  string `json:"source_nic_id"`
	TargetBridge string `json:"target_bridge"`
	VLANID       int    `json:"vlan_id,omitempty"`
}

// PreflightCheck is the outcome of a single compatibility/safety check.
type PreflightCheck struct {
	ID       string      `json:"id"`
	Code     string      `json:"code"`
	Severity Severity    `json:"severity"`
	Status   CheckStatus `json:"status"`
	Message  string      `json:"message"`
	Detail   string      `json:"detail,omitempty"`
}

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckFail    CheckStatus = "fail"
	CheckWarn    CheckStatus = "warn"
	CheckSkipped CheckStatus = "skipped"
)

// Job is an executed (or executing) migration.
type Job struct {
	ID             string     `json:"id"`
	PlanID         string     `json:"plan_id"`
	State          JobState   `json:"state"`
	Steps          []JobStep  `json:"steps"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type JobState string

const (
	JobPending    JobState = "pending"
	JobRunning    JobState = "running"
	JobSucceeded  JobState = "succeeded"
	JobFailed     JobState = "failed"
	JobCancelled  JobState = "cancelled"
	JobRolledBack JobState = "rolled_back"
)

type JobStep struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	State      JobState   `json:"state"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExternalID string     `json:"external_id,omitempty"`
	Message    string     `json:"message,omitempty"`
}

// AuditEvent is a tamper-evident record of an operator or system action.
type AuditEvent struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Result    string    `json:"result"`
	Detail    string    `json:"detail,omitempty"`
}
