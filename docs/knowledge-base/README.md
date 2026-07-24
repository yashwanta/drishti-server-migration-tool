# DRISHTI HyperShift - Knowledge Base

A practical guide for operators using DRISHTI HyperShift to migrate VMs from
VMware to Proxmox. Each article is focused and self-contained.

## How to use this knowledge base

- **New to the tool?** Start with [Getting Started](./01-getting-started.md).
- **Need to add infrastructure?** Read [Managing Connections](./02-managing-connections.md).
- **Planning a migration?** Read [Drag and Drop Migration Planning](./03-migration-planning.md).
- **Concerned about safety?** Read [Safety and Rollback](./04-safety-and-rollback.md).
- **Running into problems?** Read [Troubleshooting](./05-troubleshooting.md).
- **Have questions?** Read [FAQ](./06-faq.md).

## Articles

1. [Getting Started](./01-getting-started.md) - first run, the dashboard, what you see.
2. [Managing Connections](./02-managing-connections.md) - add/remove vCenter, ESXi, Proxmox, Hyper-V.
3. [Drag and Drop Migration Planning](./03-migration-planning.md) - create draft plans, never auto-migrate.
4. [Safety and Rollback](./04-safety-and-rollback.md) - the invariants that protect production.
5. [Troubleshooting](./05-troubleshooting.md) - common issues and fixes.
6. [FAQ](./06-faq.md) - frequently asked questions.
7. [Migration Lifecycle](./07-migration-lifecycle.md) - the 7-step workflow.

## Quick reference

| What | Where |
|------|-------|
| Dashboard URL | http://localhost:5173 |
| Backend API | http://localhost:8180 |
| Worker | http://localhost:8090 |
| Project plan | `DRISHTI_HyperShift_PROJECT_PLAN.md` |
| Security policy | `SECURITY.md` |
| Architecture | `docs/architecture.md` |