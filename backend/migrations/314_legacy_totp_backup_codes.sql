-- Imported recovery proofs remain bound to the migrated TOTP enrollment.
-- Re-enrollment changes its ciphertext, so old recovery codes cannot regain access.
CREATE TABLE IF NOT EXISTS legacy_totp_backup_codes (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT UNIQUE NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    enrollment_ciphertext TEXT NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS legacy_totp_backup_codes_user_idx
    ON legacy_totp_backup_codes(user_id) WHERE NOT used AND deleted_at IS NULL;
