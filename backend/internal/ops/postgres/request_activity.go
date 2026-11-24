package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// request_activity.go persists threshold summaries and reads their administrative
// history. The underlying table has no place for execution payloads or transient
// profile fingerprints. Account joins revalidate the latest role at insertion,
// and retention applies to native observations separately from imported history.

// A cleanup batch caps the number of expired native snapshots removed per pass.
const (
	requestActivityPruneLimit = 5000
)

// RequestActivityStore reads and writes aggregate PostgreSQL activity.
type RequestActivityStore struct {
	db *sql.DB // application-owned database pool
}

// NewRequestActivityStore attaches aggregate persistence to the supplied pool.
func NewRequestActivityStore(db *sql.DB) *RequestActivityStore {
	return &RequestActivityStore{db: db}
}

// Save writes the threshold-qualified users from one sample atomically.
// Existing distinct rows for a native account and sample bucket stay unchanged.
// Deleted, inactive, and administrator accounts are omitted even when their
// earlier request had a valid user identity.
func (s *RequestActivityStore) Save(ctx context.Context, sample ops.RequestActivitySnapshot) error {
	ids := make([]int64, 0, len(sample.Accounts))
	values := make([]int64, 0, len(sample.Accounts))
	for _, account := range sample.Accounts {
		if account.Profiles >= ops.RequestActivityThreshold {
			ids = append(ids, account.UserID)
			values = append(values, int64(account.Profiles))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ops_request_activity (observed_at, user_id, active_profiles, global_profiles, user_limit, global_limit)
SELECT $1, u.id, observed.profiles, $4, $5, $6
FROM unnest($2::bigint[], $3::bigint[]) AS observed(user_id, profiles)
JOIN users AS u ON u.id = observed.user_id
WHERE u.status = 'active' AND u.role = 'user' AND u.deleted_at IS NULL
ON CONFLICT DO NOTHING`, sample.ObservedAt, pq.Array(ids), pq.Array(values),
		sample.Profiles, sample.UserLimit, sample.GlobalLimit)
	if err != nil {
		return fmt.Errorf("persist request activity snapshot: %w", err)
	}
	return nil
}

// List reads a timestamp-ordered page of history, including old snapshots.
// A positive cursor selects earlier timestamp-and-ID pairs rather than an
// offset, so newer imports do not move rows between already traversed pages.
// The caller validates the positive limit and an optional linked account ID.
func (s *RequestActivityStore) List(ctx context.Context, userID, beforeID int64, limit int) (_ ops.RequestActivityPage, err error) {
	page := ops.RequestActivityPage{Items: make([]ops.RequestActivityEntry, 0, limit)}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, observed_at, user_id, legacy_user_id, active_profiles, global_profiles, user_limit, global_limit
FROM ops_request_activity AS a
WHERE ($1 = 0 OR user_id = $1) AND
    ($2 = 0 OR (a.observed_at, a.id) < (SELECT observed_at, id FROM ops_request_activity WHERE id = $2))
ORDER BY observed_at DESC, id DESC LIMIT $3`, userID, beforeID, limit+1)
	if err != nil {
		return page, fmt.Errorf("list request activity: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		// The lookahead determines whether another page exists. Return the
		// last delivered observation as the next page's exclusive boundary.
		if len(page.Items) == limit {
			page.NextID = page.Items[limit-1].ID
			break
		}
		var item ops.RequestActivityEntry
		if err := rows.Scan(&item.ID, &item.ObservedAt, &item.UserID, &item.LegacyUserID,
			&item.Profiles, &item.GlobalProfiles, &item.UserLimit, &item.GlobalLimit); err != nil {
			return page, fmt.Errorf("scan request activity: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

// Prune deletes a bounded group of expired native rows and retains imported snapshots.
func (s *RequestActivityStore) Prune(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM ops_request_activity
WHERE id IN (
    SELECT id FROM ops_request_activity WHERE legacy_id IS NULL AND observed_at < $1
    ORDER BY observed_at, id LIMIT $2
)`, cutoff, requestActivityPruneLimit)
	if err != nil {
		return fmt.Errorf("prune request activity: %w", err)
	}
	return nil
}
