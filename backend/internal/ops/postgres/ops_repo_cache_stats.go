package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// ops_repo_cache_stats.go queries overall and per-group cache token totals
// from saved usage rows.
// Both queries share the dashboard usage predicate, so the overall block and
// the group rows describe the same window and filters. Group rows only cover
// requests recorded with a group; requests without one still count toward the
// overall totals. Hit rates are computed from the summed counters after the
// query, which keeps a window without input-side tokens free of a fabricated
// zero rate.

// cacheStatsGroupLimit bounds the per-group breakdown returned for one window.
// Groups are ordered by descending total input tokens, so the limit keeps the
// largest groups when a window covers more groups than the response carries.
const cacheStatsGroupLimit = 100

// GetCacheStats returns cache token totals for customer usage rows in the
// supplied half-open window, plus the leading recorded groups by total input
// tokens. Nonzero ordered endpoints are required, equality is accepted, and
// platform is trimmed and lowercased before it reaches the query. The response
// retains the selected group identity, while only a positive value adds a SQL
// predicate. Query and scan failures reject the response; the supplied filter
// is read without mutation.
func (r *Store) GetCacheStats(ctx context.Context, filter *ops.OpsCacheStatsFilter) (*ops.OpsCacheStatsResponse, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if filter == nil {
		return nil, fmt.Errorf("nil filter")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, fmt.Errorf("start_time/end_time required")
	}
	// An empty time range is valid, matching service validation.
	if filter.StartTime.After(filter.EndTime) {
		return nil, fmt.Errorf("start_time must be <= end_time")
	}

	dashboardFilter := &ops.OpsDashboardFilter{
		StartTime: filter.StartTime.UTC(),
		EndTime:   filter.EndTime.UTC(),
		Platform:  strings.TrimSpace(strings.ToLower(filter.Platform)),
		GroupID:   filter.GroupID,
	}

	join, where, args, next := buildUsageWhere(dashboardFilter, dashboardFilter.StartTime, dashboardFilter.EndTime, 1)

	resp := &ops.OpsCacheStatsResponse{
		TimeRange: strings.TrimSpace(filter.TimeRange),
		StartTime: dashboardFilter.StartTime,
		EndTime:   dashboardFilter.EndTime,
		Platform:  dashboardFilter.Platform,
		GroupID:   dashboardFilter.GroupID,
		Groups:    make([]*ops.OpsCacheStatsGroupItem, 0, 16),
	}

	overallSQL := `
SELECT
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(ul.input_tokens), 0)::bigint AS input_tokens,
  COALESCE(SUM(ul.cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
  COALESCE(SUM(ul.cache_read_tokens), 0)::bigint AS cache_read_tokens,
  (COALESCE(SUM(ul.input_tokens), 0) + COALESCE(SUM(ul.cache_creation_tokens), 0) + COALESCE(SUM(ul.cache_read_tokens), 0))::bigint AS total_input_tokens
FROM (SELECT * FROM usage_logs WHERE request_source = 'user') ul
` + join + `
` + where

	var overall ops.OpsCacheStatsOverall
	if err := r.db.QueryRowContext(ctx, overallSQL, args...).Scan(
		&overall.RequestCount,
		&overall.InputTokens,
		&overall.CacheCreationTokens,
		&overall.CacheReadTokens,
		&overall.TotalInputTokens,
	); err != nil {
		return nil, err
	}
	overall.HitRate = ops.CacheHitRate(overall.CacheReadTokens, overall.InputTokens, overall.CacheCreationTokens)
	resp.Overall = overall

	groupSQL := `
SELECT
  ul.group_id AS group_id,
  COALESCE(g.name, '') AS group_name,
  COUNT(*)::bigint AS request_count,
  COALESCE(SUM(ul.input_tokens), 0)::bigint AS input_tokens,
  COALESCE(SUM(ul.cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
  COALESCE(SUM(ul.cache_read_tokens), 0)::bigint AS cache_read_tokens,
  (COALESCE(SUM(ul.input_tokens), 0) + COALESCE(SUM(ul.cache_creation_tokens), 0) + COALESCE(SUM(ul.cache_read_tokens), 0))::bigint AS total_input_tokens
FROM (SELECT * FROM usage_logs WHERE request_source = 'user') ul
LEFT JOIN groups g ON g.id = ul.group_id
` + join + `
` + where + `
  AND ul.group_id IS NOT NULL
GROUP BY ul.group_id, g.name
ORDER BY total_input_tokens DESC, ul.group_id ASC
LIMIT $` + fmt.Sprintf("%d", next)

	groupArgs := make([]any, 0, len(args)+1)
	groupArgs = append(groupArgs, args...)
	groupArgs = append(groupArgs, cacheStatsGroupLimit)

	rows, err := r.db.QueryContext(ctx, groupSQL, groupArgs...)
	if err != nil {
		return nil, err
	}
	// Close the cursor after scanning or an early failure; cleanup errors are
	// suppressed, while query, scan, and iteration errors determine the response.
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		item := &ops.OpsCacheStatsGroupItem{}
		if err := rows.Scan(
			&item.GroupID,
			&item.GroupName,
			&item.RequestCount,
			&item.InputTokens,
			&item.CacheCreationTokens,
			&item.CacheReadTokens,
			&item.TotalInputTokens,
		); err != nil {
			return nil, err
		}
		item.HitRate = ops.CacheHitRate(item.CacheReadTokens, item.InputTokens, item.CacheCreationTokens)
		resp.Groups = append(resp.Groups, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}
