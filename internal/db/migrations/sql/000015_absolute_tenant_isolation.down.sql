-- 000015_absolute_tenant_isolation.down.sql
-- Rollback tenant isolation policies & constraints

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
        'restock_orders',
        'risk_profiles',
        'tills_shifts',
        'api_keys_v2',
        'surge_config',
        'printers',
        'staff',
        'tenant_files'
    ];
BEGIN
    FOREACH tbl IN ARRAY tenant_tables LOOP
        IF EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = tbl) THEN
            EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_policy ON %I', tbl);
            EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', tbl);
            EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', tbl);
        END IF;
    END LOOP;
END $$;

DROP TABLE IF EXISTS tenant_files CASCADE;
DROP TABLE IF EXISTS staff CASCADE;
DROP TABLE IF EXISTS printers CASCADE;
