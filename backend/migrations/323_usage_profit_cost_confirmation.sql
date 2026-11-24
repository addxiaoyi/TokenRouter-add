-- Preserve whether an upstream cost multiplier was confirmed when usage was
-- recorded. Historical rows remain NULL and reports treat the legacy default
-- multiplier of one as unconfirmed instead of inventing a historical margin.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS provider_cost_confirmed boolean;
