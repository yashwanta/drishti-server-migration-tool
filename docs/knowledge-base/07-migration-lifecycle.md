# Migration Lifecycle

The complete migration workflow from plan to rollback. Every step is gated by
safety interlocks.

## The 7-step lifecycle

### 1. Create a plan (drag and drop)
Drag a powered-off VM from a source panel onto a Proxmox target node. This
opens the wizard and creates a **draft** plan. Nothing is migrated.

### 2. Preflight
Click **Preflight** on the plan. Eight checks run:
- Power state (must be off)
- Target capacity (CPU, memory)
- Storage availability (each disk fits)
- Unsupported features (blocks RDM, passthrough)
- Firmware compatibility
- VMware Tools status
- Snapshot presence (warning)
- Target node online

If any check blocks, fix the issue before proceeding.

### 3. Approve
Click **Approve**. An authorized operator signs off on the plan. This is
required before execution.

### 4. Execute
Click **Execute**. The state machine runs:
1. Source VM powered off (if not already)
2. Each disk exported with SHA256 checksum
3. Each disk converted (vmdk to qcow2)
4. Target VM created on isolated network
5. Disks attached to target
6. Target booted on validation network (NOT production)

The target is now running but isolated. The source is off and retained.

### 5. Validate
Click **Validate**. Checks run against the target:
- Target powered on
- Disk count matches
- VirtIO drivers recognized
- Network reachable on isolated bridge
- Required TCP ports open

If validation fails, do NOT cutover.

### 6. Cutover
Click **Cutover**. The safety sequence runs:
1. Confirm source is powered off (refuses if not)
2. Confirm validation passed (refuses if not)
3. Move target to production network
4. Verify production health
5. Start 14-day source retention window

The source stays registered and retained for rollback.

### 7. Rollback (if needed)
Click **Rollback** at any time during the retention window:
1. Target isolated to validation network
2. Target powered off
3. Confirm no duplicate identity
4. Source VM powered on
5. Source health validated

## States

| Plan state | Meaning |
|-----------|---------|
| draft | Created, awaiting preflight |
| preflight | Checks have run |
| approved | Operator approved |
| rejected | Preflight blocked or declined |

| Job state | Meaning |
|-----------|---------|
| pending | Created or waiting for operator |
| running | Execution in progress |
| succeeded | Cutover completed |
| failed | A step failed |
| rolled_back | Rollback completed |

## Safety at every step

- **Never** can cutover happen while source is on
- **Never** is the source deleted
- **Never** does a drag execute anything
- **Always** is the full sequence audited