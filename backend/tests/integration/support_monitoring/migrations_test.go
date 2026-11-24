//go:build integration

package support_monitoring

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	infra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// migrations_test.go validates additive support migrations against an isolated
// production-backup restoration. The named restore database is supplied by the
// release procedure; this test cannot use the production database name and
// verifies user and usage counts survive the real migration runner unchanged.

// TestReleaseMigrationRestore applies pending migrations to an explicitly named restore probe.
// Existing account and usage rows must survive, and both new domains must be
// queryable afterward. Without a restore DSN this release-only check is skipped.
func TestReleaseMigrationRestore(t *testing.T) {
	dsn := os.Getenv("RESTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("RESTORE_TEST_DSN is required for the isolated release restore probe")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	require.Equal(t, "support_restore", name)
	var users, usage int64
	require.NoError(t, db.QueryRow("SELECT count(*) FROM users").Scan(&users))
	require.NoError(t, db.QueryRow("SELECT count(*) FROM usage_logs").Scan(&usage))
	require.NoError(t, infra.ApplyMigrations(t.Context(), db, migrations.FS))
	var afterUsers, afterUsage, tickets, jobs int64
	require.NoError(t, db.QueryRow("SELECT count(*) FROM users").Scan(&afterUsers))
	require.NoError(t, db.QueryRow("SELECT count(*) FROM usage_logs").Scan(&afterUsage))
	require.Equal(t, users, afterUsers)
	require.Equal(t, usage, afterUsage)
	require.NoError(t, db.QueryRow("SELECT count(*) FROM support_tickets").Scan(&tickets))
	require.NoError(t, db.QueryRow("SELECT count(*) FROM model_quality_jobs").Scan(&jobs))
	t.Logf("restore migration preserved users=%d usage=%d; support and assessment tables queryable", users, usage)
}
