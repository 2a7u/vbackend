-- 000004_vouchers_verification_gate.up.sql: Mandatory verification gate before applying final constraints
DO $$
DECLARE
    unencrypted_count   INT;
    missing_hmac_count  INT;
BEGIN
    SELECT COUNT(*) INTO unencrypted_count 
    FROM vouchers_vault 
    WHERE pin_encrypted = '' OR pin_encrypted IS NULL;

    SELECT COUNT(*) INTO missing_hmac_count 
    FROM vouchers_vault 
    WHERE pin_hmac = '' OR pin_hmac IS NULL;

    IF unencrypted_count > 0 OR missing_hmac_count > 0 THEN
        RAISE EXCEPTION 'MIGRATION VERIFICATION GATE FAILED: % unencrypted vouchers and % missing HMAC rows remain in vouchers_vault. Complete encryption before applying constraint gate.', unencrypted_count, missing_hmac_count;
    END IF;
END $$;
