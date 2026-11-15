-- 000002_fintech_ledger.up.sql: Double-entry ledger, immutability, audit trail & idempotency
CREATE TABLE IF NOT EXISTS ledger_entries (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tx_id          UUID NOT NULL,
    account_id     BIGINT NOT NULL REFERENCES users(id),
    debit          BIGINT NOT NULL DEFAULT 0 CHECK (debit >= 0),
    credit         BIGINT NOT NULL DEFAULT 0 CHECK (credit >= 0),
    balance_after  BIGINT NOT NULL,
    ref_type       TEXT NOT NULL,
    ref_id         TEXT NOT NULL,
    prev_hash      TEXT NOT NULL DEFAULT '',
    entry_hash     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_debit_or_credit CHECK ((debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0))
);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_account ON ledger_entries (account_id, id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_tx ON ledger_entries (tx_id);

-- Enforce append-only immutability trigger
CREATE OR REPLACE FUNCTION prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'CRITICAL: ledger_entries is strictly append-only. UPDATE and DELETE operations are forbidden.';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_ledger_modification ON ledger_entries;
CREATE TRIGGER trg_prevent_ledger_modification
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW EXECUTE FUNCTION prevent_ledger_modification();

-- Audit log
CREATE TABLE IF NOT EXISTS audit_logs (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    user_email     TEXT NOT NULL DEFAULT '',
    user_name      TEXT NOT NULL DEFAULT '',
    store_name     TEXT NOT NULL DEFAULT '',
    action         TEXT NOT NULL,
    entity         TEXT NOT NULL DEFAULT '',
    entity_id      TEXT NOT NULL DEFAULT '',
    details        JSONB NOT NULL DEFAULT '{}'::jsonb,
    ip_address     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_id BIGINT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_email TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS resource_type TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS resource_id TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_audit_logs_user ON audit_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action, created_at DESC);

-- Idempotency keys
CREATE TABLE IF NOT EXISTS idempotency_records (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key            TEXT NOT NULL UNIQUE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_path   TEXT NOT NULL,
    payload_hash   TEXT NOT NULL,
    response_code  INT NOT NULL,
    response_body  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_idempotency_lookup ON idempotency_records (key, user_id);

-- Fraud rules & events
CREATE TABLE IF NOT EXISTS fraud_rules (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_code           TEXT NOT NULL UNIQUE,
    name_ar             TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    condition_type      TEXT NOT NULL,
    threshold_value     BIGINT NOT NULL DEFAULT 0,
    time_window_seconds INT NOT NULL DEFAULT 60,
    action              TEXT NOT NULL DEFAULT 'alert' CHECK (action IN ('alert', 'freeze_wallet', 'require_otp')),
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fraud_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_code    TEXT NOT NULL,
    severity     TEXT NOT NULL DEFAULT 'high' CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    action_taken TEXT NOT NULL DEFAULT 'alert',
    details      JSONB NOT NULL DEFAULT '{}'::jsonb,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewed', 'dismissed')),
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at  TIMESTAMPTZ,
    reviewed_by  BIGINT REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_fraud_events_user ON fraud_events (user_id, triggered_at DESC);
