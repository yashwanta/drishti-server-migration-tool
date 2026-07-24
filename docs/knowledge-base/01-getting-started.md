# Getting Started

Welcome to DRISHTI HyperShift. This guide covers your first run and what you
will see on the dashboard.

## What is DRISHTI HyperShift?

A self-hosted web application that connects to VMware (vCenter or ESXi) and
Proxmox VE, shows both inventories side by side, and lets an operator
**drag a source VM onto a target Proxmox node** to begin a controlled,
auditable migration plan.

> The drag never moves or deletes a VM. It opens a plan that must pass
> preflight checks and be explicitly approved before anything executes.

## Starting the application

### With Podman (recommended)

Open a terminal in the project folder and run:

```powershell
.\scripts\podman-run.ps1 up
```

Then open **http://localhost:5173** in your browser.

### Without containers (native)

```powershell
# Terminal 1 - backend API
cd backend
go run ./cmd/hypershift

# Terminal 2 - frontend UI
cd frontend
npm install   # first time only
npm run dev
```

Open **http://localhost:5173**.

## The dashboard layout

The screen is divided into three columns:

| Column | What it shows |
|--------|---------------|
| **Left: VMware / Source** | All source connections (vCenter, ESXi). Each shows datacenters, clusters, hosts, and VMs as draggable cards. |
| **Middle: Proxmox / Target** | All target connections (Proxmox). Each shows nodes with capacity, storage, bridges, and existing VMs. These are drop zones. |
| **Right: Draft Plans** | Migration plans you have created by dragging. Each is a draft until approved. |

## Identifying VMs you can migrate

Each source VM card shows:
- **Name and OS family** ([L] = Linux, [W] = Windows).
- **Power state** - a green badge means powered on; grey means off. Cold migration requires the VM to be powered off.
- **Resources** - vCPU, memory, disk count, total disk size.
- **Snapshots** - an amber badge shows how many snapshots exist.
- **Hover** the card to see notes (static IP, VLAN, migration hints).

## Next steps

- Add a real vCenter or ESXi: see [Managing Connections](./02-managing-connections.md).
- Plan your first migration: see [Drag and Drop Migration Planning](./03-migration-planning.md).