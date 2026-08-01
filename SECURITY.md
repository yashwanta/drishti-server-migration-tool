# Security Policy

## Reporting a Vulnerability

Report security issues privately to **yashwanta@lexingtonpcclinic.com**. Do **not** open a public issue for security-sensitive bugs. Acknowledge within 2 business days and target a fix within 30 days.

## Non-Negotiable Safety Invariants

This project migrates production virtual machines. The following invariants are enforced in code, review, and documentation. Any change that weakens them must be rejected.

1. **Never delete or unregister the source VMware VM automatically.** Source cleanup is a separate, explicitly approved, audited action that occurs only after the retention window.
2. **Never run source and destination simultaneously** on the same production network when hostname, IP, MAC, or application identity is preserved.
3. **A drag-and-drop never starts a migration.** It only opens a draft plan that requires preflight, explicit mapping, and approval.
4. **Never log secrets.** Passwords, API tokens, session cookies, private keys, BitLocker recovery keys, and guest credentials are redacted by the structured logger before output.
5. **No destructive operation against production in early phases.** Use disposable lab VMs and isolated networks.
6. **Never claim success solely because the target powered on.** Validate disks, networking, guest health, and application checks.
7. **Every mutating operation is idempotent or safely resumable,** and every state transition is recorded.
8. **Rollback is always available** until an administrator closes the retention window and authorizes cleanup.

## Secrets Handling

- Secrets are **referenced by `SecretRef`**, never stored inline in the database or config files.
- The structured logger (`internal/logging`) redacts known-sensitive keys and connection strings with embedded credentials.
- `.gitignore` blocks `.env`, `*.pem`, `*.key`, and `secrets/`.
- Phase 9 will add an encrypted secrets provider / pluggable secrets-provider interface.

## TLS

- Platform connections validate TLS certificates by default.
- Insecure TLS overrides are explicit, disabled by default, clearly labeled, and **never recommended for production**.

## Least Privilege

Platform service accounts must use read-only roles until Phase 6. Least-privilege role guidance is documented per adapter (see `docs/vmware-least-privilege.md` and `docs/proxmox-least-privilege.md` — to be added in Phases 2-3).

## Worker Isolation

The migration worker (`worker/`) only executes commands from a fixed safelist (`internal/cmdsafelist`). It never invokes a shell and validates all arguments for metacharacters.
