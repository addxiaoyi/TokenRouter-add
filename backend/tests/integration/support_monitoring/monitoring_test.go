//go:build integration

package support_monitoring

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	infra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	opspostgres "github.com/TokenFlux/TokenRouter/internal/ops/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// monitoring_test.go verifies the user dashboard against migrated PostgreSQL.
// Success logs, recovered attempts, final failures, and hidden groups coexist in
// the fixture so the public projection must preserve each boundary. The optional
// external database is restricted to the isolated support_test release probe.

// TestUserMonitoringPostgres verifies aggregation, timing, and group isolation together.
// The hidden group contributes real rows but cannot affect the authorized query;
// recovered errors and end-boundary samples are also excluded from its totals.
func TestUserMonitoringPostgres(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	if dsn := os.Getenv("SUPPORT_TEST_DSN"); dsn != "" {
		var err error
		db, err = sql.Open("postgres", dsn)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		var name string
		require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))
		require.Equal(t, "support_test", name)
		require.NoError(t, infra.ApplyMigrations(ctx, db, migrations.FS))
	} else {
		suite := &postgrescontainer.Suite{}
		t.Cleanup(func() { require.NoError(t, suite.Close()) })
		db = suite.New(t)
	}
	var user, key, provider, visible, hidden, idle int64
	uid := uuid.NewString()
	require.NoError(t, db.QueryRow(`INSERT INTO users(email,password_hash) VALUES($1,'test') RETURNING id`, uid+"@example.com").Scan(&user))
	require.NoError(t, db.QueryRow(`INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'monitor test') RETURNING id`, user, uid).Scan(&key))
	require.NoError(t, db.QueryRow(`INSERT INTO providers(name,platform,type) VALUES($1,'openai','apikey') RETURNING id`, uid).Scan(&provider))
	for _, id := range []*int64{&visible, &hidden, &idle} {
		require.NoError(t, db.QueryRow(`INSERT INTO groups(name) VALUES($1) RETURNING id`, uuid.NewString()).Scan(id))
	}
	start := time.Date(2043, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, group := range []int64{visible, hidden} {
		_, err := db.Exec(`INSERT INTO usage_logs(user_id,billing_user_id,api_key_id,provider_id,group_id,request_id,platform,model,duration_ms,timings,created_at) VALUES($1,$1,$2,$3,$4,$5,'openai','test-model',200,'{"upstream_stream":true,"ttft_mode":"first_output","values":{"upstream_started_ms":10,"upstream_first_output_ms":60}}',$6)`, user, key, provider, group, uuid.NewString(), start.Add(time.Minute))
		require.NoError(t, err)
	}
	for _, status := range []int{200, 503} {
		_, err := db.Exec(`INSERT INTO ops_error_logs(request_id,user_id,api_key_id,provider_id,group_id,platform,model,error_phase,error_type,status_code,created_at,is_count_tokens) VALUES($1,$2,$3,$4,$5,'openai','test-model','request','api_error',$6,$7,FALSE)`, uuid.NewString(), user, key, provider, visible, status, start.Add(2*time.Minute))
		require.NoError(t, err)
	}
	rows, err := opspostgres.NewUserMonitoringStore(db).ReadUserMonitoring(ctx, []ops.MonitoringGroup{{ID: visible}, {ID: idle}}, start, end)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(1), rows[0].Success)
	require.Equal(t, int64(1), rows[0].Errors)
	require.Equal(t, 50.0, *rows[0].SuccessRate)
	require.Equal(t, 200.0, *rows[0].AvgDurationMS)
	require.Equal(t, 50.0, *rows[0].AvgTTFTMS)
	require.Len(t, rows[0].Trend, 1)
	require.Len(t, rows[0].Models, 1)
	require.Nil(t, rows[1].SuccessRate)
}
