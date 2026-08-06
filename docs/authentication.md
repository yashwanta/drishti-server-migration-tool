# Session authentication and RBAC

The backend refuses to start without `DRISHTI_AUTH_USERS_FILE`. The file is a
deployment secret: restrict it to the backend service account and do not commit
it. Passwords must be bcrypt hashes; plaintext passwords are rejected.

```json
{
  "users": [
    {
      "id": "operator-alice",
      "username": "alice",
      "name": "Alice Operator",
      "email": "alice@example.invalid",
      "password_hash": "$2b$12$REPLACE_WITH_A_REAL_BCRYPT_HASH",
      "roles": ["operator"]
    }
  ]
}
```

Supported roles are `viewer`, `planner`, `operator`, `approver`,
`platform_admin`, and `auditor`. Assign separate named accounts; do not share an
administrator login. Session cookies are HttpOnly and SameSite=Strict. Set
`DRISHTI_SESSION_SECURE=true` when HTTPS is in use; live and production modes require it.
For a separate UI origin, set exactly one trusted `DRISHTI_ALLOWED_ORIGIN`.
Docker Compose mounts `DRISHTI_AUTH_USERS_HOST_FILE` read-only; its default is
`./secrets/drishti-auth-users.json`, which is ignored by Git.

Authenticated state-changing requests require the CSRF token returned by login
or `GET /api/v1/auth/session`. The browser UI handles this automatically. A
backend restart invalidates all sessions.
