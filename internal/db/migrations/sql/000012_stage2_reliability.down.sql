-- 000012_stage2_reliability.down.sql

DROP INDEX IF EXISTS idx_expenses_sync;
DROP INDEX IF EXISTS idx_debts_sync;
DROP INDEX IF EXISTS idx_sales_sync;
DROP INDEX IF EXISTS idx_sims_sync;

DROP TABLE IF EXISTS sync_tombstones CASCADE;
DROP TABLE IF EXISTS jobs CASCADE;
