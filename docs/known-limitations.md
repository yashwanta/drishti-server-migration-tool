# Known Limitations and Blocked Features

**Version:** 0.1
**Date:** 2026-08-06

## Current limitations

### Real-mode execution remains locked
Lab, live, and production modes select the real VMware and Proxmox adapters,
PostgreSQL repositories, and worker conversion client. Execute, cutover, and
rollback nevertheless return `501 execution_disabled` after authentication and
authorization, pending a separate lab qualification and readiness review.

**Impact:** The full workflow (plan, preflight, execute, validate, cutover,
rollback) runs end-to-end but does not touch real VMs. This is intentional for
safe development and pilot rehearsal.

### Authentication users and sessions are process-local
Session authentication and route-level RBAC are enforced. User definitions are
loaded from a protected bcrypt-hash file and active server-side sessions are
process-local, so a backend restart signs every operator out.

**Impact:** Deployments must provision the users file securely and operators
must sign in again after a backend restart. There is no external identity
provider, MFA, or distributed session store yet.

### Connection registry and direct credentials remain process-local
Plans, jobs, ordered job steps, and audit events use PostgreSQL in lab, live,
and production modes. Platform connection registration and direct credentials
remain process-local; direct passwords are deliberately never persisted.

**Impact:** Connections must be re-added after restart. Durable migration state
remains available once the connections and ephemeral credentials are restored.

### Windows guest remediation is advisory
VirtIO driver injection for Windows is modeled but not executed. Windows VMs
are flagged for manual review rather than automatically remediated.

**Impact:** Windows migrations require manual VirtIO driver preparation before
cutover.

### Real conversion is not yet qualified with an exported lab VM
The worker performs real `qemu-img` VMDK-to-raw/qcow2 conversion, publishes the
output atomically, and records checksum evidence. It has passed a generated
VMDK fixture test, but no operator-provided disposable VMware lab VM has yet
been exported and migrated end-to-end.

**Impact:** Bootability and transfer timing for an actual VMware-exported disk
remain a required lab qualification before execution can be considered.

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
