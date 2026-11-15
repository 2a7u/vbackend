-- 000003_vouchers_encryption.down.sql
DROP INDEX IF EXISTS idx_vouchers_vault_pin_hmac_partial;
DROP TABLE IF EXISTS legacy_migration_duplicates;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS net_profit_recorded_at;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS net_profit;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS pin_hmac;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS key_version;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS nonce;
ALTER TABLE vouchers_vault DROP COLUMN IF EXISTS pin_encrypted;
