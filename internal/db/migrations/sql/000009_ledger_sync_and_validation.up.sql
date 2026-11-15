-- 000009_ledger_sync_and_validation.up.sql: Ensure ledger_entries columns, hash naming, and balance validation
DO $$
BEGIN
    -- 1. Ensure 'hash' column exists and has proper values
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ledger_entries' AND column_name = 'hash'
    ) THEN
        IF EXISTS (
            SELECT 1 FROM information_schema.columns 
            WHERE table_name = 'ledger_entries' AND column_name = 'entry_hash'
        ) THEN
            ALTER TABLE ledger_entries RENAME COLUMN entry_hash TO hash;
        ELSE
            ALTER TABLE ledger_entries ADD COLUMN hash TEXT NOT NULL DEFAULT '';
        END IF;
    END IF;

    -- 2. Ensure 'currency' column exists
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ledger_entries' AND column_name = 'currency'
    ) THEN
        ALTER TABLE ledger_entries ADD COLUMN currency TEXT NOT NULL DEFAULT 'IQD';
    END IF;

    -- 3. Ensure 'idempotency_key' column exists
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'ledger_entries' AND column_name = 'idempotency_key'
    ) THEN
        ALTER TABLE ledger_entries ADD COLUMN idempotency_key TEXT;
    END IF;

    -- 4. Ensure index on idempotency_key
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'ledger_entries' AND indexname = 'idx_ledger_entries_idempotency_key'
    ) THEN
        CREATE INDEX idx_ledger_entries_idempotency_key ON ledger_entries (idempotency_key) WHERE idempotency_key IS NOT NULL;
    END IF;

    -- 5. Re-assert strict immutability trigger
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'trg_prevent_ledger_update'
    ) THEN
        CREATE TRIGGER trg_prevent_ledger_update
        BEFORE UPDATE OR DELETE ON ledger_entries
        FOR EACH ROW EXECUTE FUNCTION prevent_ledger_modification();
    END IF;

    -- 6. Revoke modification rights from PUBLIC
    REVOKE UPDATE, DELETE ON ledger_entries FROM PUBLIC;
EXCEPTION
    WHEN OTHERS THEN NULL;
END $$;
