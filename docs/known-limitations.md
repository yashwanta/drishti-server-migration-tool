# Known Limitations and Blocked Features

**Version:** 0.1
**Date:** 2026-07-24

## Current limitations

### Mock mode only
All platform operations use **mock adapters** that simulate the migration
lifecycle. Real VMware (govmomi) and Proxmox (PVE API) connectors satisfy the
same adapter interfaces but require actual credentials and infrastructure to
build, test, and activate.

**Impact:** The full workflow (plan, preflight, execute, validate, cutover,
rollback) runs end-to-end but does not touch real VMs. This is intentional for
safe development and pilot rehearsal.

### No authentication
There is no login or session management. RBAC roles and permission checks are
implemented in code but not enforced via middleware (Phase 9 code complete,
enforcement pending real auth provider).

**Impact:** Anyone with network access to the UI can create plans and run the
migration workflow. Restrict network access accordingly.

### No persistent storage
Plans, jobs, and audit events are stored **in-memory**. Restarting the backend
loses all state. The Postgres schema baseline exists but the migrator
implementation is not wired in production mode yet.

**Impact:** Not suitable for long-running migrations or audit retention
requirements. Restart-safe for mock/testing only.

### Windows guest remediation is advisory
VirtIO driver injection for Windows is modeled but not executed. Windows VMs
are flagged for manual review rather than automatically remediated.

**Impact:** Windows migrations require manual VirtIO driver preparation before
cutover.

### No real disk transfer
Disk export and conversion use placeholder data with checksums, not real VMDK
files. The worker safelist and converter interface are production-ready but the
actual qemu-img invocation runs against mock data.

**Impact:** Disk sizes and transfer times are not representative of real
migrations.

## Blocked features (by design)

These are intentionally blocked and will remain so until the roadmap phase
completes:

| Feature | Blocked because | Roadmap |
|---------|----------------|---------|
| Live migration | Requires CBT research; not proven safe | 12A |
| Source auto-deletion | Safety invariant: never auto-delete source | Never automatic |
| Encrypted VM migration | No proven decryption/transfer path | Future |
| Hyper-V migration | Adapter not built | 12B |
| GPU/PCI passthrough | Unsupported in Proxmox target model | Future |
| NSX overlay migration | No equivalent in Proxmox | Future |

## Safety interlocks that cannot be bypassed

- **Cutover refuses if source is not confirmed powered off.**
- **Cutover refuses if validation has not passed.**
- **Network restore refuses if VirtIO drivers are not ready.**
- **Drag-and-drop never executes a migration.**
- **Worker only runs allowlisted commands (qemu-img, sha256sum).**
- **Source VM is never deleted or unregistered.**