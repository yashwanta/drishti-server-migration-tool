# Test Evidence Index

**Version:** 0.1
**Date:** 2026-07-24

## Automated test results

### Backend unit tests (21 tests, all pass)

| Package | Tests | Status | Coverage |
|---------|-------|--------|----------|
| internal/api | 13 | PASS | 29.2% |
| internal/config | 4 | PASS | 81.8% |
| internal/logging | 4 | PASS | 80.0% |
| internal/mock | 4 | PASS | 40.3% |
| internal/server | 4 | PASS | 61.4% |
| internal/store | 3 | PASS | 73.7% |

Test detail:
- TestListConnections, TestCreateConnection, TestCreateConnectionRejectsBadKind
- TestCreateConnectionRejectsMissingFields, TestCreateConnectionDuplicate
- TestDeleteConnection, TestTestConnection, TestGetInventory, TestGetInventoryNotFound
- TestCreatePlanRejectsMissingFields, TestCreatePlanAlwaysDraft, TestPlanLifecycleGetDelete
- TestPlanCreateWritesAudit
- TestLoadDefaults, TestLoadRequiresDBInLabMode, TestLoadRejectsInvalidMode, TestParseSeconds
- TestSecretsAreRedacted, TestLevelFiltering, TestStructuredFields, TestSafeStringValue
- TestConnectionsShape, TestVMwareInventory, TestProxmoxInventory, TestUnknownConnection
- TestHealthEndpoint, TestLivezEndpoint, TestRecoveryCatchesPanic, TestCorsSetsHeaders
- TestLoadEmbeddedMigrations, TestUpIdempotent, TestUpStopsOnError

### Worker unit tests (7 tests, all pass)

| Package | Tests | Status |
|---------|-------|--------|
| internal/cmdsafelist | 3 | PASS |
| internal/config | 1 | PASS |
| internal/runner | 3 | PASS |

Test detail:
- TestResolveKnown, TestResolveUnknown, TestAllowed
- TestLoadDefaults
- TestRunRejectsUnknownCommand, TestRunRejectsMetachars, TestValidateArg

### Frontend

| Check | Status |
|-------|--------|
| TypeScript strict typecheck (tsc --noEmit) | PASS |
| Production build (vite build) | PASS |
| Bundle size | 166 KB JS (52 KB gzip), 5 KB CSS |

### Verification script (8 steps, all pass)

| Step | Status |
|------|--------|
| Backend build | PASS |
| Backend go vet | PASS |
| Backend tests | PASS |
| Worker build | PASS |
| Worker go vet | PASS |
| Worker tests | PASS |
| Frontend typecheck | PASS |
| Frontend build | PASS |

## End-to-end test evidence

### Full migration lifecycle + rollback drill

See [phase-11-rollback-drill.md](./phase-11-rollback-drill.md).

Result: **PASSED** - plan, preflight (8/8), approve, execute (6/9 succeeded),
validate (passed), cutover (success, 14-day retention), rollback (success).

### Audit trail verified

All 8 lifecycle events recorded with timestamps, actors, actions, and results.

## Manual tests (cannot be automated in mock mode)

These require real infrastructure and are documented for future lab verification:

- [ ] Real vCenter inventory matches normalized model
- [ ] Real Proxmox inventory matches normalized model
- [ ] Real VMDK export and qemu-img conversion produces bootable disk
- [ ] Real VirtIO driver injection boots Windows guest
- [ ] Real cutover does not cause IP conflict on production network
- [ ] Real rollback restores source within maintenance window

## How to reproduce

```
cd backend && go test ./... -v -cover
cd worker && go test ./... -v
cd frontend && npx tsc --noEmit && npm run build
.\scripts\verify.ps1
```