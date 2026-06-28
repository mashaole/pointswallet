-- Ledger direction for audit: credit (add) or debit (subtract).
-- Idempotent: safe to re-run on server startup.

ALTER TABLE ledger_entries
    ADD COLUMN IF NOT EXISTS direction TEXT;

UPDATE ledger_entries SET direction = 'debit'
WHERE direction IS NULL AND kind = 'spend';

UPDATE ledger_entries SET direction = 'credit'
WHERE direction IS NULL AND kind IN ('earn', 'adjustment');

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'ledger_entries'
          AND column_name = 'direction'
          AND is_nullable = 'YES'
    ) THEN
        ALTER TABLE ledger_entries ALTER COLUMN direction SET NOT NULL;
    END IF;
END $$;

ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_direction_check;

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_entries_direction_check
    CHECK (direction IN ('credit', 'debit'));
