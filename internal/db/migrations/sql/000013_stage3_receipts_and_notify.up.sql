-- 000013_stage3_receipts_and_notify.up.sql
-- Stage 3: Receipts, Notifications, Reports (seal, notify, reports)

-- 1. SEAL: Cryptographically signed receipt verification tokens with HMAC-SHA256
CREATE TABLE IF NOT EXISTS seal_tokens (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token         TEXT NOT NULL UNIQUE,
    receipt_type  TEXT NOT NULL DEFAULT 'voucher', -- voucher, sim, debt_payment
    receipt_id    TEXT NOT NULL,
    amount        BIGINT NOT NULL CHECK (amount >= 0),
    signature     TEXT NOT NULL,
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_seal_tokens_token ON seal_tokens (token);
CREATE INDEX IF NOT EXISTS idx_seal_tokens_merchant ON seal_tokens (merchant_id, receipt_type, receipt_id);

-- 2. NOTIFY: Unified channel configuration with fallback priority
CREATE TABLE IF NOT EXISTS notify_channels (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL DEFAULT 0, -- 0 for global system admin notifications
    channel_type  TEXT NOT NULL CHECK (channel_type IN ('telegram', 'whatsapp', 'sms', 'push')),
    config        JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority      INT NOT NULL DEFAULT 1,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notify_channels_lookup ON notify_channels (merchant_id, is_active, priority ASC);

CREATE TABLE IF NOT EXISTS notify_logs (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL DEFAULT 0,
    event_type    TEXT NOT NULL, -- topup, plan_subscription, stock_low, security_freeze, daily_summary
    channel_used  TEXT NOT NULL,
    recipient     TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL CHECK (status IN ('sent', 'failed', 'fallback')),
    error         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notify_logs_event ON notify_logs (event_type, created_at DESC);

-- 3. REPORTS: Daily financial summary tracking
CREATE TABLE IF NOT EXISTS daily_reports (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL DEFAULT 0,
    report_date   DATE NOT NULL,
    total_sales   BIGINT NOT NULL DEFAULT 0,
    total_profit  BIGINT NOT NULL DEFAULT 0,
    total_debts   BIGINT NOT NULL DEFAULT 0,
    active_sims   INT NOT NULL DEFAULT 0,
    pdf_path      TEXT NOT NULL DEFAULT '',
    sent_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, report_date)
);
CREATE INDEX IF NOT EXISTS idx_daily_reports_date ON daily_reports (merchant_id, report_date DESC);
