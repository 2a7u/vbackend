-- 000014_stage4_growth_and_governance.up.sql
-- Stage 4: Growth & Governance (rewards, restock, risk, forecast, tills, api, surge)

-- 1. REWARDS: Sales target bonus rules and idempotent reward postings
CREATE TABLE IF NOT EXISTS rewards_rules (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id       BIGINT NOT NULL DEFAULT 0, -- 0 for platform global rule
    title             TEXT NOT NULL,
    target_sales_iqd  BIGINT NOT NULL CHECK (target_sales_iqd > 0),
    reward_amount_iqd BIGINT NOT NULL CHECK (reward_amount_iqd > 0),
    period_type       TEXT NOT NULL DEFAULT 'weekly' CHECK (period_type IN ('daily', 'weekly', 'monthly')),
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rewards_ledger (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id            BIGINT NOT NULL REFERENCES rewards_rules(id) ON DELETE CASCADE,
    period_key         TEXT NOT NULL, -- e.g. '2026-W46', '2026-11'
    achieved_sales_iqd BIGINT NOT NULL,
    reward_amount_iqd  BIGINT NOT NULL,
    ledger_tx_id       TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, rule_id, period_key)
);
CREATE INDEX IF NOT EXISTS idx_rewards_ledger_merchant ON rewards_ledger (merchant_id, period_key);

-- 2. RESTOCK: External stockout replenishment provider interface
CREATE TABLE IF NOT EXISTS restock_providers (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id   BIGINT NOT NULL DEFAULT 0,
    name          TEXT NOT NULL,
    provider_type TEXT NOT NULL DEFAULT 'mock' CHECK (provider_type IN ('mock', 'asiacell_b2b', 'zain_direct', 'external_http')),
    config        JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS restock_orders (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id       BIGINT NOT NULL REFERENCES restock_providers(id) ON DELETE CASCADE,
    item_id           TEXT NOT NULL,
    quantity          INT NOT NULL CHECK (quantity > 0),
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ordered', 'completed', 'failed')),
    external_order_id TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_restock_orders_merchant ON restock_orders (merchant_id, status);

-- 3. RISK: Risk scoring engine profiles and anomaly events
CREATE TABLE IF NOT EXISTS risk_profiles (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    risk_score         INT NOT NULL DEFAULT 0 CHECK (risk_score >= 0 AND risk_score <= 100),
    velocity_count     INT NOT NULL DEFAULT 0,
    failed_auth_count  INT NOT NULL DEFAULT 0,
    suspicious_flags   JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_auto_frozen     BOOLEAN NOT NULL DEFAULT FALSE,
    last_evaluated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_risk_profiles_score ON risk_profiles (risk_score DESC);

-- 4. TILLS: Cashier shifts, device binding, and shift closures
CREATE TABLE IF NOT EXISTS tills_shifts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cashier_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id      TEXT NOT NULL DEFAULT '',
    opened_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at      TIMESTAMPTZ,
    starting_cash  BIGINT NOT NULL DEFAULT 0,
    expected_cash  BIGINT NOT NULL DEFAULT 0,
    actual_cash    BIGINT NOT NULL DEFAULT 0,
    discrepancy    BIGINT NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    note           TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_tills_shifts_active ON tills_shifts (merchant_id, cashier_id, status);

-- 5. API KEYS V2: External hashed partner API keys with scopes
CREATE TABLE IF NOT EXISTS api_keys_v2 (
    id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label                 TEXT NOT NULL,
    key_hash              TEXT NOT NULL UNIQUE,
    prefix                TEXT NOT NULL,
    scopes                TEXT[] NOT NULL DEFAULT '{"read"}',
    rate_limit_per_minute INT NOT NULL DEFAULT 60,
    usage_count           BIGINT NOT NULL DEFAULT 0,
    last_used_at          TIMESTAMPTZ,
    is_active             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_api_keys_v2_lookup ON api_keys_v2 (key_hash, is_active);

-- 6. SURGE PRICING: Dynamic pricing cap configuration
CREATE TABLE IF NOT EXISTS surge_config (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id        BIGINT NOT NULL DEFAULT 0, -- 0 for platform default
    is_enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    multiplier_percent INT NOT NULL DEFAULT 0 CHECK (multiplier_percent >= 0),
    max_cap_percent    INT NOT NULL DEFAULT 15 CHECK (max_cap_percent >= 0 AND max_cap_percent <= 50),
    reason             TEXT NOT NULL DEFAULT '',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
