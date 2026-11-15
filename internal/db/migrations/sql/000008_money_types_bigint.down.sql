-- 000008_money_types_bigint.down.sql
ALTER TABLE vouchers_vault ALTER COLUMN cost_price TYPE DOUBLE PRECISION USING cost_price::DOUBLE PRECISION;
ALTER TABLE vouchers_vault ALTER COLUMN selling_price TYPE DOUBLE PRECISION USING selling_price::DOUBLE PRECISION;
