-- Retain only counters from ephemeral request profiles. Legacy rows preserve
-- the source identity independently of present account ownership.
CREATE TABLE ops_request_activity (
    id BIGSERIAL PRIMARY KEY,
    observed_at TIMESTAMPTZ NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    active_profiles INTEGER NOT NULL CHECK (active_profiles >= 5),
    global_profiles INTEGER NOT NULL CHECK (global_profiles >= active_profiles),
    user_limit INTEGER NOT NULL CHECK (user_limit > 0),
    global_limit INTEGER NOT NULL CHECK (global_limit > 0),
    legacy_id BIGINT UNIQUE CHECK (legacy_id > 0),
    legacy_user_id BIGINT CHECK (legacy_user_id > 0),
    CHECK ((legacy_id IS NULL) = (legacy_user_id IS NULL))
);

CREATE UNIQUE INDEX ops_request_activity_native_bucket_user
    ON ops_request_activity (observed_at, user_id)
    WHERE legacy_id IS NULL;
CREATE INDEX ops_request_activity_recent
    ON ops_request_activity (observed_at DESC, id DESC);
CREATE INDEX ops_request_activity_by_user
    ON ops_request_activity (user_id, observed_at DESC, id DESC);
