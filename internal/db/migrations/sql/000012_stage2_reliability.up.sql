-- 000012_stage2_reliability.up.sql
-- Stage 2: Reliability (jobs, refund, sync)

-- 1. JOBS: Background reliable worker queue with FOR UPDATE SKIP LOCKED
CREATE TABLE IF NOT EXISTS jobs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL, -- zaincash_poll, refund_retry, notify_retry, etc.
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'dead')),
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 10,
    next_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_jobs_pending ON jobs (status, next_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_dead ON jobs (status, created_at) WHERE status = 'dead';
CREATE INDEX IF NOT EXISTS idx_jobs_merchant ON jobs (merchant_id, status);

-- 2. SYNC TOMBSTONES: Track entity deletions for delta synchronization
CREATE TABLE IF NOT EXISTS sync_tombstones (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    table_name    TEXT NOT NULL,
    record_id     TEXT NOT NULL,
    deleted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sync_tombstones_cursor ON sync_tombstones (merchant_id, deleted_at);

-- 3. Ensure sync timestamps and indexes on synced entity tables (sim_inventory, sim_sales, debt_contracts, store_expenses)
ALTER TABLE sim_inventory ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_sim_inventory_sync ON sim_inventory (user_id, updated_at);

ALTER TABLE sim_sales ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_sim_sales_sync ON sim_sales (user_id, updated_at);

ALTER TABLE debt_contracts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_debt_contracts_sync ON debt_contracts (user_id, updated_at);

ALTER TABLE store_expenses ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_store_expenses_sync ON store_expenses (user_id, updated_at);

-- 4. Compatibility Views or Table Alteration for legacy sync queries
DO $$
BEGIN
    -- If legacy tables exist, add updated_at and indexes
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'sims') THEN
        ALTER TABLE sims ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
        CREATE INDEX IF NOT EXISTS idx_sims_sync ON sims (user_id, updated_at);
    ELSE
        -- Otherwise create the compatibility view
        CREATE OR REPLACE VIEW sims AS
            SELECT id, user_id, barcode, COALESCE(phone_number, '') AS phone_number,
                   ''::text AS serial_number, operator, 'prepaid'::text AS sim_type,
                   purchase_price AS buy_price, selling_price AS sell_price,
                   (status = 'sold') AS is_sold, created_at, COALESCE(updated_at, created_at) AS updated_at
            FROM sim_inventory;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'sales') THEN
        ALTER TABLE sales ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
        CREATE INDEX IF NOT EXISTS idx_sales_sync ON sales (user_id, updated_at);
    ELSE
        CREATE OR REPLACE VIEW sales AS
            SELECT id, user_id, COALESCE(sim_id, 0) AS sim_id, barcode,
                   ''::text AS phone_number, operator, selling_price AS sale_price,
                   purchase_price AS cost_price, profit,
                   ''::text AS customer_name, ''::text AS customer_phone,
                   false AS is_credit, created_at, COALESCE(updated_at, created_at) AS updated_at
            FROM sim_sales;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'debts') THEN
        ALTER TABLE debts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
        CREATE INDEX IF NOT EXISTS idx_debts_sync ON debts (user_id, updated_at);
    ELSE
        CREATE OR REPLACE VIEW debts AS
            SELECT id, user_id, debtor_name AS customer_name, phone AS customer_phone,
                   total_amount, paid_amount, (total_amount - paid_amount) AS remaining_amount,
                   (status = 'completed') AS is_paid, due_date, created_at, COALESCE(updated_at, created_at) AS updated_at
            FROM debt_contracts;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'expenses') THEN
        ALTER TABLE expenses ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
        CREATE INDEX IF NOT EXISTS idx_expenses_sync ON expenses (user_id, updated_at);
    ELSE
        CREATE OR REPLACE VIEW expenses AS
            SELECT id, user_id, category, amount,
                   (title || ' ' || notes) AS description, created_at, COALESCE(updated_at, created_at) AS updated_at
            FROM store_expenses;
    END IF;
END $$;
