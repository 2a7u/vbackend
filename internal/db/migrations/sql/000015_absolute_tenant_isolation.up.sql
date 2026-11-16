-- 000015_absolute_tenant_isolation.up.sql
-- Enterprise Absolute Tenant Isolation with Row Level Security (RLS) & FORCE RLS

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1. Identity & Users Tenant Mapping
ALTER TABLE users ADD COLUMN IF NOT EXISTS merchant_id UUID NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_merchant_id ON users (merchant_id);

-- 2. Ensure auxiliary tenant tables exist (printers, staff, tenant_files)
CREATE TABLE IF NOT EXISTS printers (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id  UUID NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    printer_type TEXT NOT NULL DEFAULT 'esc_pos',
    mac_address  TEXT NOT NULL DEFAULT '',
    paper_width  INT NOT NULL DEFAULT 58,
    is_default   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS staff (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id    UUID NOT NULL,
    user_id        BIGINT NOT NULL,
    name           TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'cashier',
    phone          TEXT NOT NULL DEFAULT '',
    pin_hash       TEXT NOT NULL DEFAULT '',
    can_view_costs BOOLEAN NOT NULL DEFAULT FALSE,
    can_view_profits BOOLEAN NOT NULL DEFAULT FALSE,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_files (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id UUID NOT NULL,
    file_path   TEXT NOT NULL,
    file_name   TEXT NOT NULL,
    mime_type   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. Dynamic idempotent procedure to convert, index, and apply RLS across all tenant tables
DO $$
DECLARE
    tbl text;
    tenant_tables text[] := ARRAY[
        'sim_inventory',
        'sim_sales',
        'credit_sales',
        'debt_contracts',
        'debt_payments',
        'store_expenses',
        'support_tickets',
        'subscriber_backups',
        'audit_logs',
        'login_history',
        'wallet_ledger',
        'subscriptions',
        'zaincash_transactions',
        'vouchers_vault',
        'ledger_entries',
        'integrity_alerts',
        'fraud_events',
        'reconciliation_reports',
        'financial_closes',
        'idempotency_keys',
        'subscriber_price_overrides',
        'agent_commissions',
        'bulk_actions_log',
        'coupons',
        'coupon_redemptions',
        'remote_commands',
        'automation_rules',
        'automation_executions',
        'webhooks',
        'webhook_deliveries',
        'api_keys',
        'guard_settings',
        'idem_keys',
        'limits_blocks',
        'jobs',
        'sync_tombstones',
        'seal_tokens',
        'notify_channels',
        'notify_logs',
        'daily_reports',
        'rewards_rules',
        'rewards_ledger',
        'restock_providers',
        'restock_orders',
        'risk_profiles',
        'tills_shifts',
        'api_keys_v2',
        'surge_config',
        'printers',
        'staff',
        'tenant_files'
    ];
    col_type text;
BEGIN
    FOREACH tbl IN ARRAY tenant_tables LOOP
        -- Check if table exists in public schema
        IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = tbl) THEN

            -- Check if merchant_id column exists
            SELECT data_type INTO col_type
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = tbl AND column_name = 'merchant_id';

            IF col_type IS NULL THEN
                -- Add merchant_id as UUID
                EXECUTE format('ALTER TABLE %I ADD COLUMN merchant_id UUID', tbl);

                -- Backfill merchant_id from users table if user_id column exists
                IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = tbl AND column_name = 'user_id') THEN
                    EXECUTE format('UPDATE %I t SET merchant_id = u.merchant_id FROM users u WHERE t.user_id = u.id AND t.merchant_id IS NULL', tbl);
                END IF;

                -- For any remaining rows, assign a default UUID
                EXECUTE format('UPDATE %I SET merchant_id = gen_random_uuid() WHERE merchant_id IS NULL', tbl);
                EXECUTE format('ALTER TABLE %I ALTER COLUMN merchant_id SET DEFAULT gen_random_uuid()', tbl);
                EXECUTE format('ALTER TABLE %I ALTER COLUMN merchant_id SET NOT NULL', tbl);

            ELSIF col_type <> 'uuid' THEN
                -- Convert existing non-UUID merchant_id column to UUID safely
                EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS merchant_uuid UUID', tbl);

                -- Backfill matching users.id
                EXECUTE format('UPDATE %I t SET merchant_uuid = u.merchant_id FROM users u WHERE t.merchant_id = u.id', tbl);
                EXECUTE format('UPDATE %I SET merchant_uuid = gen_random_uuid() WHERE merchant_uuid IS NULL', tbl);

                -- Drop old constraints and swap
                EXECUTE format('ALTER TABLE %I DROP COLUMN merchant_id CASCADE', tbl);
                EXECUTE format('ALTER TABLE %I RENAME COLUMN merchant_uuid TO merchant_id', tbl);
                EXECUTE format('ALTER TABLE %I ALTER COLUMN merchant_id SET DEFAULT gen_random_uuid()', tbl);
                EXECUTE format('ALTER TABLE %I ALTER COLUMN merchant_id SET NOT NULL', tbl);
            END IF;

            -- Create Index starting with merchant_id
            EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (merchant_id)', 'idx_' || tbl || '_merchant_id', tbl);

            -- Add composite UNIQUE (merchant_id, id) constraint if table has an id column
            IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = tbl AND column_name = 'id') THEN
                IF NOT EXISTS (
                    SELECT 1 FROM pg_constraint
                    WHERE conrelid = tbl::regclass AND conname = 'uq_' || tbl || '_merchant_id_id'
                ) THEN
                    BEGIN
                        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I UNIQUE (merchant_id, id)', tbl, 'uq_' || tbl || '_merchant_id_id');
                    EXCEPTION WHEN OTHERS THEN
                        -- Ignore if already covered by another constraint
                        NULL;
                    END;
                END IF;
            END IF;

            -- Enable RLS and FORCE RLS on table
            EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', tbl);
            EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', tbl);

            -- Drop old policy if exists and create strict tenant_isolation_policy
            EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_policy ON %I', tbl);
            EXECUTE format(
                'CREATE POLICY tenant_isolation_policy ON %I ' ||
                'FOR ALL TO PUBLIC ' ||
                'USING (merchant_id = NULLIF(current_setting(''app.merchant_id'', true), '''')::uuid) ' ||
                'WITH CHECK (merchant_id = NULLIF(current_setting(''app.merchant_id'', true), '''')::uuid)',
                tbl
            );

        END IF;
    END LOOP;
END $$;

-- 4. Tenant-scoped Unique Constraints
DO $$
BEGIN
    -- sim_inventory: UNIQUE(merchant_id, barcode)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'sim_inventory') THEN
        ALTER TABLE sim_inventory DROP CONSTRAINT IF EXISTS sim_inventory_user_id_barcode_key;
        CREATE UNIQUE INDEX IF NOT EXISTS uq_sim_inventory_merchant_barcode ON sim_inventory (merchant_id, barcode);
    END IF;

    -- vouchers_vault: UNIQUE(merchant_id, serial_number)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'vouchers_vault') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_vouchers_vault_merchant_serial ON vouchers_vault (merchant_id, serial_number);
    END IF;

    -- idem_keys: UNIQUE(merchant_id, key)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'idem_keys') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_idem_keys_merchant_key ON idem_keys (merchant_id, key);
    END IF;

    -- rewards_ledger: UNIQUE(merchant_id, rule_id, period_key)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'rewards_ledger') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_rewards_ledger_merchant_rule ON rewards_ledger (merchant_id, rule_id, period_key);
    END IF;

    -- guard_settings: UNIQUE(merchant_id, item_id)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'guard_settings') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_guard_settings_merchant_item ON guard_settings (merchant_id, item_id);
    END IF;

    -- surge_config: UNIQUE(merchant_id)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'surge_config') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_surge_config_merchant ON surge_config (merchant_id);
    END IF;

    -- risk_profiles: UNIQUE(merchant_id)
    IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = 'risk_profiles') THEN
        CREATE UNIQUE INDEX IF NOT EXISTS uq_risk_profiles_merchant ON risk_profiles (merchant_id);
    END IF;
END $$;

-- 5. Create app_user Role for Application Execution (NOBYPASSRLS, non-superuser)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        BEGIN
            CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user_secure_pwd' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE NOTICE 'Role app_user could not be created due to privileges; continuing';
        END;
    END IF;
END $$;

-- Grant permissions to app_user if role exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        GRANT USAGE ON SCHEMA public TO app_user;
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
        GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_user;
        ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO app_user;
    END IF;
END $$;
