-- 000002_fintech_ledger.down.sql
DROP TABLE IF EXISTS fraud_events;
DROP TABLE IF EXISTS fraud_rules;
DROP TABLE IF EXISTS idempotency_records;
DROP TABLE IF EXISTS audit_logs;
DROP TRIGGER IF EXISTS trg_prevent_ledger_modification ON ledger_entries;
DROP FUNCTION IF EXISTS prevent_ledger_modification;
DROP TABLE IF EXISTS ledger_entries;
