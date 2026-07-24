# Frequently Asked Questions

## General

**Is this production-ready?**
No. The project is in Phase 0 (bootstrap). It runs in mock mode with sample
data. Real VMware and Proxmox connectors arrive in Phases 2-3, and full cold
migration in Phase 6. See the project plan for the roadmap.

**What does mock mode mean?**
The application generates realistic sample inventory (VMs, hosts, nodes,
storage) without connecting to any real platform. It lets you exercise the UI
and workflow safely. Every connection you add gets generated inventory.

**Can I migrate VMs right now?**
No. You can create draft migration plans by dragging, but execution does not
exist yet (Phase 6). The source VM is never touched in Phase 0-1.

## Connections

**Can I add multiple vCenters?**
Yes. Click "+ Add Source" multiple times with different endpoints. Each gets
its own panel with its own inventory.

**What is the Secret Reference field?**
It is a pointer to where the credential lives (a vault path or token ID), not
the credential itself. The application never stores or logs the actual secret.

**Why is Hyper-V listed?**
Hyper-V is on the roadmap (Phase 12B). You can register a Hyper-V connection and
see preview inventory, but migration from Hyper-V is not in the current MVP.

**Does removing a connection delete anything?**
No. Removing a connection only removes it from the registry. It does not touch
any VM, datastore, or network on the platform.

## Migration

**Why does dragging not migrate?**
By design. A drag only opens a draft plan. This is a core safety invariant: no
migration can start without preflight, explicit configuration, and approval.

**What is cold migration?**
The source VM is powered off before disks are copied. This ensures a
consistent, corruption-free transfer. Live/zero-downtime migration is out of
scope for the MVP (roadmap item 12A).

**What happens to the source VM after migration?**
It stays registered and retained in VMware for rollback. It is never deleted
automatically. Source cleanup is a separate, explicit, audited action after the
retention window.

## Security

**Are my passwords logged?**
No. The structured logger redacts passwords, tokens, cookies, private keys,
and connection strings with embedded credentials before writing any output.

**Is TLS validated?**
Yes, by default. An insecure TLS option exists for lab testing but is clearly
labeled and never recommended for production.

**Who can perform migrations?**
In Phase 0-1 there is no authentication. Phase 9 adds RBAC with roles: Viewer,
Migration Planner, Migration Operator, Approver, Platform Administrator, and
Auditor.