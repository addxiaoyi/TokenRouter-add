package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// visitor_fingerprints.go stores scoped browser and address digests. Visits
// lock the associated account row while upserting and trimming observations so
// simultaneous logins cannot each keep an independent set of five pairs.
//
// Reads hide expired observations regardless of worker progress. Duplicate
// matching compares both aliases and the source namespace, excludes deleted
// users, and never counts repeated observations from the same account. Cleanup
// deletes small locked batches so it can share the table with ongoing visits.

const (
	// Each expiry pass deletes a fixed batch to limit transaction duration.
	visitorFingerprintExpiryBatch = 1000
)

// VisitorFingerprintStore owns queries for persisted browser and address aliases.
type VisitorFingerprintStore struct {
	db *sql.DB // shared application pool
}

// NewVisitorFingerprintStore binds a store to the application database pool.
func NewVisitorFingerprintStore(db *sql.DB) *VisitorFingerprintStore {
	return &VisitorFingerprintStore{db: db}
}

// ObserveVisitorFingerprint upserts a pair and trims the account history in one
// transaction. The account lock covers writes for all pairs and source types.
// A delayed request contributes its first-seen timestamp without overwriting a
// more recent last-seen value. A write failure rolls back both the observation
// and pruning, leaving the previous valid set intact.
func (s *VisitorFingerprintStore) ObserveVisitorFingerprint(ctx context.Context, visit identity.VisitorFingerprint) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin visitor observation: %w", err)
	}
	// Rollback releases the account lock on every error path; after a commit the
	// transaction is already closed and there is no cleanup failure to report.
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, rollbackErr)
		}
	}()

	var owner int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, visit.UserID).Scan(&owner)
	if err != nil {
		return fmt.Errorf("lock visitor account: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO visitor_fingerprints (user_id, source, visitor_hash, ip_hash, first_seen_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, source, visitor_hash, ip_hash) DO UPDATE
		SET first_seen_at = LEAST(visitor_fingerprints.first_seen_at, EXCLUDED.first_seen_at),
		    last_seen_at = GREATEST(visitor_fingerprints.last_seen_at, EXCLUDED.last_seen_at)`,
		owner, visit.Source, visit.VisitorHash, visit.IPHash, visit.FirstSeenAt, visit.LastSeenAt)
	if err != nil {
		return fmt.Errorf("record visitor observation: %w", err)
	}
	// Preserve the newest observations by timestamp across browser revisions and
	// imports. Same-time observations retain the highest allocated IDs first.
	_, err = tx.ExecContext(ctx, `DELETE FROM visitor_fingerprints WHERE id IN (
		SELECT id FROM visitor_fingerprints WHERE user_id=$1 ORDER BY last_seen_at DESC,id DESC OFFSET $2
	)`, owner, identity.VisitorFingerprintCap)
	if err != nil {
		return fmt.Errorf("trim visitor observations: %w", err)
	}
	return tx.Commit()
}

// ListVisitorFingerprints returns one cursor page of recent observations. A
// duplicate candidate must have another current account with the same source,
// visitor hash and IP hash, even when a filter narrows the displayed account.
func (s *VisitorFingerprintStore) ListVisitorFingerprints(ctx context.Context, filter identity.VisitorFingerprintFilter, cutoff time.Time) (_ []identity.VisitorFingerprint, err error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id,f.user_id,f.source,f.visitor_hash,f.ip_hash,f.first_seen_at,f.last_seen_at
		FROM visitor_fingerprints f JOIN users u ON u.id=f.user_id AND u.deleted_at IS NULL
		WHERE f.last_seen_at >= $1
		  AND ($2::bigint = 0 OR f.user_id = $2)
		  AND ($3 = '' OR f.visitor_hash = $3)
		  AND ($4 = '' OR f.ip_hash = $4)
		  AND ($5::bigint = 0 OR f.id < $5)
		  AND (NOT $6::boolean OR EXISTS (
			SELECT 1 FROM visitor_fingerprints peer
			JOIN users peer_user ON peer_user.id=peer.user_id AND peer_user.deleted_at IS NULL
			WHERE peer.source=f.source AND peer.visitor_hash=f.visitor_hash AND peer.ip_hash=f.ip_hash
			  AND peer.user_id<>f.user_id AND peer.last_seen_at >= $1
		  ))
		ORDER BY f.id DESC LIMIT $7`, cutoff, filter.UserID, filter.VisitorHash, filter.IPHash, filter.Before, filter.Duplicates, identity.VisitorFingerprintPage+1)
	if err != nil {
		return nil, fmt.Errorf("query visitor observations: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	visits := make([]identity.VisitorFingerprint, 0)
	for rows.Next() {
		var visit identity.VisitorFingerprint
		if err := rows.Scan(&visit.ID, &visit.UserID, &visit.Source, &visit.VisitorHash, &visit.IPHash, &visit.FirstSeenAt, &visit.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan visitor observation: %w", err)
		}
		visits = append(visits, visit)
	}
	return visits, rows.Err()
}

// DeleteExpiredVisitorFingerprints deletes up to one batch past the cutoff.
// Rows locked by another writer remain for a later sweep, while all reads
// already exclude timestamps outside the observation window.
func (s *VisitorFingerprintStore) DeleteExpiredVisitorFingerprints(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM visitor_fingerprints WHERE id IN (
		SELECT id FROM visitor_fingerprints WHERE last_seen_at < $1
		ORDER BY last_seen_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	)`, cutoff, visitorFingerprintExpiryBatch)
	if err != nil {
		return fmt.Errorf("delete expired visitor observations: %w", err)
	}
	return nil
}
