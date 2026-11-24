//go:build integration

package postgres

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	infra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/modelquality"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// store_integration_test.go verifies durable assessment admission and recovery
// against real PostgreSQL. Isolated fixtures exercise retry identity, global
// admission, lease ownership, public projection, and comparable baselines without
// executing a real provider or creating any billable upstream request.

// qualityDatabase owns migrated isolated storage for assessment integration tests.
// An external DSN must name support_test. Ordinary runs use the repository's
// container suite, and both modes close owned pools before their fixture exits.
func qualityDatabase(t *testing.T) *sql.DB {
	t.Helper()
	if dsn := os.Getenv("SUPPORT_TEST_DSN"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		var name string
		require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
		require.Equal(t, "support_test", name)
		require.NoError(t, infra.ApplyMigrations(t.Context(), db, migrations.FS))
		return db
	}
	suite := &postgrescontainer.Suite{}
	t.Cleanup(func() { require.NoError(t, suite.Close()) })
	return suite.New(t)
}

// TestAssessmentPersistence verifies global admission, retry lookup, and lease-bound updates.
// Completed results become safe public history and comparable baselines, while
// abandoned work becomes incomplete and cannot be overwritten by its old worker.
func TestAssessmentPersistence(t *testing.T) {
	db := qualityDatabase(t)
	store := NewStore(db)
	ctx := t.Context()
	var owner, provider int64
	require.NoError(t, db.QueryRow(`INSERT INTO users(email,password_hash,role) VALUES($1,'fixture','admin') RETURNING id`, uuid.NewString()+"@example.test").Scan(&owner))
	require.NoError(t, db.QueryRow(`INSERT INTO providers(name,platform,type) VALUES('fixture','openai','apikey') RETURNING id`).Scan(&provider))
	job := modelquality.Job{OperatorID: owner, ProviderID: provider, Fingerprint: "private-fingerprint", Report: modelquality.Report{ID: uuid.NewString(), Label: "Public label", Model: "test-model", SuiteVersion: modelquality.SuiteVersion, CheckedAt: time.Now().UTC(), Status: "queued", Total: 8, Probes: []modelquality.ProbeResult{}}}
	client := uuid.NewString()
	first, err := store.Enqueue(ctx, job, client)
	require.NoError(t, err)
	second, err := store.Enqueue(ctx, job, client)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	_, err = store.Enqueue(ctx, job, uuid.NewString())
	require.Error(t, err)
	claimed, err := store.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	for range 8 {
		claimed.Report.Probes = append(claimed.Report.Probes, modelquality.ProbeResult{Status: "pass"})
	}
	claimed.Report = modelquality.Summarize(claimed.Report, nil)
	require.NoError(t, store.Save(ctx, *claimed))
	history, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, 100, *history[0].Score)
	job.Report.ID = uuid.NewString()
	job.Report.CheckedAt = time.Now().UTC()
	next, err := store.Enqueue(ctx, job, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, first.SeriesID, next.SeriesID)
	claimed, err = store.Claim(ctx)
	require.NoError(t, err)
	baseline, err := store.Baseline(ctx, *claimed)
	require.NoError(t, err)
	require.NotNil(t, baseline)
	require.Equal(t, first.ID, baseline.ID)
	_, err = db.Exec(`UPDATE model_quality_jobs SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, claimed.Report.ID)
	require.NoError(t, err)
	none, err := store.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, none)
	require.Error(t, store.Save(ctx, *claimed))
	recovered, err := store.Get(ctx, claimed.Report.ID)
	require.NoError(t, err)
	require.Equal(t, "incomplete", recovered.Status)
	require.Nil(t, recovered.Score)
}
