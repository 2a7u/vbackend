-- 000006_net_profit_backfill.up.sql: Safe backfill touching ONLY genuinely sold rows without recorded profit
UPDATE vouchers_vault 
SET net_profit = (selling_price - cost_price)::BIGINT,
    net_profit_recorded_at = COALESCE(sold_at, now())
WHERE status = 'sold'
  AND net_profit_recorded_at IS NULL;
