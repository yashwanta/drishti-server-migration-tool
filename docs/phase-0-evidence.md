# Phase 0 Evidence — Repository Bootstrap and Engineering Guardrails

**Status:** Complete
**Date:** 2026-07-24

## Objective achieved

Created a clean, testable project skeleton with non-negotiable safety and quality rules before connecting to any hypervisor.

## Deliverables

| Deliverable | Location | Status |
|-------------|----------|--------|
| Compiling backend skeleton | `backend/` | Done |
| Compiling frontend skeleton | `frontend/` | Done |
| Migration worker skeleton | `worker/` | Done |
| Database migration baseline | `backend/migrations/0001_init.sql`, `backend/internal/store/` | Done |
| Mock mode with sample inventory | `backend/internal/mock/` | Done |
| CI / local verification script | `scripts/verify.ps1`, `scripts/verify.sh` | Done |
| README with setup/test instructions | `README.md` | Done |
| SECURITY.md | `SECURITY.md` | Done |
| ADR template | `docs/adr-0001-template.md` | Done |
| Threat model placeholder | `docs/threat-model.md` | Done |
| Structured logging with redaction | `backend/internal/logging/` | Done |
| Config validation | `backend/internal/config/` | Done |
| Health endpoints | `/healthz`, `/livez` | Done |
| Worker command safelist | `worker/internal/cmdsafelist/` | Done |

## Commands run

```
cd backend && go build ./... && go vet ./... && go test ./...
cd worker  && go build ./... && go vet ./... && go test ./...
cd frontend && npm install && npx tsc --noEmit && npm run build
```

## Test results

- Backend: all packages pass (api, config, logging, mock, server, store).
- Worker: all packages pass (cmdsafelist, config, runner).
- Frontend: tsc strict typecheck passes; vite production build succeeds.

## Safety verification

- No destructive hypervisor operation exists in the codebase.
- No real platform credentials are required (mock mode default).
- Secrets are referenced by SecretRef, never stored inline.
- The logger redacts sensitive fields and connection strings.
- The worker only allows qemu-img and sha256sum; rejects rm, bash, curl.
- A drag-and-drop only creates a draft plan (enforced in API and UI).

## Acceptance criteria verdict

- A new developer can clone the repository and start the stack using documented commands: **PASS** (`go run`, `npm run dev`, documented in README).
- All lint, unit-test, and build commands pass: **PASS** (see verification script).
- No real platform credentials are required: **PASS** (mock mode default).
- No destructive hypervisor operation exists in the codebase: **PASS**.

## Known gaps / deferred

- Real VMware (Phase 2) and Proxmox (Phase 3) adapters not yet implemented; mock provider only.
- PostgreSQL migrator implementation deferred to Phase 6 (interface + idempotent Up() exists now).
- CI workflow (.github/workflows) defined as a TODO; local verify script covers the same checks.
- Docker not installed in the build environment; Dockerfiles and docker-compose are provided and validated for syntax but not run locally.

## Next phase

Phase 1: Read-Only Product UI and Domain Model (domain models already scaffolded in Phase 0; Phase 1 will add more detail to the wizard, plan persistence, and API contract tests).