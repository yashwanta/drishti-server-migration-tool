# Contributing

## Getting Started

See [README.md](./README.md) for setup, build, and test instructions.

## Safety First

Before contributing, read [SECURITY.md](./SECURITY.md). This project moves production virtual machines, so safety invariants are non-negotiable. Any PR that weakens them will be rejected.

- Never implement destructive hypervisor operations (delete, unregister, power changes against production) without an approved, audited, gated workflow.
- Never log credentials. Use the structured logger; it redacts sensitive fields.
- A drag-and-drop must only create a draft plan.

## Development Workflow

1. Branch from `main`.
2. Make focused changes. Keep diffs minimal.
3. Add or update tests for behavior changes.
4. Run the full verification script: `./scripts/verify.ps1` (Windows) or `./scripts/verify.sh` (Linux/macOS).
5. Update documentation and `docs/` ADRs for architecture changes.
6. Open a PR describing the change, the safety implications, and test evidence.

## Code Style

- Go: `gofmt` + `go vet`. Tab indentation. Packages have doc comments.
- TypeScript/React: 2-space indent, strict mode, no unused locals.
- SQL: 2-space indent, lowercase keywords.
- Commit messages: imperative mood, present tense.

## Architecture Decision Records

Record significant decisions in `docs/` using the ADR template (`docs/adr-0001-template.md`).