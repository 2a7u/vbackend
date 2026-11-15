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

-- Reconcile legacy production columns on existing users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_balance BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS credit_limit BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'subscriber';
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_approved BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS store_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS address TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS plan_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS subscription_ends_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS parent_agent_id BIGINT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS price_tier_id BIGINT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS zaincash_wallet_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

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

-- Reconcile legacy production columns on existing subscription_plans table
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS original_price_iqd BIGINT NOT NULL DEFAULT 0;
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS price_iqd BIGINT NOT NULL DEFAULT 0;
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS discount_percent INT NOT NULL DEFAULT 0;
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS duration_days INT NOT NULL DEFAULT 30;
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS badge_text TEXT NOT NULL DEFAULT '';
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

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

-- Reconcile legacy production columns on existing vouchers_vault table
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS is_sold BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'available';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_to_user_id BIGINT;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_to_store_name TEXT NOT NULL DEFAULT '';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_at TIMESTAMPTZ;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS sold_price BIGINT NOT NULL DEFAULT 0;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS reprint_count INT NOT NULL DEFAULT 0;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS cost_price DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS selling_price DOUBLE PRECISION NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_vouchers_vault_available ON vouchers_vault (voucher_id, id) WHERE is_sold = FALSE;

CREATE TABLE IF NOT EXISTS system_settings (
    key                TEXT PRIMARY KEY,
    value              TEXT NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
