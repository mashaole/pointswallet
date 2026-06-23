CREATE TABLE IF NOT EXISTS accounts (
    account_id     TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    email          TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('member', 'admin')),
    balance_points BIGINT NOT NULL DEFAULT 0 CHECK (balance_points >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_email_unique UNIQUE (email),
    CONSTRAINT accounts_email_format CHECK (
        email ~ '^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$'
    )
);

CREATE TABLE IF NOT EXISTS sessions (
    id         UUID PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(account_id),
    jti        TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sessions_account_id ON sessions (account_id);
CREATE INDEX IF NOT EXISTS idx_sessions_jti ON sessions (jti);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id         UUID PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(account_id),
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id                   BIGSERIAL PRIMARY KEY,
    ref                  TEXT NOT NULL UNIQUE,
    account_id           TEXT NOT NULL REFERENCES accounts(account_id),
    kind                 TEXT NOT NULL CHECK (kind IN ('earn', 'spend', 'adjustment')),
    points               BIGINT NOT NULL CHECK (points > 0),
    balance_after_points BIGINT NOT NULL CHECK (balance_after_points >= 0),
    occurred_at          TIMESTAMPTZ NOT NULL,
    recorded_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_account_id     TEXT NOT NULL REFERENCES accounts(account_id),
    source               TEXT NOT NULL CHECK (source IN ('api', 'batch'))
);

CREATE INDEX IF NOT EXISTS idx_ledger_account_recorded
    ON ledger_entries (account_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS batch_jobs (
    id              UUID PRIMARY KEY,
    status          TEXT NOT NULL CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    total_rows      INT NOT NULL DEFAULT 0,
    accepted_count  INT NOT NULL DEFAULT 0,
    rejected_count  INT NOT NULL DEFAULT 0,
    duplicate_count INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    error_message   TEXT
);

CREATE TABLE IF NOT EXISTS audit_events (
    id         BIGSERIAL PRIMARY KEY,
    batch_id   UUID NOT NULL REFERENCES batch_jobs(id),
    ref        TEXT,
    account_id TEXT,
    status     TEXT NOT NULL CHECK (status IN ('accepted', 'rejected')),
    reason     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_batch_id ON audit_events (batch_id, id);
