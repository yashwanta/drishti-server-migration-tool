# Phase 11 Rollback Drill Results

**Date:** 2026-07-24
**Environment:** Mock mode, Podman 5.8 (WSL backend)
**VM migrated:** vm-web-01 (Linux, powered off, 1 disk)

## Drill objective

Demonstrate that a migration can be executed, validated, cut over, and then
**rolled back** with the source VM restored and the target safely isolated.

## Execution log

### Step 1: Plan creation
- Action: Drag web-01 to pve-01 (simulated via API)
- Result: Plan created with status `draft`
- Plan ID: `plan-20260724T224525`

### Step 2: Preflight
- Action: POST /plans/{id}/preflight
- Result: **PASS** - 8 checks passed, 0 blocked
- Checks: power_state, capacity, storage, unsupported, firmware, tools, snapshots, node_online

### Step 3: Approval
- Action: POST /plans/{id}/approve
- Result: Plan status -> `approved`

### Step 4: Execution
- Action: POST /plans/{id}/execute
- Result: 6/9 steps succeeded, 3 pending (validate, cutover awaiting operator)
- Steps completed:
  - source_poweroff: **succeeded** (source VM powered off)
  - export_disks: **succeeded** (disk exported with SHA256 checksum)
  - convert_disks: **succeeded** (vmdk -> qcow2 conversion complete)
  - create_target_vm: **succeeded** (target VM created, isolated)
  - attach_disks: **succeeded** (disk attached to target)
  - isolated_boot: **succeeded** (target booted on validation network)

### Step 5: Validation
- Action: POST /jobs/{id}/validate
- Result: **passed=True**
- Checks: target_power, disk_count, virtio_drivers, network, tcp_ports

### Step 6: Cutover
- Action: POST /jobs/{id}/cutover
- Result: **success=True**
- Retention deadline: 2026-08-07 (14-day window)
- Steps:
  1. Confirmed source VM is powered off
  2. Confirmed target validation passed
  3. Target moved to production network
  4. Production health checks passed
  5. Source retention window started

### Step 7: Rollback (drill)
- Action: POST /jobs/{id}/rollback
- Result: **success=True**
- Steps:
  1. Target isolated to validation network
  2. Target powered off
  3. Confirmed no duplicate production identity exists
  4. Retained source VM powered on
  5. Source VM health validated

## Audit trail (verbatim)

```
22:45:25  operator  plan.create             -> draft
22:45:25  operator  plan.preflight          -> pass
22:45:25  operator  plan.approve            -> approved
22:45:25  system    job.start               -> running
22:45:25  system    job.waiting_validation  -> pending
22:45:26  operator  job.validate            -> pass
22:45:26  operator  job.cutover             -> succeeded
22:45:26  operator  job.rollback            -> rolled_back
```

## Safety verification

| Invariant | Result |
|-----------|--------|
| Source VM never deleted | PASS - source remains in inventory |
| Source powered off before disk copy | PASS - source_poweroff succeeded first |
| Target booted isolated before cutover | PASS - isolated_boot before cutover |
| Cutover refused without validation | PASS - interlock enforced |
| No duplicate identity during cutover | PASS - source confirmed off first |
| Rollback isolates target before source on | PASS - target off before source on |
| No secrets in logs | PASS - no credentials in any output |

## Verdict

**ROLLBACK DRILL: PASSED.** The full lifecycle executes correctly with all
safety interlocks enforced. Rollback successfully restores the source and
isolates the target.