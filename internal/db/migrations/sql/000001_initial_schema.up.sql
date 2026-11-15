-- 000001_initial_schema.up.sql: Baseline UPOS and Venus schema
CREATE TABLE IF NOT EXISTS users (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email              TEXT NOT NULL UNIQUE,
    password_hash      TEXT NOT NULL,
    full_name          TEXT NOT NULL DEFAULT '',
    store_name         TEXT NOT NULL DEFAULT '',
    phone              TEXT NOT NULL DEFAULT '',
    role               TEXT NOT NULL DEFAULT 'subscriber' CHECK (role IN ('superadmin', 'admin', 'supervisor', 'agent', 'subscriber', 'system')),
    wallet_balance     BIGINT NOT NULL DEFAULT 0,
    credit_limit       BIGINT NOT NULL DEFAULT 0 CHECK (credit_limit >= 0),
    wallet_status      TEXT NOT NULL DEFAULT 'active' CHECK (wallet_status IN ('active', 'frozen', 'restricted')),
    is_approved        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_wallet_balance_check CHECK (role = 'system' OR wallet_balance >= -credit_limit)
);

CREATE TABLE IF NOT EXISTS subscription_plans (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    plan_key           TEXT NOT NULL UNIQUE,
    title              TEXT NOT NULL,
    original_price_iqd BIGINT NOT NULL DEFAULT 0,
    price_iqd          BIGINT NOT NULL DEFAULT 0,
    discount_percent   INT NOT NULL DEFAULT 0,
    duration_days      INT NOT NULL DEFAULT 30,
    badge_text         TEXT NOT NULL DEFAULT '',
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vouchers_vault (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    brand_id           TEXT NOT NULL,
    voucher_id         TEXT NOT NULL,
    pin                TEXT DEFAULT '',
    serial_number      TEXT NOT NULL,
    cost_price         DOUBLE PRECISION NOT NULL DEFAULT 0,
    selling_price      DOUBLE PRECISION NOT NULL DEFAULT 0,
    is_sold            BOOLEAN NOT NULL DEFAULT FALSE,
    sold_to_store_name TEXT NOT NULL DEFAULT '',
    sold_at            TIMESTAMPTZ,
    reprint_count      INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_vouchers_vault_available ON vouchers_vault (voucher_id, id) WHERE is_sold = FALSE;

CREATE TABLE IF NOT EXISTS system_settings (
    key                TEXT PRIMARY KEY,
    value              TEXT NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
