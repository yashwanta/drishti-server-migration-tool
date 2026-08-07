# Session authentication, users, and RBAC

HyperShift uses named operator accounts, bcrypt password hashes, server-side
sessions, per-session CSRF tokens, and route-level RBAC. It does not implement
email delivery or password reset by email.

## Runtime user storage

Mock mode uses a concurrency-safe in-memory user store seeded from
`DRISHTI_AUTH_USERS_FILE`. Mock-created users do not survive a restart and mock
mode does not require PostgreSQL.

Lab, live, and production use PostgreSQL as the authoritative user store. When
the `auth_users` table is empty, the backend imports the protected JSON file in
one transaction. The bootstrap must contain an active `platform_admin`. Once
the table contains any user, the JSON file is ignored and is never reapplied or
used to overwrite database users. A populated database can therefore restart
without the bootstrap file.

The bootstrap format remains:

```json
{
  "users": [
    {
      "id": "admin-alice",
      "username": "alice",
      "name": "Alice Administrator",
      "password_hash": "$2b$12$REPLACE_WITH_A_REAL_BCRYPT_HASH",
      "roles": ["platform_admin"]
    }
  ]
}
```

Protect this file as a deployment secret and never commit it. Plaintext
passwords are rejected in bootstrap configuration.

## User administration

Only `platform_admin` may call the user-management routes:

- `GET /api/v1/users`
- `POST /api/v1/users`
- `POST /api/v1/users/{id}/deactivate`

List and mutation responses never contain password hashes. Creation accepts a
username, one or more existing roles, and an initial plaintext password. The
server immediately generates the bcrypt hash; the plaintext is never stored,
logged, audited, or returned. Creation and role assignment are separate audit
events attributed to the acting administrator.

Deactivation preserves the user and its audit identity, rejects
self-deactivation, rejects deactivation of the last active `platform_admin`,
and invalidates all active sessions for the disabled user.

## Self-service password changes

`POST /api/v1/auth/change-password` requires an authenticated session, a valid
CSRF token, the current password, and a new password. A successful change
stores a new bcrypt hash, audits only that a change occurred, invalidates every
session for that user, clears the current cookie, and requires a fresh login.
Incorrect current credentials receive the same generic credential failure used
by login.

The login screen obtains a temporary session with the supplied username and
current password before invoking this authenticated endpoint. It does not show
whether a username exists.

## Sessions and deployment

Supported roles are `viewer`, `planner`, `operator`, `approver`,
`platform_admin`, and `auditor`. Assign separate named accounts; do not share an
administrator login.

Session cookies are HttpOnly and SameSite=Strict. Set
`DRISHTI_SESSION_SECURE=true` when HTTPS is in use; live and production modes
require it. For a separate UI origin, set exactly one trusted
`DRISHTI_ALLOWED_ORIGIN`.

Authenticated state-changing requests require the CSRF token returned by login
or `GET /api/v1/auth/session`. The browser UI handles this automatically.
Sessions remain process-local; a backend restart signs all users out without
removing their durable PostgreSQL accounts.
