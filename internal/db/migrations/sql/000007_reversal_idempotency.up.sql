-- 000007_reversal_idempotency.up.sql: DB-enforced idempotency for general reversals & wallet refunds
DROP INDEX IF EXISTS idx_ledger_entries_unique_reversal;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_entries_unique_reversal_tx
ON ledger_entries (ref_type, ref_id, account_id)
WHERE ref_type IN ('reversal', 'refund', 'agent_commission_reversal');
