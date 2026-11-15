-- 000003_vouchers_encryption.up.sql: Vouchers encryption vault columns & duplicate reporting table
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'available';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS pin_encrypted TEXT NOT NULL DEFAULT '';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS nonce TEXT NOT NULL DEFAULT '';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS key_version INT NOT NULL DEFAULT 1;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS pin_hmac TEXT NOT NULL DEFAULT '';
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS net_profit BIGINT NOT NULL DEFAULT 0;
ALTER TABLE vouchers_vault ADD COLUMN IF NOT EXISTS net_profit_recorded_at TIMESTAMPTZ;

-- Table to catch and report legacy duplicates during migration without crashing
CREATE TABLE IF NOT EXISTS legacy_migration_duplicates (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    legacy_voucher_id BIGINT,
    serial_number     TEXT NOT NULL DEFAULT '',
    pin_hmac          TEXT NOT NULL,
    reason            TEXT NOT NULL DEFAULT '',
    pin_encrypted     TEXT NOT NULL DEFAULT '',
    nonce             TEXT NOT NULL DEFAULT '',
    detected_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial index for fast duplicate checks during encryption phase
CREATE INDEX IF NOT EXISTS idx_vouchers_vault_pin_hmac_partial ON vouchers_vault (pin_hmac) WHERE pin_hmac <> '';
