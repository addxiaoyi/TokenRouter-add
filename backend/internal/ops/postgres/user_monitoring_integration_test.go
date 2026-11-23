//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/timing"
	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// user_monitoring_integration_test.go checks monitoring arithmetic against PostgreSQL.
// Migrated tables enforce the same stored timing and request-source contracts as
// production. Each fixture owns a distinct future interval and removes its rows
// before referenced entities, so this test never needs external provider traffic.

// TestUserMonitoringMeasuredStreamsAndBuckets mixes complete streams, missing timing,
// internal probes, another group, and an exclusive-end request. Only authorized
// user observations contribute to rates, model counts, and five-minute buckets.
func TestUserMonitoringMeasuredStreamsAndBuckets(t *testing.T) {
	ctx := t.Context()
	start := time.Date(2053, 11, 23, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	var userID, keyID, providerID int64
	groupIDs := make([]int64, 2)
	// Release fixture logs before their referenced account and route records.
	// Cleanup includes excluded boundary rows and still runs after any failed assertion.
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, table := range []string{"usage_logs", "ops_error_logs"} {
			_, err := integrationDB.ExecContext(cleanup, "DELETE FROM "+table+" WHERE created_at >= $1 AND created_at <= $2", start, end)
			require.NoError(t, err)
		}
		_, err := integrationDB.ExecContext(cleanup, "DELETE FROM api_keys WHERE id=$1", keyID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanup, "DELETE FROM providers WHERE id=$1", providerID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanup, "DELETE FROM groups WHERE id=ANY($1)", pq.Array(groupIDs))
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanup, "DELETE FROM users WHERE id=$1", userID)
		require.NoError(t, err)
	})
	require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('monitoring-fixture@test.local','fixture') RETURNING id").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,'monitoring-fixture-key','monitoring') RETURNING id", userID).Scan(&keyID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO providers(name,platform,type) VALUES('monitoring-fixture','openai','apikey') RETURNING id").Scan(&providerID))
	for i, name := range []string{"monitoring-visible", "monitoring-private"} {
		require.NoError(t, integrationDB.QueryRowContext(ctx, "INSERT INTO groups(name) VALUES($1) RETURNING id", name).Scan(&groupIDs[i]))
	}
	performance := timing.NewPerformance(timing.FirstOutput, 100)
	performance.Stream = true
	performance.Complete = true
	performance.UsageComplete = true
	performance.Values["upstream_first_output_ms"] = 300
	performance.Values["upstream_last_output_ms"] = 900
	encoded, err := json.Marshal(performance)
	require.NoError(t, err)
	for i, row := range []struct {
		group   int64  // stored routing group identity
		minute  int    // offset from fixture start
		source  string // user or internal probe origin
		model   string // public model request identity
		timings string // JSON; empty object means unavailable
	}{
		{groupIDs[0], 1, "user", "stream", string(encoded)},
		{groupIDs[0], 31, "user", "unknown-timing", "{}"},
		{groupIDs[0], 60, "user", "excluded-end", string(encoded)},
		{groupIDs[0], 10, "group_probe", "internal", string(encoded)},
		{groupIDs[1], 10, "user", "private", string(encoded)},
	} {
		var actor, key any = userID, keyID
		var system any
		if row.source != "user" {
			// Internal probes have no customer funding identity; valid attribution
			// keeps this exclusion fixture faithful to the production table constraint.
			actor = nil
			key = nil
			system = `{"outcome":"succeeded","usage_state":"complete","cost_state":"known"}`
		}
		_, err = integrationDB.ExecContext(ctx, `INSERT INTO usage_logs
            (user_id,billing_user_id,api_key_id,provider_id,group_id,request_id,model,
            input_tokens,cache_creation_tokens,cache_read_tokens,output_tokens,duration_ms,timings,request_source,created_at,system_usage)
            VALUES($1,$1,$2,$3,$4,$5,$6,20,20,60,80,1000,$7::jsonb,$8,$9,$10::jsonb)`,
			actor, key, providerID, row.group, "monitoring-fixture-"+time.Duration(i).String(), row.model, row.timings, row.source, start.Add(time.Duration(row.minute)*time.Minute), system)
		require.NoError(t, err)
	}
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO ops_error_logs(group_id,error_phase,error_type,status_code,created_at) VALUES($1,'request','fixture',503,$2)`, groupIDs[0], start.Add(7*time.Minute))
	require.NoError(t, err)
	groups, err := NewUserMonitoringStore(integrationDB).ReadUserMonitoring(ctx, []ops.MonitoringGroup{{ID: groupIDs[0]}}, start, end)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	observed := groups[0]
	require.EqualValues(t, 2, observed.Success)
	require.EqualValues(t, 1, observed.Errors)
	require.EqualValues(t, 2, observed.ActiveModels)
	require.InDelta(t, 200.0/3.0, *observed.SuccessRate, 0.00001)
	require.Equal(t, 0.6, *observed.CacheHitRate)
	require.Equal(t, 100.0, *observed.AvgTPS)
	require.Len(t, observed.Trend, 3)
	require.Equal(t, start, observed.Trend[0].Time)
	require.Equal(t, start.Add(5*time.Minute), observed.Trend[1].Time)
	require.Equal(t, start.Add(30*time.Minute), observed.Trend[2].Time)
}
