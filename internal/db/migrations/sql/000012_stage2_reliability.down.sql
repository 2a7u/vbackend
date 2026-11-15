-- 000012_stage2_reliability.down.sql

DROP VIEW IF EXISTS expenses;
DROP VIEW IF EXISTS debts;
DROP VIEW IF EXISTS sales;
DROP VIEW IF EXISTS sims;

DROP INDEX IF EXISTS idx_store_expenses_sync;
DROP INDEX IF EXISTS idx_debt_contracts_sync;
DROP INDEX IF EXISTS idx_sim_sales_sync;
DROP INDEX IF EXISTS idx_sim_inventory_sync;

DROP INDEX IF EXISTS idx_expenses_sync;
DROP INDEX IF EXISTS idx_debts_sync;
DROP INDEX IF EXISTS idx_sales_sync;
DROP INDEX IF EXISTS idx_sims_sync;

DROP TABLE IF EXISTS sync_tombstones CASCADE;
DROP TABLE IF EXISTS jobs CASCADE;
