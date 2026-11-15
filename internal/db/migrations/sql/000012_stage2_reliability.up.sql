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

-- 3. Ensure sync timestamps and indexes on synced entity tables
ALTER TABLE sims ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_sims_sync ON sims (user_id, updated_at);

ALTER TABLE sales ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_sales_sync ON sales (user_id, updated_at);

ALTER TABLE debts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_debts_sync ON debts (user_id, updated_at);

ALTER TABLE expenses ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_expenses_sync ON expenses (user_id, updated_at);
