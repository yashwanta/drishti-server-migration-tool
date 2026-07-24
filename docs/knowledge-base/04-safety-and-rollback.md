# Safety and Rollback

Safety is the foundation of DRISHTI HyperShift. Every design decision enforces
these non-negotiable invariants.

## The eight invariants

1. **Never delete or unregister the source VM automatically.**
   The original VMware VM stays registered and retained. After cutover it is
   powered off and held for rollback until an administrator explicitly approves
   deletion after the retention period.

2. **Never run source and destination simultaneously**
   on the same production network when hostname, IP, MAC, or application
   identity is preserved. Duplicate identity causes conflicts and outages.

3. **A drag never starts a migration.**
   It only opens a draft plan requiring preflight, explicit mapping, and approval.

4. **Never log secrets.**
   Passwords, API tokens, session cookies, private keys, BitLocker recovery
   keys, and guest credentials are redacted before they reach any log.

5. **No destructive operations against production in early phases.**
   Testing uses disposable lab VMs and isolated networks.

6. **Never claim success solely because the target powered on.**
   Validation checks disks, networking, guest health, and application checks.

7. **Every mutating operation is idempotent or safely resumable,**
   and every state transition is recorded in the audit log.

8. **Rollback is always available**
   until an administrator closes the retention window and authorizes cleanup.

## How the UI enforces safety

- **The wizard only creates drafts.** The submit button is labeled "Create
  Draft Plan" and the API forces `status: draft` server-side.
- **Safety banner** is always visible at the top of the dashboard.
- **No power controls** exist in the UI for source VMs. You cannot accidentally
  power off a VM through the tool in Phase 0-1.
- **No delete buttons** for VMs, disks, or datastores exist anywhere.

## Rollback model

After a migration cutover (Phase 8+):

1. The Proxmox target is isolated and powered off.
2. The system confirms no duplicate production identity can exist.
3. The retained VMware source is powered back on.
4. Source health is validated and the result is recorded.

The source VM is **never** cleaned up automatically. Source cleanup is a
separate, explicitly approved, audited action that happens only after the
retention window closes.

## Audit trail

Every action is recorded:

- `connection.create` / `connection.delete`
- `plan.create`
- (future) `plan.preflight`, `plan.approve`, `job.start`, `job.rollback`

View the audit log via `GET /api/v1/audit` or the API.

## What to do if something looks wrong

- **Stop.** Do not approve or execute anything.
- **Read the audit log** to see what actions were taken.
- **The source VM is untouched** in Phase 0-1. No data can be lost through the UI.
- See [Troubleshooting](./05-troubleshooting.md) for common issues.