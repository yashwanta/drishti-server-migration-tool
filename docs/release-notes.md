# Release Notes - DRISHTI HyperShift v0.1

**Release:** 0.1 (Phase 0-11 complete)
**Date:** 2026-07-24
**Status:** Release candidate - mock mode

## Overview

DRISHTI HyperShift v0.1 is a self-hosted VMware-to-Proxmox migration
orchestrator with a full drag-and-drop planning workflow, preflight
compatibility engine, cold migration execution, validation, cutover, rollback,
reporting, RBAC, and audit trail.

This release runs in **mock mode** - all platform operations are simulated.
Real VMware and Proxmox connectors use the same adapter interfaces and can be
activated with real credentials in a future release.

## Features

### Connection management
- Add/remove VMware vCenter, ESXi, Proxmox VE, and Hyper-V connections
- Per-connection inventory panels
- Connectivity test button

### Migration planning
- Drag-and-drop VM to target node (never auto-migrates)
- 4-step wizard: source summary, target config, mappings, review
- Storage and network mapping with no silent guesses

### Preflight engine (8 checks)
- Power state (cold migration requires source off)
- Target capacity (CPU, memory)
- Storage availability per disk
- Unsupported features (RDM, passthrough, encryption)
- Firmware compatibility
- VMware Tools status
- Snapshot presence (warning)
- Target node online

### Cold migration execution (9-step state machine)
- Source power off
- Disk export with checksums
- Disk conversion (vmdk -> qcow2)
- Target VM creation (isolated)
- Disk attachment
- Isolated boot
- Validation (power, disks, VirtIO, network, TCP ports)
- Cutover (with safety interlocks)
- All steps idempotent and resumable

### Cutover and rollback
- Cutover refuses if source not off or validation not passed
- 14-day source retention window
- One-click rollback: isolate target, power off, restore source

### Security
- Structured logging with secret redaction
- Worker command safelist (no shell, no metacharacters)
- RBAC: 6 roles, 9 actions
- Pluggable secrets provider
- Full audit trail

### Reporting
- Audit-ready migration reports with timestamps, steps, checks, results
- Retention deadline tracking

## Architecture

- **Backend:** Go 1.25 (API/orchestrator + migration worker)
- **Frontend:** React 18 + TypeScript (strict), Vite, nginx
- **Containerization:** Podman 5.x (pod-based), Docker Compose supported
- **Database:** PostgreSQL schema baseline (in-memory for mock mode)

## Test results

- Backend: 21 unit tests pass (api, config, logging, mock, server, store)
- Worker: 7 unit tests pass (cmdsafelist, config, runner)
- Frontend: TypeScript strict typecheck pass, production build pass
- End-to-end: Full migration lifecycle + rollback drill passed
- All 8 verification script steps pass

## Known limitations

See [known-limitations.md](./known-limitations.md).

## Installation

See [README.md](../README.md) for setup instructions.

## What is NOT in this release

- Real VMware/Proxmox platform connections (mock only)
- Live/zero-downtime migration
- Automatic source deletion
- Authentication/session management
- Persistent storage (in-memory only)

## Next steps

1. Connect real vCenter/ESXi (Phase 2 real connector via govmomi)
2. Connect real Proxmox VE (Phase 3 real connector via PVE API)
3. Wire PostgreSQL persistence for production mode
4. Add authentication provider (Phase 9 enforcement)
5. Pilot with real disposable lab VMs