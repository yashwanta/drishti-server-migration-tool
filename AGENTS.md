# AGENTS.md — DRISHTI HyperShift

## Project

DRISHTI HyperShift is a VMware-to-Proxmox migration orchestrator. See
`DRISHTI_HyperShift_PROJECT_PLAN.md` for the full project master.

## Layout

- `backend/` — Go API/orchestrator (module `github.com/drishti/hypershift`)
- `frontend/` — React + TypeScript SPA (Vite)
- `worker/` — Go migration worker (module `github.com/drishti/hypershift-worker`)
- `docs/` — ADRs, threat model, runbooks
- `scripts/` — local verification scripts

## Safety invariants (non-negotiable)

Read `SECURITY.md`. Highlights:
- Never delete/unregister the source VM automatically.
- Drag-and-drop only creates a draft plan; never migrates.
- Never log secrets. Use the structured logger.
- Worker only runs allowlisted commands with validated args.

## Build / test

```
cd backend && go build ./... && go test ./...
cd worker  && go build ./... && go test ./...
cd frontend && npm install && npx tsc --noEmit && npm run build
```

Or run `./scripts/verify.ps1` / `./scripts/verify.sh`.

## Conventions

- Go: gofmt, go vet, tab indent, package doc comments.
- TS/React: strict, 2-space, no unused locals.
- Keep secrets out of source control and logs.
- Do not commit to git unless explicitly asked.