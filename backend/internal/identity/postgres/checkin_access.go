package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"entgo.io/ent/dialect"
	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// checkin_access.go reads cumulative account experience and derives temporary
// public-group restrictions from the same completed dates and invitation counts.
// Every stored completed check-in date contributes one point, regardless of
// interruptions or row expiry; only active dates determine the current streak
// and today's status. The affiliate profile's accumulated count supplies
// invitation points, and account status uses the site calendar date.
//
// The installed experience plugin enables this site-specific rule; installations
// without that plugin retain ordinary public-group permissions. The shared domain
// threshold unlocks free groups when cumulative experience reaches level one.
// Restrictions remain a transient account field, so earning access removes only
// the derived denial while administrator-managed public-group denials retain their
// independent effect. PostgreSQL plugin storage owns these observations, and
// unsupported experience readers report service unavailability.

// Experience queries share completed-date deduplication and the saved invitation
// count so account progress and free-group authorization evaluate identical points.
const (
	experienceTotalsSQL = `
WITH requested_users AS (
	SELECT unnest($1::bigint[]) AS user_id
), completed_days AS (
	SELECT DISTINCT (value->>'user_id')::bigint AS user_id,
		(value->>'day')::date AS day
	FROM plugin_kv
	WHERE plugin_id = 'tokenflux.checkin' AND collection = 'checkins'
		AND value->>'completed' = 'true'
		AND (value->>'user_id')::bigint = ANY($1::bigint[])
), active_days AS (
	SELECT DISTINCT (value->>'user_id')::bigint AS user_id,
		(value->>'day')::date AS day
	FROM plugin_kv
	WHERE plugin_id = 'tokenflux.checkin' AND collection = 'checkins'
		AND value->>'completed' = 'true'
		AND (expires_at IS NULL OR expires_at > NOW())
		AND (value->>'user_id')::bigint = ANY($1::bigint[])
), experience_totals AS (
	SELECT requested_users.user_id, COUNT(completed_days.day)::bigint AS checkin_exp,
		COALESCE(user_affiliates.aff_count, 0)::bigint AS invite_exp
	FROM requested_users
	LEFT JOIN completed_days ON completed_days.user_id = requested_users.user_id
	LEFT JOIN user_affiliates ON user_affiliates.user_id = requested_users.user_id
	GROUP BY requested_users.user_id, user_affiliates.aff_count
)
`

	checkinFreeGroupRestrictionsSQL = experienceTotalsSQL + `
SELECT experience_totals.user_id, groups.id
FROM experience_totals
CROSS JOIN groups
WHERE groups.rate_multiplier = 0 AND groups.is_exclusive = false
	AND groups.deleted_at IS NULL
	AND experience_totals.checkin_exp + experience_totals.invite_exp < $2
	AND EXISTS (
		SELECT 1 FROM plugin_installations WHERE plugin_id = 'helstare.exp'
	)
ORDER BY experience_totals.user_id, groups.id
`

	experienceStatusSQL = experienceTotalsSQL + `
, consecutive_days AS (
	SELECT user_id, day, day - ROW_NUMBER() OVER (
		PARTITION BY user_id ORDER BY day
	)::integer AS block
	FROM active_days
	WHERE day <= $2::date
), current_streak AS (
	SELECT user_id, COUNT(*)::bigint AS streak
	FROM consecutive_days
	GROUP BY user_id, block
	HAVING MAX(day) >= $2::date - 1
)
SELECT checkin_exp, invite_exp,
	COALESCE(current_streak.streak, 0),
	EXISTS(SELECT 1 FROM active_days WHERE active_days.user_id = experience_totals.user_id AND day = $2::date)
FROM experience_totals
LEFT JOIN current_streak ON current_streak.user_id = experience_totals.user_id
`
)

// GetExperienceTotals reads completed check-in dates, accumulated invitations,
// and the current calendar streak for an account. Experience counts every
// completed unique date still stored, including expired rows, while streak and
// today status use active records. Today uses the site timezone supplied by
// UserService, and streak ends today or yesterday; older interrupted dates still
// contribute experience.
//
// PostgreSQL is required because plugin records and affiliate profiles share its
// storage schema. Missing SQL access and other database dialects return service
// unavailability, while query, scan, iteration, and row-close failures remain
// visible to the calling account service.
func (r *UserStore) GetExperienceTotals(ctx context.Context, userID int64, today string) (_ *identity.ExperienceTotals, returnErr error) {
	if r.sql == nil || r.client != nil && r.client.Driver().Dialect() != dialect.Postgres {
		return nil, identity.ErrExperienceUnavailable
	}
	rows, err := r.sql.QueryContext(ctx, experienceStatusSQL, pq.Array([]int64{userID}), today)
	if err != nil {
		return nil, fmt.Errorf("load account experience: %w", err)
	}
	// Return the query connection after reading statistics, preserving close
	// failures alongside an earlier read failure for the account service.
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	if !rows.Next() {
		return nil, fmt.Errorf("read account experience: %w", errors.Join(sql.ErrNoRows, rows.Err()))
	}
	var totals identity.ExperienceTotals
	if err := rows.Scan(&totals.CheckinExp, &totals.InviteExp, &totals.Streak, &totals.CheckedInToday); err != nil {
		return nil, fmt.Errorf("read account experience: %w", err)
	}
	return &totals, rows.Err()
}

// loadCheckinGroupRestrictions returns derived free-group denials by account ID.
// Empty input, Ent-only stores, and configured non-PostgreSQL clients return an
// empty map because plugin policy storage requires PostgreSQL. One query serves
// the complete requested account batch, and storage failures propagate before
// callers mark permissions as loaded.
func (r *UserStore) loadCheckinGroupRestrictions(ctx context.Context, userIDs []int64) (_ map[int64][]int64, returnErr error) {
	groups := make(map[int64][]int64, len(userIDs))
	if len(userIDs) == 0 || r.sql == nil {
		return groups, nil
	}
	// SQLite account stores exercise Ent-managed schemas; plugin installations
	// and check-in records belong to the PostgreSQL plugin storage adapter.
	if r.client != nil && r.client.Driver().Dialect() != dialect.Postgres {
		return groups, nil
	}
	rows, err := r.sql.QueryContext(ctx, checkinFreeGroupRestrictionsSQL, pq.Array(userIDs), identity.ExperienceLevelStep)
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
