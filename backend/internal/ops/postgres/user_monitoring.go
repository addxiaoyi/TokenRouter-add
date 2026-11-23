package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// user_monitoring.go aggregates authorized group traffic for user service monitoring.
// Successful usage records and final client errors are separate observations;
// recovered upstream attempts do not become client failures. Aligned time buckets keep
// missing traffic distinct from measured success, and nullable timing preserves
// historical records without first-token or duration observations.
//
// Every query restricts the supplied authorized group IDs and time boundaries.
// Only counts, group identity, model names, and aggregate timings leave storage.
// Model rows are bounded per group, and no provider, credential, request payload,
// or internal diagnostic fields enter the user-facing projection. Cache rates
// reuse the administrator token denominator while retaining the authorized group
// set, and missing input tokens remain distinct from an observed zero hit rate.
// TPS averages only complete measured streams through the same expression used
// by usage reports; missing timing never contributes a synthetic zero speed.

// UserMonitoringStore reads group summaries using the application connection pool.
type UserMonitoringStore struct{ db *sql.DB } // externally managed query pool

// NewUserMonitoringStore binds aggregate reads without starting background work.
func NewUserMonitoringStore(db *sql.DB) *UserMonitoringStore { return &UserMonitoringStore{db: db} }

// ReadUserMonitoring fills authorized group observations through bounded aggregate queries.
// The caller supplies a validated window of at most seven days. Empty group
// input skips storage entirely, zero observations preserve nil success rates,
// and each query returns errors instead of synthesizing a healthy response.
// The supplied group slice is updated in place and contains current identity
// metadata. Success counts and final errors combine only for request success;
// cache rates use successful usage token totals, since failed requests supply
// no recorded input-token denominator in this projection.
func (s *UserMonitoringStore) ReadUserMonitoring(ctx context.Context, groups []ops.MonitoringGroup, start, end time.Time) (_ []ops.MonitoringGroup, returnErr error) {
	if len(groups) == 0 {
		return []ops.MonitoringGroup{}, nil
	}
	ids := make([]int64, 0, len(groups))
	byID := map[int64]int{}
	for i := range groups {
		ids = append(ids, groups[i].ID)
		byID[groups[i].ID] = i
		groups[i].Trend = []ops.MonitoringPoint{}
		groups[i].Models = []ops.MonitoringModel{}
	}
	rows, err := s.db.QueryContext(ctx, `WITH observations AS (
        SELECT group_id,count(*) AS success,0::bigint AS errors,
            avg(duration_ms) AS duration,avg(`+usageTTFTSQL("")+`) AS ttft,
            COALESCE(sum(input_tokens),0) AS input_tokens,
            COALESCE(sum(cache_creation_tokens),0) AS cache_creation_tokens,
            COALESCE(sum(cache_read_tokens),0) AS cache_read_tokens,
            avg(`+usageTPSSQL("")+`) AS tps,count(DISTINCT model) AS active_models
        FROM usage_logs WHERE request_source='user' AND group_id=ANY($1)
            AND created_at>=$2 AND created_at<$3 GROUP BY group_id
        UNION ALL
        SELECT group_id,0,count(*),NULL,NULL,0,0,0,NULL,0 FROM ops_error_logs
        WHERE group_id=ANY($1) AND created_at>=$2 AND created_at<$3
            AND is_count_tokens=FALSE AND COALESCE(status_code,0)>=400 GROUP BY group_id
    ) SELECT group_id,sum(success),sum(errors),max(duration),max(ttft),sum(input_tokens),
        sum(cache_creation_tokens),sum(cache_read_tokens),max(tps),max(active_models)
    FROM observations GROUP BY group_id`, pq.Array(ids), start, end)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, success, failures, input, creation, read, models int64
		var duration, ttft, tps *float64
		if err = rows.Scan(&id, &success, &failures, &duration, &ttft, &input, &creation, &read, &tps, &models); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		g := &groups[byID[id]]
		g.InputTokens = input
		g.CacheCreationTokens = creation
		g.CacheReadTokens = read
		g.CacheHitRate = ops.CacheHitRate(read, input, creation)
		g.Success = success
		g.Errors = failures
		g.AvgDurationMS = duration
		g.AvgTTFTMS = ttft
		g.AvgTPS = tps
		g.ActiveModels = models
		if success+failures > 0 {
			rate := float64(success) * 100 / float64(success+failures)
			g.SuccessRate = &rate
		}
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	// Align buckets to a stable UTC origin so refreshes do not shift historical
	// observations; the exact range still bounds both partial endpoint buckets.
	minutes := ops.UserMonitoringBucketMinutes(start, end)
	rows, err = s.db.QueryContext(ctx, `SELECT group_id,bucket,sum(success),sum(errors) FROM (
        SELECT group_id,date_bin($4 * interval '1 minute',created_at,TIMESTAMPTZ '2000-01-01 00:00:00+00') AS bucket,
            count(*) AS success,0::bigint AS errors FROM usage_logs
        WHERE request_source='user' AND group_id=ANY($1) AND created_at>=$2 AND created_at<$3 GROUP BY group_id,bucket
        UNION ALL
        SELECT group_id,date_bin($4 * interval '1 minute',created_at,TIMESTAMPTZ '2000-01-01 00:00:00+00'),0,count(*) FROM ops_error_logs
        WHERE group_id=ANY($1) AND created_at>=$2 AND created_at<$3 AND is_count_tokens=FALSE AND COALESCE(status_code,0)>=400 GROUP BY group_id,2
    ) samples GROUP BY group_id,bucket ORDER BY bucket`, pq.Array(ids), start, end, minutes)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var point ops.MonitoringPoint
		if err = rows.Scan(&id, &point.Time, &point.Success, &point.Errors); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		groups[byID[id]].Trend = append(groups[byID[id]].Trend, point)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, `WITH models AS (SELECT group_id,model,count(*) AS requests,avg(duration_ms) AS duration,avg(`+usageTTFTSQL("")+`) AS ttft,avg(`+usageTPSSQL("")+`) AS tps FROM usage_logs WHERE request_source='user' AND group_id=ANY($1) AND created_at>=$2 AND created_at<$3 GROUP BY group_id,model), ranked AS (SELECT *,row_number() OVER(PARTITION BY group_id ORDER BY requests DESC,model) AS rank FROM models) SELECT group_id,model,requests,duration,ttft,tps FROM ranked WHERE rank<=10 ORDER BY group_id,rank`, pq.Array(ids), start, end)
	if err != nil {
		return nil, err
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	for rows.Next() {
		var id int64
		var model ops.MonitoringModel
		if err = rows.Scan(&id, &model.Model, &model.Requests, &model.AvgDurationMS, &model.AvgTTFTMS, &model.AvgTPS); err != nil {
			return nil, err
		}
		groups[byID[id]].Models = append(groups[byID[id]].Models, model)
	}
	return groups, rows.Err()
}
