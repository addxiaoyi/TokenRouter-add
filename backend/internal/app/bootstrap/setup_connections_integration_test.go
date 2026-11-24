//go:build integration

package bootstrap

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// setup_connections_integration_test.go verifies empty-password setup against
// PostgreSQL. The release probe supplies an isolated support_test database;
// ordinary runs use the repository container fixture. Administrator persistence
// must target the requested database instead of silently using postgres.

// TestSetupEmptyPasswordTargetsRequestedDatabase exercises the initialization
// connection string with an empty password. Schema and administrator creation
// must affect the selected database, preserving the configured login identity.
func TestSetupEmptyPasswordTargetsRequestedDatabase(t *testing.T) {
	dsn := os.Getenv("SUPPORT_TEST_DSN")
	if dsn == "" {
		suite := &postgrescontainer.Suite{}
		t.Cleanup(func() { require.NoError(t, suite.Close()) })
		db := suite.New(t)
		// The ordinary authenticated fixture verifies creation through its existing
		// pool; the release probe separately exercises the empty-password connection.
		created, _, err := CreateInitialAdmin(t.Context(), db, identity.InitialAdminInput{Email: "setup@example.test", Password: "SetupFixture2026!", Concurrency: 1, Now: time.Now}, nil)
		require.NoError(t, err)
		require.True(t, created)
		return
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var database string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&database))
	require.Equal(t, "support_test", database)
	cfg := &SetupDatabaseConfig{Host: "127.0.0.1", Port: 5432, User: "postgres", DBName: database, SSLMode: "disable"}
	require.NoError(t, InitializeSetupDatabase(context.Background(), cfg, time.Minute))
	created, _, err := InitializeSetupAdmin(t.Context(), cfg, identity.InitialAdminInput{Email: "setup@example.test", Password: "SetupFixture2026!", Concurrency: 1, Now: time.Now}, nil)
	require.NoError(t, err)
	require.True(t, created)
	var email string
	require.NoError(t, db.QueryRow("SELECT email FROM users WHERE role='admin'").Scan(&email))
	require.Equal(t, "setup@example.test", email)
}
