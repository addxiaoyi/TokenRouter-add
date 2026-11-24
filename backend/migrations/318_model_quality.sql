CREATE TABLE model_quality_jobs (
    id UUID PRIMARY KEY,
    provider_id BIGINT REFERENCES providers(id),
    operator_id BIGINT REFERENCES users(id),
    model TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    suite_version TEXT NOT NULL,
    series_id UUID NOT NULL,
    client_id UUID UNIQUE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued','running','complete','incomplete')),
    lease_id UUID,
    lease_until TIMESTAMPTZ,
    report JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    legacy_id TEXT UNIQUE
);
CREATE UNIQUE INDEX model_quality_one_active_idx ON model_quality_jobs ((true)) WHERE status IN ('queued','running');
CREATE INDEX model_quality_recent_idx ON model_quality_jobs(created_at DESC);
CREATE INDEX model_quality_baseline_idx ON model_quality_jobs(provider_id,model,fingerprint,suite_version,created_at DESC) WHERE status='complete';
