# Architecture

## Overview

DRISHTI HyperShift is a monorepo with three runtime components plus shared documentation.

```
        +-------------------+        +------------------+
        |   React/TS Web UI | <----> |  Go API Server   |
        |  (drag & drop,    |  HTTP  |  (orchestrator)  |
        |   wizard, plans)  |        +--------+---------+
        +-------------------+                 |
                                              | job dispatch
                                              v
                                   +----------+----------+
                                   | Migration Worker    |
                                   | (disk transfer,     |
                                   |  conversion)        |
                                   +---------------------+
        +-------------------+                 ^            +-------------------+
        | VMware Adapter    |                 |            | Proxmox Adapter   |
        | (read-only until  | <--------------+----------> | (read-only until  |
        |  Phase 6)         |   normalized domain model   |  Phase 6)         |
        +-------------------+                             +-------------------+
```

## Components

### Backend (`backend/`)

Module `github.com/drishti/hypershift`. Go 1.25.

- `cmd/hypershift/main.go` — entrypoint, wires config + logger + handlers + server.
- `internal/config/` — typed, validated env-based config. `DRISHTI_MODE` = `mock` (default), `lab`, or `production`. `lab`/`production` require `DRISHTI_DB_URL`.
- `internal/logging/` — structured JSON logger with secret redaction (passwords, tokens, URLs with credentials).
- `internal/domain/` — normalized, hypervisor-agnostic models (Connection, VM, Disk, NIC, Plan, Job, AuditEvent, etc.).
- `internal/mock/` — development-only provider returning realistic VMware + Proxmox inventory without contacting real systems.
- `internal/api/` — REST handlers under `/api/v1` (connections, inventory, plans, audit) + an in-memory plan/audit store for Phase 0/1.
- `internal/server/` — HTTP server, health (`/healthz`, `/livez`), CORS, panic recovery, graceful shutdown.
- `internal/store/` — embeds SQL migrations; `Up()` applies them idempotently via a Migrator interface.
- `migrations/0001_init.sql` — Postgres schema baseline (connections, plans, jobs, job_steps, audit_events).

### Frontend (`frontend/`)

React 18 + TypeScript (strict), Vite, no UI framework dependency.

- `src/types.ts` — TypeScript types mirroring the Go domain model.
- `src/api.ts` — typed REST client.
- `src/App.tsx` — inventory dashboard: source panel (VMware), target panel (Proxmox), draft-plans panel.
- `src/components/VmCard.tsx` — draggable VM card.
- `src/components/NodeCard.tsx` — Proxmox drop target.
- `src/components/PlanWizard.tsx` — 4-step migration-plan wizard. **Submitting only creates a draft plan; it never migrates.**

### Worker (`worker/`)

Module `github.com/drishti/hypershift-worker`. Go 1.25.

- `cmd/worker/main.go` — worker entrypoint with `/healthz`.
- `internal/cmdsafelist/` — fixed allowlist of executables (qemu-img, sha256sum). `rm`, `bash`, `curl`, etc. are rejected.
- `internal/runner/` — executes allowlisted binaries with validated argument arrays. No shell, no metacharacters.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health (mode, uptime) |
| GET | `/livez` | Liveness probe |
| GET | `/api/v1/connections` | List platform connections |
| GET | `/api/v1/connections/{id}` | Get a connection |
| GET | `/api/v1/connections/{id}/inventory` | Normalized inventory |
| GET | `/api/v1/plans` | List migration plans |
| POST | `/api/v1/plans` | Create a **draft** plan (never executes) |
| GET | `/api/v1/plans/{id}` | Get a plan |
| DELETE | `/api/v1/plans/{id}` | Remove a draft plan |
| GET | `/api/v1/audit` | List audit events |

## Modes

- **mock** (default): no platform connections; uses the in-memory mock provider. Safe for development.
- **lab**: may connect to explicitly configured lab endpoints; requires `DRISHTI_DB_URL`.
- **production**: requires all safety interlocks; requires `DRISHTI_DB_URL`.