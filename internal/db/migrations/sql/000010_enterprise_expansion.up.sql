-- 000010_enterprise_expansion.up.sql: Full Enterprise Platform Migration (Phases 3-8)
-- All money in BIGINT IQD. Strictly append-only migrations.

-- 1. Users table enterprise extensions
ALTER TABLE users ADD COLUMN IF NOT EXISTS device_id TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS device_model TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS app_version TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 1;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_device_locked BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_heartbeat TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_users_device_lookup ON users (device_id) WHERE device_id <> '';

-- 2. Bulk Subscriber Actions Log
CREATE TABLE IF NOT EXISTS bulk_actions_log (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id          UUID NOT NULL,
    action_type       TEXT NOT NULL,
    filters           JSONB NOT NULL DEFAULT '{}'::jsonb,
    subscribers_count INT NOT NULL DEFAULT 0,
    status            TEXT NOT NULL DEFAULT 'completed',
    details           JSONB NOT NULL DEFAULT '{}'::jsonb,
    executed_by       BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bulk_actions_batch ON bulk_actions_log (batch_id);

-- 3. Promotions: Coupons & Representative Codes
CREATE TABLE IF NOT EXISTS coupons (
    id                          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code                        TEXT NOT NULL UNIQUE,
    discount_type               TEXT NOT NULL DEFAULT 'percentage' CHECK (discount_type IN ('percentage', 'fixed')),
    discount_value              BIGINT NOT NULL CHECK (discount_value > 0),
    max_uses                    INT NOT NULL DEFAULT 100,
    used_count                  INT NOT NULL DEFAULT 0,
    max_uses_per_user           INT NOT NULL DEFAULT 1,
    representative_id           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    representative_commission_iqd BIGINT NOT NULL DEFAULT 0,
    expires_at                  TIMESTAMPTZ,
    is_active                   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons (code);

CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    coupon_id            BIGINT NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    discount_applied_iqd BIGINT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(coupon_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_user ON coupon_redemptions (user_id);

-- 4. Remote App Config & Feature Flags
CREATE TABLE IF NOT EXISTS app_config (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    config_key   TEXT NOT NULL UNIQUE,
    config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    version      INT NOT NULL DEFAULT 1,
    updated_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_app_config_key ON app_config (config_key);

-- Seed default remote config
INSERT INTO app_config (config_key, config_value, version)
VALUES
    ('maintenance_mode', '{"enabled": false, "message": "النظام يخضع للصيانة المجدولة، يرجى المحاولة لاحقاً", "whitelisted_uids": [1]}'::jsonb, 1),
    ('force_update', '{"min_version": "1.0.0", "latest_version": "1.2.0", "is_forced": false, "update_url": "https://urukiq.com/download"}'::jsonb, 1),
    ('feature_flags', '{"zaincash_gateway": true, "voucher_vault": true, "free_trial": true, "debt_management": true, "pos_credit_sales": true}'::jsonb, 1),
    ('announcement_ticker', '{"enabled": true, "ticker_text": "أهلاً بكم في منظومة VENUS السحابية المتكاملة", "link_url": ""}'::jsonb, 1),
    ('theme_studio', '{"primary_color": "#0058BC", "accent_color": "#3B82F6", "event_theme": "default", "contact_phone": "+9647700000000", "whatsapp_number": "+9647700000000"}'::jsonb, 1),
    ('support_templates', '{"activation_msg": "مرحباً بكم في شبكة فينوس! تم تفعيل اشتراككم بنجاح.", "reminder_msg": "عزيزي المشترك، يرجى العلم أن اشتراككم يوشك على الانتهاء."}'::jsonb, 1)
ON CONFLICT (config_key) DO NOTHING;

-- 5. Banners & Promo Sliders
CREATE TABLE IF NOT EXISTS app_banners (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title         TEXT NOT NULL DEFAULT '',
    image_url     TEXT NOT NULL,
    target_url    TEXT NOT NULL DEFAULT '',
    display_order INT NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    starts_at     TIMESTAMPTZ,
    ends_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 6. Remote Commands Whitelist & Queue
CREATE TABLE IF NOT EXISTS remote_commands (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    command_id  UUID NOT NULL UNIQUE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    command     TEXT NOT NULL CHECK (command IN ('force_sync', 'clear_cache', 'pull_diagnostics')),
    payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'acked', 'failed', 'expired')),
    result      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    acked_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_remote_commands_user ON remote_commands (user_id, status);

-- 7. Automation Engine & Rules
CREATE TABLE IF NOT EXISTS automation_rules (
    id                         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_name                  TEXT NOT NULL,
    trigger_event              TEXT NOT NULL,
    conditions                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active                  BOOLEAN NOT NULL DEFAULT TRUE,
    max_executions_per_target  INT NOT NULL DEFAULT 1,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS automation_executions (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_id          BIGINT NOT NULL REFERENCES automation_rules(id) ON DELETE CASCADE,
    target_user_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key  TEXT NOT NULL UNIQUE,
    executed_action  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_automation_exec_rule ON automation_executions (rule_id, target_user_id);

-- 8. WAF, Token-Bucket & IP Ban List
CREATE TABLE IF NOT EXISTS ip_rules (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ip_address     TEXT NOT NULL UNIQUE,
    rule_type      TEXT NOT NULL CHECK (rule_type IN ('whitelist', 'blacklist', 'auto_ban')),
    reason         TEXT NOT NULL DEFAULT '',
    ban_expires_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ip_rules_lookup ON ip_rules (ip_address, rule_type);

-- 9. Enterprise Webhooks with HMAC & Backoff
CREATE TABLE IF NOT EXISTS webhooks (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             TEXT NOT NULL,
    url              TEXT NOT NULL,
    events           TEXT[] NOT NULL,
    secret_token     TEXT NOT NULL DEFAULT '',
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    webhook_id      BIGINT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_name      TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_status INT,
    response_body   TEXT,
    attempts        INT NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'delivered' CHECK (status IN ('delivered', 'failed', 'retrying')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_wh ON webhook_deliveries (webhook_id, created_at DESC);

-- 10. External API Keys (hashed storage)
CREATE TABLE IF NOT EXISTS api_keys (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key_name     TEXT NOT NULL,
    key_prefix   TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,
    role         TEXT NOT NULL DEFAULT 'agent',
    scopes       TEXT[] NOT NULL DEFAULT '{}',
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys (key_hash);

-- 11. Business Intelligence (BI) Materialized Aggregates
CREATE TABLE IF NOT EXISTS materialized_daily_bi (
    report_date          DATE PRIMARY KEY,
    hourly_sales         JSONB NOT NULL DEFAULT '{}'::jsonb,
    governorate_sales    JSONB NOT NULL DEFAULT '{}'::jsonb,
    subscription_profit  BIGINT NOT NULL DEFAULT 0,
    vouchers_profit      BIGINT NOT NULL DEFAULT 0,
    total_net_profit     BIGINT NOT NULL DEFAULT 0,
    active_subscribers   INT NOT NULL DEFAULT 0,
    churned_subscribers  INT NOT NULL DEFAULT 0,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
