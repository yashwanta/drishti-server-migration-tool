# DRISHTI HyperShift

A self-hosted **VMware-to-Proxmox migration orchestrator**. Connect to vCenter/ESXi and Proxmox VE, see both inventories in one interface, and drag a source VM onto a target Proxmox node to open a controlled, auditable migration plan.

> **Drag-and-drop never moves or deletes a VM.** It opens a migration plan with preflight, mappings, approvals, and an auditable workflow. The original VMware VM remains registered and retained for rollback until an administrator explicitly approves cleanup.

See `DRISHTI_HyperShift_PROJECT_PLAN.md` for the full project master and `docs/architecture.md` for the design.

## Current status: Phase 0

This is the repository bootstrap. It includes:

- Compiling **Go backend** (API/orchestrator) with config, structured logging, health checks, mock providers, domain models, REST API, and DB migration baseline.
- Compiling **React + TypeScript frontend** with an inventory dashboard, drag-and-drop, and a migration-plan wizard that only creates drafts.
- Compiling **Go migration worker** skeleton with a hardened command safelist.
- Tests across config, logging (secret redaction), mock inventory, API handlers, migrations, server, and worker.
- `SECURITY.md`, `CONTRIBUTING.md`, ADR template, and architecture docs.

No real platform credentials, no destructive operations, no real migration execution exist in this phase.

## Prerequisites

- **Go** 1.25+
- **Node.js** 20+ and **npm** 10+
- *(Optional)* **Docker** + Docker Compose for the containerized stack
- *(Optional, Phase 2+)* **PostgreSQL** for non-mock deployments

Verify versions:

```powershell
go version; node --version; npm --version
```

## Quick start (mock mode, no Docker)

The app defaults to **mock mode** — no real infrastructure needed.

**1. Backend** (serves API on `:8080`):

```powershell
cd backend
go run ./cmd/hypershift
```

**2. Frontend** (dev server on `:5173`, proxies `/api` to `:8080`):

```powershell
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`. You'll see sample VMware VMs on the left and a Proxmox node on the right. Drag a VM onto the node to open the plan wizard.

**3. Worker** (optional, health on `:8090`):

```powershell
cd worker
go run ./cmd/worker
```

## Using the application

### Adding connections

The dashboard has two panels with add buttons:

- **Left panel (+ Add Source):** connect VMware vCenter, standalone ESXi, or Hyper-V.
- **Middle panel (+ Add Target):** connect Proxmox VE.

Click the button, fill in the modal (platform, endpoint, name, secret
reference), optionally click **Test**, then **Add Connection**. The connection
appears in its panel and inventory loads automatically. Remove a connection
with the red **x** on its header.

See [Managing Connections](./docs/knowledge-base/02-managing-connections.md).

### Planning a migration (drag and drop)

1. Drag a powered-off VM from a source panel onto a Proxmox node.
2. The wizard opens: review source, configure target, map storage and networks,
   then review.
3. Click **Create Draft Plan**. A draft appears in the right panel.

**The drag never migrates.** It only creates a draft plan awaiting preflight and
approval (future phases). See [Migration Planning](./docs/knowledge-base/03-migration-planning.md).

## Knowledge base

Full operator documentation lives in [`docs/knowledge-base/`](./docs/knowledge-base/README.md):

- [Getting Started](./docs/knowledge-base/01-getting-started.md)
- [Managing Connections](./docs/knowledge-base/02-managing-connections.md)
- [Drag and Drop Migration Planning](./docs/knowledge-base/03-migration-planning.md)
- [Safety and Rollback](./docs/knowledge-base/04-safety-and-rollback.md)
- [Troubleshooting](./docs/knowledge-base/05-troubleshooting.md)
- [FAQ](./docs/knowledge-base/06-faq.md)

## Verify everything

Run the full verification script:

```powershell
# Windows
./scripts/verify.ps1
```

```bash
# Linux / macOS
./scripts/verify.sh
```

This builds and tests all three components and reports pass/fail.

## Docker Compose (when Docker is available)

```bash
docker compose up --build
```

- Frontend: `http://localhost:5173`
- Backend API: `http://localhost:8080`
- Worker: `http://localhost:8090`
- PostgreSQL: `localhost:5432`

## Podman (tested)

The stack has been built and tested on **Podman 5.8** (no compose provider needed). A helper script manages a Podman pod hosting all three containers on a shared network.

```powershell
# Windows
.\scripts\podman-run.ps1 build    # build all three images
.\scripts\podman-run.ps1 up       # create pod + start backend, worker, frontend
.\scripts\podman-run.ps1 test     # smoke-test all endpoints
.\scripts\podman-run.ps1 status   # show pod/container status
.\scripts\podman-run.ps1 logs     # tail logs
.\scripts\podman-run.ps1 down     # stop and remove everything
```

```bash
# Linux / macOS
./scripts/podman-run.sh build
./scripts/podman-run.sh up
./scripts/podman-run.sh test
./scripts/podman-run.sh down
```

When running via Podman the helper publishes:
- Frontend: `http://localhost:5173` (nginx serves the SPA and proxies `/api` to the backend inside the pod)
- Backend: `http://localhost:8180` (mapped to container `:8080`; `:8080` is often in use by other services)
- Worker: `http://localhost:8090`
## Configuration

Configuration is environment-based. None is required for mock mode.

| Variable | Default | Description |
|----------|---------|-------------|
| `DRISHTI_MODE` | `mock` | `mock`, `lab`, or `production` |
| `DRISHTI_HTTP_ADDR` | `:8080` | API listen address |
| `DRISHTI_DB_URL` | _(empty)_ | Postgres URL; required for `lab`/`production` |
| `DRISHTI_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `DRISHTI_READ_TIMEOUT` | `15s` | HTTP read timeout |
| `DRISHTI_WRITE_TIMEOUT` | `30s` | HTTP write timeout |
| `DRISHTI_WORKER_ADDR` | `:8090` | Worker listen address |
| `DRISHTI_WORKSPACE_ROOT` | _(temp dir)_ | Worker temp workspace |

## Safety

Read **`SECURITY.md`** before contributing. Key invariants:

- Never delete/unregister the source VM automatically.
- Drag-and-drop only creates a draft plan.
- Never log secrets.
- Worker only runs allowlisted commands.

## Project structure

```
backend/    Go API/orchestrator
frontend/   React + TypeScript SPA
worker/     Go migration worker
docs/       ADRs, architecture, runbooks
scripts/    verification scripts
```

## License

Internal project. Contact yashwanta.thakur@martinrea.com.