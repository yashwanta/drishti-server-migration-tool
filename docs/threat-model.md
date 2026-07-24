# Threat Model (Placeholder — to be completed in Phase 9)

## Scope

This document records threats, their controls, and residual risk for DRISHTI HyperShift. It will be completed formally during Phase 9 (Security, RBAC, Secrets, and Audit Hardening). This placeholder establishes the categories from the project's Initial Risk Register.

## Threats

| ID | Threat | Severity | Primary Control | Status |
|----|--------|----------|-----------------|--------|
| T1 | Duplicate IP/hostname from concurrent source+target | Critical | Source-off and target-isolated interlocks; refuse cutover if state uncertain | Planned (Phase 8) |
| T2 | Disk corruption / inconsistent copy | Critical | Cold migration first; checksums; clean shutdown; never copy a writing guest | Planned (Phase 5-6) |
| T3 | Credential compromise | Critical | Least privilege; encrypted secrets; TLS validation; redaction; rotation; worker isolation | Partial (Phase 0 redaction); Phase 9 full |
| T4 | Unsupported VM features silently migrated | High | Block at preflight; explicit support matrix | Planned (Phase 4) |
| T5 | Wrong VLAN or storage mapping | High | No silent guesses; named mapping templates; operator review | Partial (Phase 0 mapping UI); Phase 4 full |
| T6 | Partial job / retry duplicates | High | Durable states; idempotency keys; external task IDs; reconciliation | Planned (Phase 6) |
| T7 | Insufficient target capacity | High | Preflight capacity checks; reservation/recheck at execution | Planned (Phase 4, 6) |
| T8 | Rollback fails | High | Retain source unchanged; rehearse; isolate target first | Planned (Phase 8) |
| T9 | Command injection in worker | High | Fixed safelist; validated args; no shell; metacharacter rejection | Implemented (Phase 0) |
| T10 | Secret leakage in logs | High | Structured logger with redaction of keys, tokens, connection strings | Implemented (Phase 0) |
| T11 | Malicious / tampered disk image | Medium | Checksums; controlled workspace; validation | Planned (Phase 5) |
| T12 | Privilege escalation via API | Medium | RBAC; authorization checks; input validation | Planned (Phase 9) |

## Review cadence

Review this model at each phase boundary and after any security incident.