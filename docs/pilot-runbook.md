# Pilot Runbook

**Version:** 0.1
**Date:** 2026-07-24

## Purpose

Step-by-step instructions for running a controlled pilot of DRISHTI HyperShift
in mock mode. This runbook covers the full migration lifecycle including
rollback.

## Prerequisites

- Podman 5.x installed and machine running
- Go 1.25+, Node.js 20+, npm 10+ (for native builds)
- This repository cloned locally

## Step 1: Build and start the stack

```
cd "Drishti-ServerMigration tool"
.\scripts\podman-run.ps1 build
.\scripts\podman-run.ps1 up
```

Verify:
- `http://localhost:8180/healthz` returns `{"status":"ok"}`
- `http://localhost:8090/healthz` returns `{"status":"ok"}`
- `http://localhost:5173` shows the dashboard

## Step 2: Verify inventory

1. Left panel shows "vCenter Lab" with 3 VMs:
   - web-01 (Linux, off) - migration candidate
   - db-01 (Windows, off) - has snapshot
   - legacy-rdm (Linux, on) - unsupported (RDM)
2. Middle panel shows "Proxmox Lab" with node pve-01

## Step 3: Create a migration plan

1. Drag **web-01** onto **pve-01**
2. The wizard opens
3. Click **Next** through Source, Target, Mappings, Review
4. Click **Create Draft Plan**
5. The plan appears in the right panel as `draft`

## Step 4: Run preflight

1. Click **Preflight** on the plan
2. Result should show `pass=True` with 8 checks
3. If any check fails, do not proceed - review the blocked reasons

## Step 5: Approve and execute

1. Click **Approve** (status becomes `approved`)
2. Click **Execute**
3. Wait for execution to complete
4. Verify steps in the output:
   - source_poweroff: succeeded
   - export_disks: succeeded
   - convert_disks: succeeded
   - create_target_vm: succeeded
   - attach_disks: succeeded
   - isolated_boot: succeeded

## Step 6: Validate the target

1. Click **Validate**
2. Result should show `passed=True`
3. If validation fails, do NOT cutover - investigate

## Step 7: Cutover

1. Click **Cutover**
2. Result should show `success=True` with retention deadline
3. The cutover enforces: source is off, validation passed, no duplicate identity

## Step 8: Rollback (drill)

1. Click **Rollback**
2. Confirm the prompt
3. Result should show `success=True`
4. Verify rollback steps:
   - Target isolated
   - Target powered off
   - Source powered on
   - Source health validated

## Step 9: Verify the audit trail

```
Invoke-RestMethod http://localhost:8180/api/v1/audit
```

All actions should be recorded with timestamps.

## Step 10: Generate a report

1. Click **Report** on the plan
2. Review the structured migration report

## Emergency procedures

### If cutover fails
- The cutover interlock prevents partial cutover
- Click **Rollback** to restore the source
- The source was never modified - it is safe

### If the application crashes
- All state is in-memory; restarting resets to initial mock data
- No real VMs were touched in mock mode
- Restart: `.\scripts\podman-run.ps1 restart`

### If rollback fails
- In mock mode this should not happen
- In real mode: the source VM was never deleted; manually power it on via vCenter

## Teardown

```
.\scripts\podman-run.ps1 down
```