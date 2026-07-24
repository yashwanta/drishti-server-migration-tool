# DRISHTI HyperShift
## VMware-to-Proxmox Migration Orchestrator - Project Definition, Architecture, Delivery Phases, and Codex Prompts
**Version 0.1 - Initial MVP Plan**  
Created by: Yashwanta Thakur  
Date: July 24, 2026  
Email: yashwanta.thakur@martinrea.com
> This file is the machine-readable project master for Codex. Execute one phase at a time. Do not skip safety gates or infer permission to touch production.
## 1. Project Summary
**Purpose:** Build a self-hosted application that connects to VMware vCenter or standalone ESXi and Proxmox VE, displays both inventories in one interface, and lets an authorized administrator drag a source VM onto a target Proxmox node to create a controlled migration job.
**Core principle:** Drag-and-drop must never immediately move or delete a VM. It opens a migration plan, preflight analysis, mappings, approvals, and an auditable execution workflow.
**Rollback principle:** The original VMware VM remains registered and retained. After cutover it is powered off and held for rollback until an administrator explicitly approves deletion after the retention period.
**MVP boundary:** VMware vCenter/ESXi to Proxmox VE, powered-off test VMs first, cold migration only, one VM per job, Windows and Linux guests, local storage and common shared storage, standard virtual disks, BIOS/UEFI, and standard VLAN-backed networking.

## 2. Non-Negotiable Safety Invariants
- Never delete or unregister the source VMware VM automatically.
- Never run source and destination simultaneously on the same production network when hostname, IP address, MAC address, or application identity is preserved.
- Never start an actual migration from a drag event alone; require preflight, explicit configuration, and approval.
- Never log passwords, API tokens, session cookies, private keys, BitLocker recovery information, or guest credentials.
- Never test destructive operations against production in early phases. Use disposable lab VMs and isolated networks.
- Never claim success solely because the target VM powered on. Validate disks, networking, guest health, and application checks.
- Every mutating operation must be idempotent or safely resumable, and every job state transition must be recorded.
- Rollback must remain available until an administrator closes the retention window and explicitly authorizes source cleanup.

## 3. MVP Scope
### In scope
- Connect and authenticate to one or more vCenter/ESXi endpoints.
- Connect and authenticate to one or more Proxmox VE clusters.
- Inventory source VMs, hosts, datastores, networks, disks, firmware, guest OS, snapshots, and power state.
- Inventory Proxmox nodes, storage, bridges, VLAN capabilities, resource availability, and VM IDs.
- Drag a VMware VM onto a Proxmox node or target zone to begin planning.
- Map VMware datastores and networks to Proxmox storage, bridges, and VLAN tags.
- Run compatibility, capacity, snapshot, firmware, disk, network, and guest-driver preflight checks.
- Perform a cold migration of a supported powered-off VM.
- Copy and convert VMDK disks into a Proxmox-supported target format.
- Create the target VM with controlled CPU, memory, firmware, disks, boot order, and NIC configuration.
- Validate target boot and health on an isolated validation network before production cutover.
- Provide a one-click rollback workflow that powers down/isolate the target before restarting the source.
- Generate an audit-ready migration report with timestamps, operators, checks, warnings, and results.

### Out of scope
- Zero-downtime migration or live cross-hypervisor vMotion equivalent.
- Automatic migration of encrypted VMs, vTPM, VMware Fault Tolerance, NSX overlays, RDM/shared disks, SR-IOV, GPU/PCI passthrough, or unsupported snapshot chains.
- Deleting source VMs automatically after migration.
- Universal support for Hyper-V, Nutanix, XCP-ng, or every storage vendor in the MVP.
- Automatic application-specific database consistency for every workload.
- Production migration without an approved maintenance window and rollback plan.

## 4. Architecture
- **Web UI:** React and TypeScript interface for platform connections, inventory trees, drag-and-drop planning, mappings, approvals, progress, rollback, and reports.
- **API / Orchestrator:** Go service providing REST APIs, inventory normalization, preflight decisions, job state transitions, approvals, audit events, and worker coordination.
- **VMware adapter:** Uses the vSphere API to discover inventory, collect VM configuration, manage approved snapshots where supported, export/copy disks, and control source power state.
- **Proxmox adapter:** Uses the Proxmox VE API to inspect nodes/storage/networking, reserve VM IDs, create VMs, attach imported disks, control power state, and collect task status.
- **Migration worker:** Linux worker running controlled disk transfer and conversion tools such as qemu-img and virt-v2v, with checksums, temporary workspace control, cancellation, and cleanup.
- **Guest remediation:** Optional pre/post migration helper for Windows and Linux to collect and restore static IP configuration, install/activate VirtIO support, enable QEMU Guest Agent, and run health checks.
- **Database:** PostgreSQL for platforms, normalized inventory, mappings, migration plans, jobs, job steps, approvals, audit events, validation results, and retention state.
- **Secrets:** Credential references stored through an encrypted secret provider. The database stores references and metadata, not plaintext passwords.

### Job state model
```text
DRAFT -> PREFLIGHT_RUNNING -> PREFLIGHT_FAILED -> READY_FOR_APPROVAL -> APPROVED -> SOURCE_PREPARATION -> COPYING -> CONVERTING -> CREATING_TARGET -> CONFIGURING_TARGET -> VALIDATING_ISOLATED -> READY_FOR_CUTOVER -> CUTOVER_IN_PROGRESS -> VALIDATING_PRODUCTION -> COMPLETED_WITH_ROLLBACK -> ROLLBACK_IN_PROGRESS -> ROLLED_BACK -> FAILED -> CANCELLED -> RETENTION_EXPIRED -> SOURCE_CLEANUP_APPROVED -> CLOSED
```

## 5. Recommended Repository Structure
```text
drishti-hypershift/
├─ README.md
├─ PROJECT_PLAN.md
├─ SECURITY.md
├─ CONTRIBUTING.md
├─ Makefile
├─ compose.yaml
├─ docs/
│  ├─ architecture/
│  ├─ adr/
│  ├─ api/
│  ├─ operations/
│  └─ test-evidence/
├─ backend/
│  ├─ cmd/api/
│  ├─ cmd/worker/
│  ├─ internal/domain/
│  ├─ internal/platform/vmware/
│  ├─ internal/platform/proxmox/
│  ├─ internal/preflight/
│  ├─ internal/migration/
│  ├─ internal/validation/
│  ├─ internal/audit/
│  └─ migrations/
├─ frontend/
│  ├─ src/features/connections/
│  ├─ src/features/inventory/
│  ├─ src/features/migrations/
│  ├─ src/features/mappings/
│  └─ src/features/reports/
├─ worker/
│  ├─ scripts/
│  ├─ images/
│  └─ test-fixtures/
├─ agent/
│  ├─ windows/
│  └─ linux/
├─ deploy/
│  ├─ container/
│  ├─ systemd/
│  └─ examples/
└─ tests/
   ├─ contract/
   ├─ integration/
   ├─ e2e/
   └─ fixtures/
```

## 6. Codex Operating Rules
- Read this complete file before starting a phase.
- Inspect the repository and existing evidence before editing.
- Execute only the requested phase. Do not begin future phases.
- Prefer the smallest complete change that meets acceptance criteria.
- Use mocks, fixtures, and disposable lab systems before any live integration.
- Do not use production credentials or infrastructure unless the user explicitly supplies and authorizes a lab-safe target.
- Do not delete or unregister source VMs.
- Run tests, builds, linters, and security checks available for the phase.
- Report failures verbatim; do not hide or hand-wave skipped validation.
- At phase completion, list changed files, commands, results, gaps, risks, and acceptance verdict.

### First prompt to use with Codex
```text
Read PROJECT_PLAN.md completely. Do not implement anything yet. Inspect the repository and summarize the project rules, current repository state, safety invariants, and the next incomplete phase. Then propose a file-level plan for that phase and stop for review.
```

## Phase 0: Repository Bootstrap and Engineering Guardrails
**Objective:** Create a clean, testable project skeleton and establish non-negotiable safety and quality rules before connecting to any hypervisor.

### Scope
- Initialize the monorepo structure for Go backend, React/TypeScript frontend, migration worker, documentation, and tests.
- Add local development configuration using containers for PostgreSQL and the application services.
- Add linting, formatting, unit-test commands, configuration loading, structured logging, health endpoints, and migration tooling.
- Add SECURITY.md, threat-model placeholder, architecture decision record template, and contribution rules.
- Create a development-only mock inventory provider so the UI can be built without real infrastructure.

### Deliverables
- Compiling backend and frontend skeletons.
- Database migration baseline.
- Mock mode with sample VMware and Proxmox inventory.
- CI workflow or local verification script.
- README with setup, test, and safe-development instructions.

### Acceptance criteria
- A new developer can clone the repository and start the stack using documented commands.
- All lint, unit-test, and build commands pass.
- No real platform credentials are required.
- No destructive hypervisor operation exists in the codebase.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md completely. Execute only Phase 0: Repository Bootstrap and Engineering Guardrails.

Before changing files, inspect the repository and report the current state, assumptions, and exact files you plan to create or modify. Then implement the smallest complete Phase 0 solution.

Requirements:
- Use a Go backend, React + TypeScript frontend, PostgreSQL, and a separate worker boundary.
- Add a mock provider that returns realistic VMware and Proxmox inventory without connecting to real systems.
- Add formatting, linting, unit tests, health checks, structured logging, configuration validation, database migrations, and documented local startup.
- Add SECURITY.md and an ADR template.
- Do not add real migration execution, real credentials, or destructive calls.
- Keep secrets out of source control and logs.
- Run all available tests and builds.

At completion, provide: changed files, commands run, test results, known gaps, and a clear Phase 0 acceptance-criteria verdict. Stop after Phase 0.
```

## Phase 1: Read-Only Product UI and Domain Model
**Objective:** Build the first usable interface and core domain objects using mock data only.

### Scope
- Create source and target inventory panels with expandable platform, datacenter/cluster, host/node, VM, storage, and network views.
- Implement drag-and-drop from a VMware VM card to a Proxmox node or target zone.
- Make the drop action open a migration-plan wizard; it must not execute a migration.
- Define normalized domain models for platform connections, VM hardware, disks, networks, mappings, plans, jobs, checks, and audit events.
- Add visible warnings explaining that the source VM is retained and that source and target must not run together on production networking.

### Deliverables
- Responsive read-only inventory dashboard using mock providers.
- Migration-plan wizard with source summary, target selection, storage mapping, network mapping, and review screen.
- Backend domain models and API contracts.
- Component and API contract tests.

### Acceptance criteria
- A mock VMware VM can be dragged to a mock Proxmox node.
- The resulting wizard preserves source details and allows mappings to be edited.
- No migration or power action is called.
- Refresh and navigation do not lose saved draft plans.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and the completed Phase 0 evidence. Execute only Phase 1: Read-Only Product UI and Domain Model.

Build the inventory dashboard and migration-plan wizard using mock data. Dragging a VMware VM onto a Proxmox target must only create/open a draft migration plan. It must never migrate, copy disks, change power state, unregister, or delete anything.

Implement normalized backend domain models and versioned API contracts for connections, inventory, VM hardware, disks, networks, mappings, migration plans, checks, jobs, and audit events. Add tests for drag/drop behavior, draft persistence, and API validation.

Preserve the safety language in the UI: the source remains in VMware for rollback, and source and destination cannot run simultaneously on the same production network.

Run all tests and builds. Report changed files, screenshots or test evidence where practical, acceptance-criteria results, and gaps. Stop after Phase 1.
```

## Phase 2: VMware Read-Only Connector
**Objective:** Connect safely to vCenter or standalone ESXi and normalize real source inventory without mutating it.

### Scope
- Add secure connection profiles for vCenter/ESXi with certificate validation and explicit lab-only override controls.
- Discover datacenters, clusters, hosts, datastores, networks/port groups, VMs, snapshots, disks, NICs, firmware, guest OS, VMware Tools status, and power state.
- Store normalized inventory snapshots and last-refresh status.
- Add least-privilege documentation for the VMware service account.
- Implement connection testing, timeout handling, pagination where required, and useful sanitized errors.

### Deliverables
- Read-only VMware adapter behind an interface.
- Connection and inventory APIs.
- Unit tests using mocked vSphere responses and contract fixtures.
- Lab verification instructions that make no VM changes.

### Acceptance criteria
- The application can list a lab vCenter or ESXi inventory accurately.
- No VMware mutating API method is exposed by this phase.
- Credentials and session material never appear in logs or API responses.
- A failed or unreachable endpoint produces a safe, actionable error.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 2: VMware Read-Only Connector.

Implement a read-only VMware adapter for vCenter and standalone ESXi. Normalize inventory into the existing domain model: datacenters, clusters, hosts, datastores, networks/port groups, VMs, disks, NICs, firmware, guest OS, snapshots, VMware Tools state, and power state.

Security requirements:
- Validate TLS certificates by default.
- Any insecure lab override must be explicit, disabled by default, clearly labeled, and never recommended for production.
- Never return or log passwords, tokens, cookies, or private material.
- Do not implement snapshot creation, power operations, export, unregister, delete, or any other mutation.

Use interfaces and fixtures so unit/contract tests do not require a live platform. Add a lab read-only verification procedure and least-privilege role guidance.

Run tests, static analysis, and builds. Report evidence and stop after Phase 2.
```

## Phase 3: Proxmox Read-Only Connector
**Objective:** Connect safely to Proxmox VE and normalize target capacity, storage, networking, and existing VM inventory.

### Scope
- Add token-based Proxmox connection profiles with TLS validation.
- Discover clusters, nodes, node status, CPU/memory capacity, storage, storage content support, bridges, VLAN awareness, existing VMs, and available VM IDs.
- Store normalized target inventory snapshots and last-refresh status.
- Document minimum read-only permissions and a future separated migration-executor role.
- Handle single-node and clustered Proxmox deployments.

### Deliverables
- Read-only Proxmox adapter behind an interface.
- Connection and target-inventory APIs.
- Mocked unit and contract tests.
- Lab verification instructions with no VM changes.

### Acceptance criteria
- The UI displays real lab Proxmox nodes, storage, bridges/VLAN capabilities, capacity, and VMs.
- No Proxmox mutation is available in this phase.
- API tokens are never exposed or logged.
- Unavailable nodes and storage are clearly identified.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 3: Proxmox Read-Only Connector.

Implement a read-only Proxmox VE adapter. Normalize clusters, nodes, health, capacity, storage, supported content types, bridges, VLAN-awareness, existing VMs, and candidate VM IDs into the existing domain model.

Use token authentication and TLS validation. Keep insecure lab overrides explicit and disabled by default. Do not implement VM creation, disk import, power operations, deletion, or any target mutation.

Support both a standalone Proxmox node and a cluster. Add mocked tests, safe error handling, least-privilege documentation, and a lab read-only verification procedure.

Run all tests and builds, report evidence, and stop after Phase 3.
```

## Phase 4: Mapping and Preflight Compatibility Engine
**Objective:** Determine whether a selected VM can be safely migrated and exactly how source resources map to the target.

### Scope
- Implement storage mappings, network/port-group to bridge/VLAN mappings, target node selection, target VM ID/name rules, CPU/memory settings, firmware choice, disk format, controller model, and NIC model.
- Build checks for target capacity, storage availability, snapshot presence, unsupported disks, firmware, Secure Boot, vTPM, encryption, RDM, passthrough, NSX/dependent networking, VMware Tools, guest OS support, and duplicate IP/MAC risk.
- Classify each check as PASS, WARNING, BLOCKER, or MANUAL REVIEW.
- Generate a deterministic migration plan and estimated data-transfer size.
- Require all blockers to be resolved before approval.

### Deliverables
- Versioned preflight rules engine.
- Mapping templates reusable by site or cluster.
- Preflight UI with evidence and remediation guidance.
- Exportable migration plan in JSON and human-readable form.

### Acceptance criteria
- Known unsupported configurations are blocked before execution.
- Mappings are explicit; the system never silently guesses a production VLAN.
- Preflight results are reproducible from the same inventory snapshot and plan.
- A supported lab VM can reach READY_FOR_APPROVAL without any platform mutation.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 4: Mapping and Preflight Compatibility Engine.

Implement explicit storage, network, VLAN, firmware, controller, disk-format, NIC-model, CPU, memory, name, and VM-ID mappings. Build a deterministic preflight engine that classifies checks as PASS, WARNING, BLOCKER, or MANUAL REVIEW.

At minimum detect: insufficient capacity, unavailable storage, active or complex snapshot chains, encrypted disks/VMs, vTPM, Secure Boot concerns, RDM/shared disks, PCI/GPU/SR-IOV passthrough, unsupported firmware, unsupported guest OS, NSX-dependent networking, duplicate IP/MAC risk, and missing guest driver readiness.

Never silently select a production network or ignore a blocker. Produce a versioned JSON plan plus a human-readable review. Add comprehensive rule tests using supported and unsupported fixtures.

Do not implement migration execution. Run all tests and stop after Phase 4.
```

## Phase 5: Migration Worker Proof of Concept
**Objective:** Prove controlled disk copy and conversion using a disposable powered-off lab VM without yet building full production orchestration.

### Scope
- Create a worker execution boundary with a strict allowlist of commands and arguments.
- Implement temporary workspace creation, free-space checks, controlled disk transfer, checksum calculation, VMDK inspection, and conversion to a selected target format.
- Capture structured progress and sanitized logs.
- Support cancellation and guaranteed temporary-file cleanup.
- Use a disposable test fixture or powered-off lab VM only.

### Deliverables
- Worker service or process with job-step API.
- Disk transfer/conversion proof of concept.
- Checksums and conversion evidence.
- Failure, cancellation, and cleanup tests.

### Acceptance criteria
- A test VMDK can be copied and converted with matching integrity evidence.
- No shell command is assembled from untrusted free-form input.
- Cancellation leaves no orphaned process or uncontrolled temporary data.
- The source VM and source disks remain unchanged.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 5: Migration Worker Proof of Concept.

Build a hardened worker boundary for a disposable, powered-off lab VM or test VMDK. Implement free-space checks, temporary workspace control, disk transfer, VMDK inspection, conversion to the configured Proxmox-compatible format, checksums, structured progress, cancellation, and cleanup.

Do not build full source/target orchestration yet. Do not power on/off real VMs, create target VMs, delete source data, or use production infrastructure.

Security requirements:
- No arbitrary shell execution.
- Use fixed executables and validated argument arrays.
- Sanitize logs and API errors.
- Enforce file ownership, permissions, workspace boundaries, and size limits.

Add success, failure, cancellation, insufficient-space, checksum, and cleanup tests. Provide exact lab evidence and stop after Phase 5.
```

## Phase 6: End-to-End Cold Migration Orchestration
**Objective:** Migrate one supported powered-off lab VM from VMware to Proxmox while retaining the source unchanged for rollback.

### Scope
- Add narrowly scoped VMware read/export operations and Proxmox VM creation/import operations required by the approved plan.
- Implement durable job states, step retries, task polling, idempotency keys, resume behavior, and operator cancellation.
- Create target VM configuration from the approved plan, import disks, attach disks/NICs, configure boot order, and keep the target network isolated initially.
- Preserve source registration and source disks. Do not delete source snapshots or source VM unless a separate future cleanup workflow explicitly permits it.
- Add full audit events around approvals and mutating calls.

### Deliverables
- One-VM cold migration workflow in a lab.
- Durable job state machine and job timeline UI.
- Idempotent resume/retry behavior.
- Lab migration runbook and evidence.

### Acceptance criteria
- A supported powered-off lab VM is recreated in Proxmox with expected CPU, memory, firmware, disks, and isolated NIC configuration.
- The original VMware VM remains present and unchanged except for explicitly approved power state or export-related operations.
- Repeated job polling or retry does not create duplicate VMs or disks.
- A failed step produces a recoverable state and does not falsely mark completion.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 6: End-to-End Cold Migration Orchestration.

Implement the minimum mutating platform operations needed to migrate one supported, powered-off lab VM from VMware to Proxmox. Use only an approved preflight plan.

Required behavior:
- Durable state machine, audit events, idempotency, task polling, retries, resume, and cancellation.
- Copy/convert disks, create the target VM, attach disks and NICs, configure firmware and boot order, and leave networking isolated for validation.
- Retain the VMware VM registered with its source disks. Never delete or unregister it.
- Prevent duplicate target VMs/disks when a job is retried.
- Do not perform production cutover yet.

Use least-privilege roles and lab infrastructure only. Add integration tests with adapters/mocks plus documented lab evidence. Stop after Phase 6.
```

## Phase 7: Guest Drivers and Network Remediation
**Objective:** Make migrated Windows and Linux guests boot reliably and restore approved network settings without creating duplicate identity conflicts.

### Scope
- Collect guest pre-migration facts: OS, disks, boot mode, interfaces, static/DHCP state, IPs, routes, DNS, gateways, and critical services.
- Provide a Windows remediation path for VirtIO storage/network drivers and QEMU Guest Agent.
- Provide Linux handling for common NetworkManager, Netplan, and systemd-networkd configurations.
- Restore network settings only after the target is isolated and the source is confirmed powered off.
- Redact sensitive guest details and support a manual mode when automatic remediation is unsafe.

### Deliverables
- Guest facts schema and encrypted storage policy.
- Windows and Linux remediation workflows.
- Static IP restoration with safety interlocks.
- Guest-agent and connectivity status reporting.

### Acceptance criteria
- Supported Windows and Linux lab VMs boot with recognized storage and NICs.
- A static IP is never activated while the VMware source is running on the same production network.
- Unsupported network configurations are sent to manual review rather than overwritten.
- Sensitive guest data is not exposed in normal logs.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 7: Guest Drivers and Network Remediation.

Implement safe guest preparation/remediation for supported Windows and Linux lab VMs. Collect only necessary pre-migration facts, support VirtIO readiness and QEMU Guest Agent, and handle common DHCP/static-IP configurations for Windows, NetworkManager, Netplan, and systemd-networkd.

Critical interlock: never activate the destination's preserved production IP, hostname identity, or MAC-derived identity unless the source is confirmed powered off and the destination is in the approved cutover state. Unsupported or ambiguous configurations must require manual review.

Encrypt or tightly protect stored guest facts, redact logs, add rollback-safe behavior, and test with disposable guests. Do not perform general production cutover in this phase. Stop after Phase 7.
```

## Phase 8: Validation, Cutover, and Rollback
**Objective:** Add a controlled production cutover sequence and reliable rollback to the retained VMware source.

### Scope
- Validate target boot, guest-agent response, expected disks, IP configuration, gateway, DNS, required TCP ports, services, and optional HTTP health endpoints.
- Implement approval gates for moving from isolated validation to production networking.
- Ensure the source is powered off before the destination receives production connectivity.
- Implement rollback: isolate/power off target, restore target network safeguards, power on source, and validate source health.
- Start a configurable source-retention window after successful cutover.

### Deliverables
- Validation profile templates and results UI.
- Cutover checklist and approval gate.
- Rollback button/workflow with mandatory confirmation and audit trail.
- Retention tracking and source cleanup eligibility state.

### Acceptance criteria
- A lab migration can cut over without duplicate production identity.
- A deliberately failed validation can trigger rollback and restore the VMware source.
- The target is isolated before the source is restarted.
- The source remains retained after successful migration until a separate cleanup approval.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 8: Validation, Cutover, and Rollback.

Implement isolated validation, approval-gated production cutover, and rollback. Validation must support boot status, guest-agent response, disk count/capacity, IP/gateway/DNS, required TCP ports, system services, and optional application health URLs.

Cutover safety sequence:
1. Confirm source VMware VM is powered off.
2. Confirm destination validation passed or approved exceptions exist.
3. Move/attach destination to the approved production network.
4. Validate production health.

Rollback safety sequence:
1. Isolate and power off the Proxmox destination.
2. Confirm no duplicate production identity can exist.
3. Power on the retained VMware source.
4. Validate source health and record the result.

Add retention tracking; do not delete the source. Test successful cutover, failed validation, rollback, interrupted rollback, and duplicate-identity prevention. Stop after Phase 8.
```

## Phase 9: Security, RBAC, Secrets, and Audit Hardening
**Objective:** Harden the application for controlled internal use by authorized infrastructure administrators.

### Scope
- Implement roles such as Viewer, Migration Planner, Migration Operator, Approver, Platform Administrator, and Auditor.
- Separate connection-management, planning, execution, rollback, and cleanup permissions.
- Add encrypted secret storage integration or a pluggable secrets-provider interface.
- Add CSRF/session protections, secure cookies, API authorization checks, rate limits, and tamper-evident audit exports.
- Perform threat modeling for credential theft, job tampering, command injection, duplicate identity, malicious images, network interception, and privilege escalation.

### Deliverables
- RBAC enforcement in UI and API.
- Secrets provider and credential rotation workflow.
- Threat model and security test plan.
- Audit export and security review evidence.

### Acceptance criteria
- Unauthorized users cannot create connections, approve, execute, rollback, or clean up jobs.
- Secrets do not appear in database dumps, logs, API payloads, or frontend state.
- Critical API authorization paths have automated negative tests.
- Threat-model findings are tracked with owners and severity.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 9: Security, RBAC, Secrets, and Audit Hardening.

Implement server-side authorization for Viewer, Migration Planner, Migration Operator, Approver, Platform Administrator, and Auditor roles. Separate permissions for connection management, plan editing, approval, execution, cutover, rollback, retention extension, and source cleanup approval.

Add a pluggable encrypted secrets provider, credential rotation support, secure session handling, CSRF protections where applicable, rate limits, sanitized errors, and comprehensive negative authorization tests. Ensure secrets never appear in logs, API responses, frontend state, or normal database exports.

Create/update the threat model covering credential theft, API abuse, command injection, job tampering, duplicate identity, malicious disk images, TLS interception, worker compromise, and privilege escalation. Run security-focused tests and stop after Phase 9.
```

## Phase 10: Batch Planning, Scheduling, Reporting, and Operations
**Objective:** Make the tool usable for organized migration waves while keeping execution controlled and observable.

### Scope
- Add migration waves and dependency-aware ordering without enabling unsafe automatic concurrency.
- Add maintenance-window scheduling, per-site mapping templates, bandwidth limits, worker assignment, and concurrency limits.
- Add operational dashboards for queue, progress, throughput, failures, retries, and retention deadlines.
- Generate PDF/HTML/JSON migration reports and a rollback report.
- Add notification hooks and a documented support/incident runbook.

### Deliverables
- Migration wave planner and scheduler.
- Operations dashboard and alerts.
- Exportable reports and evidence bundles.
- Deployment, backup, restore, upgrade, and incident procedures.

### Acceptance criteria
- Multiple lab plans can be grouped and scheduled without exceeding configured concurrency or bandwidth limits.
- Operators can see exactly what is running, blocked, waiting, failed, or eligible for rollback/cleanup.
- Reports identify source, target, mappings, approvals, checks, timings, outcomes, and retained rollback state.
- Application backup and restore procedures are tested.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and prior phase evidence. Execute only Phase 10: Batch Planning, Scheduling, Reporting, and Operations.

Add migration waves, maintenance windows, site mapping templates, worker assignment, bandwidth controls, and conservative concurrency limits. Do not allow one failed job to cause unsafe automatic changes to unrelated VMs.

Build an operations dashboard for queued/running/blocked/failed/completed/rollback-available jobs. Generate human-readable and machine-readable migration and rollback reports. Add notification hooks, retention-deadline visibility, and documented deployment, backup, restore, upgrade, and incident runbooks.

Test scheduling boundaries, concurrency, cancellation, restart recovery, report correctness, and application backup/restore. Stop after Phase 10.
```

## Phase 11: Beta Hardening and Controlled Pilot
**Objective:** Prepare the MVP for a limited internal pilot using non-critical workloads and formal evidence.

### Scope
- Run full unit, integration, end-to-end, security, upgrade, backup/restore, and disaster-recovery testing.
- Perform performance tests for large disks and slow links.
- Complete accessibility, usability, logging, error-message, and documentation review.
- Create a pilot checklist, support matrix, known-limitations list, rollback drill, and go/no-go criteria.
- Run a controlled pilot with disposable or non-critical systems only.

### Deliverables
- Release candidate and signed test evidence.
- Supported-configuration matrix.
- Known limitations and blocked feature list.
- Pilot runbook, rollback drill results, and release notes.

### Acceptance criteria
- All Critical and High defects are resolved or explicitly accepted by authorized stakeholders.
- Rollback has been successfully demonstrated in the pilot environment.
- The product clearly blocks unsupported VMs rather than attempting unsafe best-effort migration.
- Source cleanup remains a separate, explicit, audited action.

### Prompt to give Codex
```text
Read PROJECT_PLAN.md and all prior phase evidence. Execute only Phase 11: Beta Hardening and Controlled Pilot Preparation.

Do not add broad new features. Focus on correctness, reliability, security, documentation, performance, usability, upgrade/backup/restore, and rollback evidence.

Create the supported-configuration matrix, known-limitations list, pilot checklist, go/no-go criteria, release notes, and test-evidence index. Run the complete automated test suite and document any lab tests that cannot be automated. Perform a rollback drill using a disposable or non-critical VM.

Do not approve production use yourself. Provide an objective release-readiness verdict with unresolved risks and stop after Phase 11.
```

## Future Roadmap
- **12A - VMware warm/incremental copy research:** Research Changed Block Tracking and snapshot-consistent delta transfer. Treat as a separate experimental project; do not market as zero downtime until proven.
- **12B - Additional source adapters:** Hyper-V, XCP-ng, Nutanix, and other adapters using the same normalized inventory and job model.
- **12C - Proxmox-to-Proxmox and disaster recovery:** Reuse the workflow for cross-cluster moves and DR rehearsal.
- **12D - Dependency discovery:** Identify application, DNS, firewall, load-balancer, backup, monitoring, and service dependencies before migration.
- **12E - DRISHTI integration:** Feed migration events and health results into DRISHTI SiteOps/NOC for operational monitoring and post-cutover observability.

## Initial Risk Register
| Risk | Severity | Primary control |
|---|---|---|
| Duplicate IP/hostname | Critical | Enforce source-off and target-isolated interlocks; refuse cutover if state is uncertain. |
| Disk corruption/inconsistent copy | Critical | Cold migration first; checksums; clean shutdown; application-aware prechecks; never copy a writing guest without a proven consistent method. |
| Credential compromise | Critical | Least privilege, encrypted secrets, TLS validation, redaction, rotation, worker isolation. |
| Unsupported VM features | High | Block at preflight; maintain explicit support matrix; require manual engineering path. |
| Wrong VLAN or storage mapping | High | No silent guesses; named mapping templates; operator review and approval. |
| Partial job/retry duplicates | High | Durable states, idempotency keys, external task IDs, reconciliation before retry. |
| Insufficient target capacity | High | Preflight capacity checks plus reservation/recheck at execution. |
| Rollback fails | High | Retain source unchanged; rehearse rollback; isolate target first; validate source health. |
| Large migration window | Medium | Estimate transfer, bandwidth controls, pilot measurements, future incremental copy research. |
| Licensing or hardware-bound software | Medium | Preflight/manual review and application-owner validation. |

## Definition of MVP Complete
- A supported powered-off Windows or Linux lab VM can be planned, preflighted, migrated, validated, cut over, and rolled back through the application.
- The source VMware VM remains registered and retained throughout the rollback window.
- Unsupported configurations are blocked with clear reasons.
- No secrets appear in logs or UI responses.
- Jobs are auditable, resumable where safe, and protected by approvals and RBAC.
- The project has a tested backup/restore procedure, support matrix, runbook, and release-readiness evidence.
