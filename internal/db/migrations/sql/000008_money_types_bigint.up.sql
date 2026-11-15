-- 000008_money_types_bigint.up.sql: Convert money columns to BIGINT in Iraqi Dinar (zero float tolerance)
DO $$
DECLARE
    v_cost_frac INT;
    v_sell_frac INT;
    v_cost_sum_before NUMERIC;
    v_sell_sum_before NUMERIC;
    v_cost_sum_after BIGINT;
    v_sell_sum_after BIGINT;
BEGIN
    -- 1. Fractional check and report
    SELECT 
        COUNT(*) FILTER (WHERE cost_price::numeric % 1 <> 0),
        COUNT(*) FILTER (WHERE selling_price::numeric % 1 <> 0),
        COALESCE(SUM(cost_price::numeric), 0),
        COALESCE(SUM(selling_price::numeric), 0)
    INTO v_cost_frac, v_sell_frac, v_cost_sum_before, v_sell_sum_before
    FROM vouchers_vault;

    RAISE NOTICE 'Migration 000008 Money Check: cost_price fractional rows=%, selling_price fractional rows=%, sum_cost_before=%, sum_sell_before=%',
        v_cost_frac, v_sell_frac, v_cost_sum_before, v_sell_sum_before;

    -- 2. Convert cost_price to BIGINT using ROUND
    ALTER TABLE vouchers_vault ALTER COLUMN cost_price TYPE BIGINT USING ROUND(cost_price)::BIGINT;
    ALTER TABLE vouchers_vault ALTER COLUMN cost_price SET DEFAULT 0;

    -- 3. Convert selling_price to BIGINT using ROUND
    ALTER TABLE vouchers_vault ALTER COLUMN selling_price TYPE BIGINT USING ROUND(selling_price)::BIGINT;
    ALTER TABLE vouchers_vault ALTER COLUMN selling_price SET DEFAULT 0;

    -- 4. Convert sold_price to BIGINT if present
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'vouchers_vault' AND column_name = 'sold_price'
    ) THEN
        ALTER TABLE vouchers_vault ALTER COLUMN sold_price TYPE BIGINT USING ROUND(COALESCE(sold_price, 0))::BIGINT;
        ALTER TABLE vouchers_vault ALTER COLUMN sold_price SET DEFAULT 0;
    END IF;

    -- 5. Before/after sum integrity check
    SELECT COALESCE(SUM(cost_price), 0), COALESCE(SUM(selling_price), 0)
    INTO v_cost_sum_after, v_sell_sum_after
    FROM vouchers_vault;

    IF ROUND(v_cost_sum_before) <> v_cost_sum_after OR ROUND(v_sell_sum_before) <> v_sell_sum_after THEN
        RAISE EXCEPTION 'CRITICAL MIGRATION ERROR: Sum integrity mismatch after BIGINT conversion: cost % vs %, sell % vs %',
            ROUND(v_cost_sum_before), v_cost_sum_after, ROUND(v_sell_sum_before), v_sell_sum_after;
    END IF;
END $$;
