# Pilot Checklist and Go/No-Go Criteria

**Version:** 0.1
**Date:** 2026-07-24

## Pilot scope

Controlled pilot using **mock mode** to validate the full migration workflow
end-to-end before real infrastructure is connected. No real VMs are touched.

## Pre-pilot checklist

### Environment
- [ ] Podman 5.x running (`podman machine list` shows running)
- [ ] All three images built (`podman images | grep hypershift`)
- [ ] Pod started (`.\scripts\podman-run.ps1 up`)
- [ ] Backend health responds (`http://localhost:8180/healthz` -> ok)
- [ ] Worker health responds (`http://localhost:8090/healthz` -> ok)
- [ ] Frontend loads (`http://localhost:5173` shows dashboard)

### Security
- [ ] No real credentials in environment or config
- [ ] `.env` is not committed (check `git status`)
- [ ] No secrets in backend logs (`podman logs hs-backend` -> no passwords/tokens)
- [ ] Worker safelist enforced (no `rm`, `bash`, `curl` allowed)

### Inventory
- [ ] VMware source connection present with sample VMs
- [ ] Proxmox target connection present with sample node
- [ ] At least one powered-off Linux VM available (web-01)
- [ ] At least one unsupported VM present (legacy-rdm) for blocking test

## Go/No-Go criteria

### GO requires ALL of the following

| # | Criterion | How to verify | Pass? |
|---|-----------|---------------|-------|
| 1 | Drag-and-drop creates draft plan only | Drag VM to node, check status = draft | [ ] |
| 2 | Preflight passes for compliant VM | Click Preflight on web-01, pass=True | [ ] |
| 3 | Preflight blocks unsupported VM | Click Preflight on legacy-rdm, blocked | [ ] |
| 4 | Execute powers off source | Check source_poweroff step = succeeded | [ ] |
| 5 | Disks exported and converted | export_disks + convert_disks = succeeded | [ ] |
| 6 | Target created and booted isolated | create_target_vm + isolated_boot = succeeded | [ ] |
| 7 | Validation passes for Linux VM | Validate -> passed=True | [ ] |
| 8 | Cutover succeeds with retention window | Cutover -> success=True, retention set | [ ] |
| 9 | Cutover refuses if validation fails | Force-fail validation, cutover returns 409 | [ ] |
| 10 | Rollback isolates target + restores source | Rollback -> success=True | [ ] |
| 11 | Audit trail records all actions | GET /api/v1/audit shows full sequence | [ ] |
| 12 | Migration report generates | Report endpoint returns structured data | [ ] |
| 13 | No secrets in any log output | Inspect all container logs | [ ] |
| 14 | Source VM never deleted | Source remains in inventory after all operations | [ ] |

### NO-GO if ANY of the following occur

- Any step marked "succeeded" without actual completion evidence
- Cutover succeeds while source is still powered on
- Rollback fails to restore source
- Any secret appears in logs, API responses, or reports
- Duplicate target VMs created on retry
- Worker executes a non-allowlisted command
- Application crashes or hangs during the workflow

## Pilot execution sequence

1. Start the stack: `.\scripts\podman-run.ps1 up`
2. Open `http://localhost:5173`
3. Run the rollback drill (see [rollback drill results](./phase-11-rollback-drill.md))
4. Verify each go/no-go criterion
5. Capture audit trail: `Invoke-RestMethod http://localhost:8180/api/v1/audit`
6. Generate report for each migrated VM
7. Document results and verdict