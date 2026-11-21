-- Preserve six-place sales rates in both group configuration and usage audits.
-- Widening the decimal columns preserves existing values and defaults.
ALTER TABLE groups
    ALTER COLUMN rate_multiplier TYPE NUMERIC(20,10);
ALTER TABLE usage_logs
    ALTER COLUMN rate_multiplier TYPE NUMERIC(20,10);
