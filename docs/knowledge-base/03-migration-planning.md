# Drag and Drop Migration Planning

This is the core workflow. **Dragging a VM onto a target node never migrates
it.** It opens a migration plan wizard so you can review and configure every
detail before anything happens.

## Step-by-step

1. **Find the source VM** in the left panel. It must be powered off for cold
   migration. Cards with a grey "off" badge are ready.
2. **Drag** the VM card onto a Proxmox node in the middle panel. The target
   node highlights when it accepts the drop.
3. The **Migration Plan Wizard** opens with four steps.

### Wizard Step 1: Source Summary

Shows the VM details read from inventory: name, guest OS, power state, CPU,
memory, disk count, NIC count, and snapshot count. Review for accuracy.

### Wizard Step 2: Target Configuration

Configure the destination VM:
- **Target VM Name** - defaults to the source name.
- **CPU** - defaults to the source vCPU count.
- **Memory** - defaults to the source memory.
- **Firmware** - BIOS or UEFI (defaults to source firmware).

### Wizard Step 3: Mappings

Map source resources to target resources:
- **Storage mapping** - each source disk is assigned to a Proxmox storage pool.
- **Network mapping** - each source NIC is assigned to a Proxmox bridge.
- **Target VLAN ID** - optional VLAN tag for the target NIC.
- **Target Disk Format** - `qcow2` (default) or `raw`.

No guesswork is done silently. Every mapping is explicit and operator-controlled.

### Wizard Step 4: Review

A full summary of the plan. A red warning box reminds you:

> Submitting creates a DRAFT plan only. No migration, disk copy, power change,
> or deletion will occur. The source VM remains registered in VMware for rollback.

Click **Create Draft Plan**. The plan appears in the right panel and the audit
log records the action.

## What happens next

The draft plan is saved with status `draft`. Future phases add:
- **Preflight checks** (Phase 4) - compatibility, capacity, firmware, disk.
- **Approval gate** - an authorized approver must sign off.
- **Execution** (Phase 6) - the actual cold migration with idempotent retries.

None of that exists yet. Today, the drag-and-drop and wizard are safe to use
freely because they cannot execute anything.

## Draft plan states

| Status | Meaning |
|--------|---------|
| `draft` | Just created, awaiting preflight. This is the only state the wizard produces. |
| `preflight` | Checks are running (Phase 4+). |
| `approved` | An approver signed off (Phase 6+). |
| `rejected` | Preflight failed or approver declined. |
| `archived` | Superseded or withdrawn. |

## Tips

- You can create multiple draft plans and compare them in the right panel.
- Removing a draft plan (via the API) only works while it is still `draft`.
- The Refresh button reloads all connections, inventory, and plans.