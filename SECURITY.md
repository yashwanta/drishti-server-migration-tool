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
- Lab and live read-only modes also accept direct platform credentials. The password is sent only
  to the backend, held in process memory for the connection session, excluded
  from API responses, and discarded when the connection is removed or the
  backend restarts. Use localhost or a TLS-terminated deployment for this mode.
- The structured logger (`internal/logging`) redacts known-sensitive keys and connection strings with embedded credentials.
- Operator passwords are stored only as bcrypt hashes. Mock mode reads them
  from the protected auth users file; lab/live/production import that file only
  into an empty PostgreSQL users table and use PostgreSQL authoritatively after
  the first import. Runtime password plaintext is accepted only by bounded auth
  requests and is never logged, audited, stored, or returned. Session identifiers are random, stored server-side only as
  SHA-256 digests, delivered in HttpOnly SameSite=Strict cookies, and never logged.
- Every state-changing authenticated request requires a per-session CSRF token.
- `.gitignore` blocks `.env`, `*.pem`, `*.key`, and `secrets/`.
- Phase 9 will add an encrypted secrets provider / pluggable secrets-provider interface.

## TLS

- Platform connections validate TLS certificates by default.
- Insecure TLS overrides are explicit, disabled by default, clearly labeled, and **never recommended for production**.

## Least Privilege

Platform service accounts must use read-only roles until Phase 6. Least-privilege role guidance is documented per adapter (see `docs/vmware-least-privilege.md` and `docs/proxmox-least-privilege.md` — to be added in Phases 2-3).

## Worker Isolation

The migration worker (`worker/`) only executes commands from a fixed safelist (`internal/cmdsafelist`). It never invokes a shell and validates all arguments for metacharacters.
