-- Soft delete: preserve ledger/auth FK references; hide accounts from active queries.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_email_unique;

CREATE UNIQUE INDEX IF NOT EXISTS accounts_email_active_unique
    ON accounts (email) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_active_created
    ON accounts (created_at ASC, account_id ASC) WHERE deleted_at IS NULL;
