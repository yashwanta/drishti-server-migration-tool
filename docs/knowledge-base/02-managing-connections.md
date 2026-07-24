# Managing Connections

Connections are how DRISHTI HyperShift talks to your hypervisors. There are
two roles:

- **Source** - where VMs migrate FROM (VMware vCenter, ESXi, Hyper-V).
- **Target** - where VMs migrate TO (Proxmox VE).

## Adding a connection

1. In the dashboard, click **+ Add Source** (left panel) or **+ Add Target** (middle panel).
2. The **Add Connection** modal opens.
3. Fill in the fields:

| Field | Description |
|-------|-------------|
| **Platform** | `vmware` (vCenter/ESXi), `proxmox` (PVE), or `hyperv` (roadmap). |
| **Role** | `source` or `target`. Pre-set based on which button you clicked. |
| **Connection Name** | A friendly label, e.g. "vCenter Production". |
| **Endpoint** | Host or URL, e.g. `vcenter.example.local` or `pve-01.example.local:8006`. |
| **Secret Reference** | A vault path or token identifier. Secrets are never stored in the UI or logs. |
| **Skip TLS validation** | Lab-only checkbox. Never enable in production. |

4. Click **Test** to verify connectivity (mock mode always succeeds).
5. Click **Add Connection** to save.

The new connection appears in its panel and its inventory loads automatically.

## Supported platforms

| Platform | Role | Status |
|----------|------|--------|
| VMware vCenter / ESXi | Source | MVP (Phase 2 read-only connector) |
| Proxmox VE | Target | MVP (Phase 3 read-only connector) |
| Hyper-V | Source | Roadmap (Phase 12B); inventory preview only |

> Currently the application runs in **mock mode**, which generates realistic
> sample inventory for any connection you add. Real vCenter/Proxmox connectors
> arrive in Phases 2-3. The UI and workflow are identical either way.

## Removing a connection

Click the red **x** button on the connection header. Confirm the prompt.
Removing a connection does not affect existing draft plans or the audit log.

## Connection security

- **Secrets are referenced, never stored.** The `Secret Reference` field is a
  pointer (vault path or token id), not the credential itself.
- **TLS is validated by default.** The insecure option exists for lab testing
  only and is clearly labeled.
- **No destructive operations.** Adding or removing a connection never touches
  any VM on either platform.
- All connection changes are recorded in the audit log (`GET /api/v1/audit`).

## Adding the same endpoint twice

The application rejects a duplicate connection (same platform kind + endpoint)
with a clear error. Each endpoint should have exactly one connection entry.