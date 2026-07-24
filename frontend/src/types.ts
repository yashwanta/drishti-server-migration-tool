// Types mirroring the Go domain model in backend/internal/domain/models.go.
// These are the API contracts for the React UI.

export type PlatformKind = 'vmware' | 'proxmox' | 'hyperv';
export type ConnRole = 'source' | 'target';
export type ConnStatus = 'unknown' | 'connected' | 'error';
export type PowerState = 'on' | 'off' | 'suspended';
export type Firmware = 'bios' | 'uefi';
export type GuestFamily = 'linux' | 'windows' | 'other';
export type DiskFormat = 'vmdk' | 'raw' | 'qcow2';
export type PlanStatus = 'draft' | 'preflight' | 'approved' | 'rejected' | 'archived';
export type Severity = 'info' | 'warning' | 'error';
export type CheckStatus = 'pass' | 'fail' | 'warn' | 'skipped';
export type JobState = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'rolled_back';

export interface Connection {
  id: string;
  name: string;
  kind: PlatformKind;
  role: ConnRole;
  endpoint: string;
  insecure_tls: boolean;
  status: ConnStatus;
  secret_ref?: string;
  created_at: string;
  updated_at: string;
}

export interface Disk {
  id: string;
  label: string;
  capacity_bytes: number;
  format: DiskFormat;
  controller: string;
  thin: boolean;
  datastore_id: string;
}

export interface NIC {
  id: string;
  label: string;
  mac_address: string;
  network_id: string;
  connected: boolean;
  model: string;
}

export interface Snapshot {
  id: string;
  name: string;
  description: string;
  created_at: string;
  current: boolean;
}

export interface VM {
  id: string;
  name: string;
  host_id: string;
  power_state: PowerState;
  cpus: number;
  memory_mb: number;
  firmware: Firmware;
  guest_os: string;
  guest_family: GuestFamily;
  tools_status: string;
  disks: Disk[];
  nics: NIC[];
  snapshots: Snapshot[];
  datastore_ids: string[];
  network_ids: string[];
  notes: string;
}

export interface Host {
  id: string;
  name: string;
  power_state: string;
  cpu_total_mhz: number;
  cpu_used_mhz: number;
  memory_total_mb: number;
  memory_used_mb: number;
  vms: VM[];
}

export interface Cluster {
  id: string;
  name: string;
  hosts: Host[];
}

export interface Datacenter {
  id: string;
  name: string;
  clusters: Cluster[];
}

export interface TargetStorage {
  id: string;
  name: string;
  type: string;
  content_types: string[];
  capacity_bytes: number;
  free_bytes: number;
  shared: boolean;
}

export interface Bridge {
  id: string;
  name: string;
  vlan_aware: boolean;
  ports: string[];
}

export interface TargetVM {
  id: number;
  name: string;
  node_id: string;
  status: PowerState;
  cpus: number;
  memory_mb: number;
}

export interface TargetNode {
  id: string;
  name: string;
  online: boolean;
  cpu_total_mhz: number;
  cpu_used_mhz: number;
  memory_total_mb: number;
  memory_used_mb: number;
  storage: TargetStorage[];
  bridges: Bridge[];
  vlan_aware: boolean;
  next_vmid: number;
  vms: TargetVM[];
}

export interface InventoryRoot {
  connection_id: string;
  generated_at: string;
  datacenters?: Datacenter[];
  nodes?: TargetNode[];
}

export interface StorageMap {
  source_disk_id: string;
  target_storage_id: string;
  target_format: DiskFormat;
}

export interface NetworkMap {
  source_nic_id: string;
  target_bridge: string;
  vlan_id?: number;
}

export interface Plan {
  id: string;
  name: string;
  source_vm_id: string;
  source_connection_id: string;
  target_node_id: string;
  target_connection_id: string;
  target_vm_name: string;
  target_vmid?: number;
  cpu: number;
  memory_mb: number;
  firmware: Firmware;
  storage_maps: StorageMap[];
  network_maps: NetworkMap[];
  status: PlanStatus;
  disk_format: DiskFormat;
  created_at: string;
  updated_at: string;
  created_by: string;
}

export interface PreflightCheck {
  id: string;
  code: string;
  severity: Severity;
  status: CheckStatus;
  message: string;
  detail?: string;
}

export interface AuditEvent {
  id: string;
  timestamp: string;
  actor: string;
  action: string;
  target: string;
  result: string;
  detail?: string;
}

export interface PreflightResult {
  plan_id: string;
  pass: boolean;
  checks: PreflightCheck[];
  blocked?: string[];
}

export interface ValidationResult {
  passed: boolean;
  checks: { name: string; status: string; detail: string }[];
}

export interface CutoverResult {
  success: boolean;
  steps: string[];
  warning?: string;
  retention_deadline?: string;
}

export interface RollbackResult {
  success: boolean;
  steps: string[];
  warning?: string;
}

export interface Job {
  ID: string;
  PlanID: string;
  state: JobState;
  steps?: { Name: string; State: JobState; Message: string; StartedAt?: string; FinishedAt?: string }[];
  StartedAt?: string;
  FinishedAt?: string;
}

export interface ApiError {
  error: string;
  code?: string;
  detail?: string;
}