# Proxmox Create and Import Lab Runbook

This runbook applies only to a disposable Proxmox VE lab target. Item 1 is not
wired into the runtime adapter factory until item 2. No production-mode target
mutation is enabled.

## Implemented target boundary

The real target adapter can reserve a VMID, create a protected stopped VM,
import a raw or qcow2 disk, attach it to a deterministic SCSI slot, isolate and
start the target, gracefully stop it, and read its state.

Creation enforces:

- a target-role Proxmox connection;
- lab or live mode plus `DRISHTI_ENABLE_PLATFORM_MUTATION=true`;
- an endpoint that is not hostname-, IP-, or DNS-alias-equivalent to an entry
  in `DRISHTI_MUTATION_DENYLIST`;
- an online target node;
- an explicitly allowlisted isolation bridge;
- `protection=1`, `onboot=0`, and `link_down=1` on the target NIC;
- a DRISHTI idempotency marker bound to the create request;
- a stopped state after the Proxmox create task completes.

Production mode rejects the mutation flag during configuration loading and is
also rejected by the adapter mutation policy.

## Shared import path contract

Proxmox `import-from` reads a path visible to the target node. Configure an
absolute POSIX path such as:

```text
DRISHTI_PROXMOX_IMPORT_ROOT=/mnt/drishti-import
```

The same storage must be mounted at that exact path on the worker and target
node. The worker now performs real qemu-img conversion beneath its configured
workspace and produces a checksum manifest. End-to-end VMware export through
Proxmox import still requires an operator-provided disposable lab environment.

Imports refuse paths outside the root, traversal, commas/control characters,
unsupported formats/controllers, occupied slots, running targets, inactive
storage, and missing idempotency information. The marker binds the request to
the import path, storage, format, size, and SCSI slot.

## Isolation configuration

Explicitly list validation-only bridges:

```text
DRISHTI_PROXMOX_ISOLATED_BRIDGES=vmbr-lab,vmbr-quarantine
```

Creating or configuring a VM on any other bridge is refused. Starting a target
also rereads every NIC and verifies both `link_down=1` and membership in this
allowlist. Non-isolated network activation is not implemented in this phase.

## Disposable lab qualification

1. Use a dedicated lab node, storage, VMID range, import share, and isolation
   bridge with no production uplink.
2. Add all protected/production Proxmox endpoints to the mutation denylist.
3. Use an API token limited to the required lab node, storage, and VM pool.
4. Confirm the proposed VMID is unused.
5. Create the VM and verify it is protected, stopped, and link-down isolated.
6. Place a disposable converted image beneath the shared import root.
7. Import it into the deterministic SCSI slot and verify the Proxmox task,
   resulting volume, boot order, and idempotency marker.
8. Retry create/import and verify no duplicate VM or disk is created.
9. Start only while every NIC remains link-down on an allowlisted bridge.
10. Gracefully stop the target and disable the mutation flag.

Do not use production nodes, storage, bridges, VMIDs, or endpoints. No lab
integration test is run until the operator explicitly provides disposable lab
resources.
