-- 000005_vouchers_vault_constraints.up.sql: Total duplicate guard & complete plaintext wipe
-- 1. Wipe plaintext pin values
UPDATE vouchers_vault 
SET pin = '' 
WHERE pin_encrypted <> '' AND pin_hmac <> '';

-- 2. Relax plaintext pin column
ALTER TABLE vouchers_vault ALTER COLUMN pin DROP NOT NULL;
ALTER TABLE vouchers_vault ALTER COLUMN pin SET DEFAULT '';

-- 3. Enforce strict HMAC presence
ALTER TABLE vouchers_vault ALTER COLUMN pin_hmac SET NOT NULL;

-- 4. Replace partial index with full UNIQUE index across the entire table
DROP INDEX IF EXISTS idx_vouchers_vault_pin_hmac_partial;
DROP INDEX IF EXISTS idx_vouchers_vault_pin_hmac;
CREATE UNIQUE INDEX IF NOT EXISTS idx_vouchers_vault_pin_hmac_total ON vouchers_vault (pin_hmac);
