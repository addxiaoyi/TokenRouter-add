package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// checkin_access.go derives temporary public-group restrictions from completed
// check-in records and the installed experience policy. The repository merges
// these restrictions into user views so authentication can apply earned access
// without overwriting administrator-managed group denials.
//
// The installed experience plugin enables this site-specific rule; installations
// without that plugin retain ordinary public-group permissions. A completed
// consecutive block earns the first experience point and unlocks level one.
// Interrupted blocks remain separate, and pending rewards contribute no
// completed day to this eligibility decision.
//
// These restrictions are a separate transient field on account views, so group
// selection and consumers of CanBindGroup share one decision. They are computed
// rather than persisted as administrator settings: earning the required point
// removes only the derived restriction. Administrator forms continue to receive
// their explicitly saved public-group denials and retain those denials after an
// account reaches the required level.

// checkinFreeGroupRestrictionsSQL identifies zero-rate public groups unavailable
// to an account until it completes its first configured check-in block.
const (
	checkinFreeGroupRestrictionsSQL = `
WITH requested_users AS (
	SELECT unnest($1::bigint[]) AS user_id
), policy AS (
	SELECT COALESCE((config->>'exp_days_per_point')::integer, 7) AS days_per_point
	FROM plugin_installations
	WHERE plugin_id = 'helstare.exp'
), completed_days AS (
	SELECT DISTINCT (value->>'user_id')::bigint AS user_id,
		(value->>'day')::date AS day
	FROM plugin_kv
	WHERE plugin_id = 'tokenflux.checkin' AND collection = 'checkins'
		AND value->>'completed' = 'true'
		AND (expires_at IS NULL OR expires_at > NOW())
		AND (value->>'user_id')::bigint = ANY($1::bigint[])
), consecutive_days AS (
	SELECT user_id, day - ROW_NUMBER() OVER (
		PARTITION BY user_id ORDER BY day
	)::integer AS block
	FROM completed_days
), eligible AS (
	SELECT user_id FROM consecutive_days
	GROUP BY user_id, block
	HAVING COUNT(*) >= (SELECT days_per_point FROM policy)
)
SELECT requested_users.user_id, groups.id
FROM requested_users
CROSS JOIN policy
CROSS JOIN groups
WHERE groups.rate_multiplier = 0 AND groups.is_exclusive = false
	AND groups.deleted_at IS NULL
	AND NOT EXISTS (
		SELECT 1 FROM eligible WHERE eligible.user_id = requested_users.user_id
	)
ORDER BY requested_users.user_id, groups.id
`
)

// loadCheckinGroupRestrictions returns derived free-group denials by account ID.
// Empty input and Ent-only stores return an empty map; production stores supply
// the plugin SQL executor. One query serves the complete requested account batch,
// and storage failures propagate before callers mark permissions as loaded.
func (r *UserStore) loadCheckinGroupRestrictions(ctx context.Context, userIDs []int64) (_ map[int64][]int64, returnErr error) {
	groups := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 || r.sql == nil {
		return groups, nil
	}
	rows, err := r.sql.QueryContext(ctx, checkinFreeGroupRestrictionsSQL, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("load check-in group restrictions: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	for rows.Next() {
		var userID, groupID int64
		if err := rows.Scan(&userID, &groupID); err != nil {
			return nil, fmt.Errorf("read check-in group restriction: %w", err)
		}
		groups[userID] = append(groups[userID], groupID)
	}
	return groups, rows.Err()
}
