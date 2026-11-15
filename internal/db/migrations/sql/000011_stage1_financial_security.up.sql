-- 000011_stage1_financial_security.up.sql
-- Stage 1: Financial Security (guard, idem, limits)

-- 1. GUARD: Item and category minimum margins to prevent below-cost selling
CREATE TABLE IF NOT EXISTS guard_settings (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id            TEXT NOT NULL DEFAULT '', -- voucher_id, card_id, or '*' for global merchant default
    min_margin_iqd     BIGINT NOT NULL DEFAULT 0 CHECK (min_margin_iqd >= 0),
    min_margin_percent INT NOT NULL DEFAULT 0 CHECK (min_margin_percent >= 0 AND min_margin_percent <= 100),
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_guard_settings_merchant ON guard_settings (merchant_id, item_id);

-- 2. IDEM: Strict idempotency key storage with advisory lock capability
CREATE TABLE IF NOT EXISTS idem_keys (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key                TEXT NOT NULL,
    merchant_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource           TEXT NOT NULL DEFAULT '', -- sell, pay, topup, refund
    status             TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
    status_code        INT NOT NULL DEFAULT 200,
    response           JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, key)
);
CREATE INDEX IF NOT EXISTS idx_idem_keys_lookup ON idem_keys (merchant_id, key, status);
CREATE INDEX IF NOT EXISTS idx_idem_keys_expires ON idem_keys (expires_at);

-- 3. LIMITS: Rate limit tracking, failed attempts, and temporary bans
CREATE TABLE IF NOT EXISTS limits_blocks (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id        BIGINT NOT NULL DEFAULT 0, -- 0 if unauthenticated IP block
    ip                 TEXT NOT NULL DEFAULT '',
    identifier         TEXT NOT NULL DEFAULT '', -- email, user_id, or token
    action_type        TEXT NOT NULL DEFAULT 'activation', -- activation, login, dispense, api
    failed_attempts    INT NOT NULL DEFAULT 1,
    blocked_until      TIMESTAMPTZ NOT NULL,
    reason             TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_limits_blocks_active ON limits_blocks (ip, merchant_id, blocked_until);
