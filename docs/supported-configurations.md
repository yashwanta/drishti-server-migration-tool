# Supported-Configuration Matrix

**Version:** 0.1 (Phase 0-10 implementation)
**Date:** 2026-07-24

## Source platforms (VMware)

| Feature | Supported | Notes |
|---------|-----------|-------|
| vCenter Server | Design complete | Real connector via govmomi (Phase 2 interface ready). Mock adapter active. |
| Standalone ESXi | Design complete | Same adapter interface. |
| Powered-off VMs | Yes | Cold migration only. VM must be off before disk copy. |
| Powered-on VMs | No | Blocked at preflight. Cold migration only in MVP. |
| Linux guests | Yes | VirtIO readiness assumed (mock). Real validation in Phase 7. |
| Windows guests | Conditional | Requires VirtIO driver injection. Flagged for manual review. |
| BIOS firmware | Yes | |
| UEFI firmware | Yes | |
| Standard VMDK disks | Yes | Converted to qcow2 or raw. |
| Thin-provisioned disks | Yes | |
| Thick-provisioned disks | Yes | |
| RDM / shared disks | No | Blocked at preflight. |
| Snapshots | Warning | Export includes current snapshot chain. Operator must review. |
| VMware Tools running | Preferred | Required for quiesced snapshots. Warning if not running. |

## Target platforms (Proxmox VE)

| Feature | Supported | Notes |
|---------|-----------|-------|
| Proxmox VE cluster | Design complete | Real connector via PVE API (Phase 3 interface ready). Mock adapter active. |
| Single Proxmox node | Design complete | Same adapter interface. |
| LVM storage | Yes | local-lvm type. |
| NFS storage | Yes | Shared storage supported. |
| qcow2 disk format | Yes | Default target format. |
| raw disk format | Yes | Optional. |
| VLAN-aware bridges | Yes | vmbr with VLAN tagging. |
| Non-VLAN bridges | Yes | |
| Isolated validation network | Yes | Target boots isolated before cutover. |

## Storage mappings

| Source | Target | Status |
|--------|--------|--------|
| VMDK (local datastore) | qcow2 (local-lvm) | Yes |
| VMDK (shared datastore) | qcow2 (NFS) | Yes |
| RDM | Any | No - blocked |

## Network mappings

| Source | Target | Status |
|--------|--------|--------|
| vmxnet3 NIC | VirtIO NIC | Yes (driver swap in remediation) |
| E1000 NIC | VirtIO NIC | Conditional |
| VLAN port group | VLAN-aware bridge | Yes |
| Standard port group | Non-VLAN bridge | Yes |

## Migration features

| Feature | Status | Phase |
|---------|--------|-------|
| Drag-and-drop plan creation | Implemented | 0-1 |
| Preflight compatibility checks (8) | Implemented | 4 |
| Cold migration execution | Implemented (mock) | 6 |
| Disk conversion (vmdk to qcow2) | Implemented (mock) | 5 |
| Isolated target boot | Implemented | 6 |
| Validation (power, disks, drivers, network, TCP) | Implemented | 8 |
| Production cutover with interlocks | Implemented | 8 |
| Rollback | Implemented | 8 |
| Source retention window (14 days) | Implemented | 8 |
| Migration reports | Implemented | 10 |
| RBAC (6 roles) | Implemented | 9 |
| Audit trail | Implemented | All |
| Live/zero-downtime migration | Not supported | Roadmap 12A |
| Hyper-V source | Preview only | Roadmap 12B |
| Incremental CBT copy | Not supported | Roadmap 12A |

## Out of scope (MVP)

- Live migration / vMotion equivalent
- Encrypted VMs, vTPM, Fault Tolerance
- NSX overlays, SR-IOV, GPU/PCI passthrough
- Automatic source deletion
- Production migration without approved maintenance window