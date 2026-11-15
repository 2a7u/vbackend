-- 000005_vouchers_vault_constraints.down.sql
DROP INDEX IF EXISTS idx_vouchers_vault_pin_hmac_total;
CREATE INDEX IF NOT EXISTS idx_vouchers_vault_pin_hmac_partial ON vouchers_vault (pin_hmac) WHERE pin_hmac <> '';
ALTER TABLE vouchers_vault ALTER COLUMN pin_hmac DROP NOT NULL;
