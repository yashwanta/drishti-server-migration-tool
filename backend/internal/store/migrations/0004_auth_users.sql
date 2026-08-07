-- Durable operator identities for lab, live, and production modes. Passwords
-- are stored only as bcrypt hashes; active sessions remain process-local.

CREATE TABLE IF NOT EXISTS auth_users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE CHECK (username = lower(username)),
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    roles         JSONB NOT NULL DEFAULT '[]'::jsonb,
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_auth_users_active ON auth_users(active);
