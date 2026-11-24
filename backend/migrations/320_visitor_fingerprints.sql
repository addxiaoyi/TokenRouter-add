-- Store only keyed digests for visitor and network comparisons. Keep old and
-- current browser algorithms in separate namespaces so imports cannot match
-- live observations by accident.
CREATE TABLE visitor_fingerprints (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('browser', 'legacy')),
    visitor_hash TEXT NOT NULL CHECK (visitor_hash ~ '^[0-9a-f]{64}$'),
    ip_hash TEXT NOT NULL CHECK (ip_hash ~ '^[0-9a-f]{64}$'),
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL CHECK (last_seen_at >= first_seen_at),
    UNIQUE (user_id, source, visitor_hash, ip_hash)
);

CREATE INDEX visitor_fingerprints_owner_recent_idx
    ON visitor_fingerprints (user_id, last_seen_at DESC, id DESC);
CREATE INDEX visitor_fingerprints_pair_idx
    ON visitor_fingerprints (source, visitor_hash, ip_hash, last_seen_at DESC);
CREATE INDEX visitor_fingerprints_ip_idx
    ON visitor_fingerprints (ip_hash, id DESC);
CREATE INDEX visitor_fingerprints_expiry_idx
    ON visitor_fingerprints (last_seen_at, id);
