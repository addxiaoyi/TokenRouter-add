package bootstrap

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// setup_connections_test.go verifies that database creation uses a maintenance connection before the target
// exists. This fixture gives setup a distinct target name and checks the two
// DSNs, keeping maintenance access on postgres and the later verification
// connection on the requested database.

// TestBuildDatabaseConnectionDSNsUsesPostgresForBootstrap supplies a target
// database name with shared credentials. The maintenance DSN must name
// postgres, exclude the target name, and leave the target DSN pointing at the
// configured database.
func TestBuildDatabaseConnectionDSNsUsesPostgresForBootstrap(t *testing.T) {
	cfg := &SetupDatabaseConfig{
		Host:     "db",
		Port:     5432,
		User:     "tokenrouter",
		Password: "secret",
		DBName:   "tokenrouter",
		SSLMode:  "disable",
	}

	bootstrapDSN, targetDSN := BuildDatabaseConnectionDSNs(cfg)

	maintenance, err := url.Parse(bootstrapDSN)
	require.NoError(t, err)
	target, err := url.Parse(targetDSN)
	require.NoError(t, err)
	require.Equal(t, "/postgres", maintenance.Path)
	require.Equal(t, "/tokenrouter", target.Path)
}

// TestBuildPostgresDSNPreservesEmptyAndSpecialCredentials checks setup credentials
// containing empty text, separators, and whitespace. URL decoding must preserve
// those values and the selected database without treating another field as password.
func TestBuildPostgresDSNPreservesEmptyAndSpecialCredentials(t *testing.T) {
	for _, password := range []string{"", "a b'\\?@/"} {
		cfg := &SetupDatabaseConfig{Host: "::1", Port: 5432, User: "setup user", Password: password, DBName: "requested_db", SSLMode: "disable"}
		dsn, err := url.Parse(BuildPostgresDSN(cfg, cfg.DBName))
		require.NoError(t, err)
		require.Equal(t, "postgres", dsn.Scheme)
		decoded, present := dsn.User.Password()
		require.True(t, present)
		require.Equal(t, password, decoded)
		require.Equal(t, "setup user", dsn.User.Username())
		require.Equal(t, "/requested_db", dsn.Path)
		require.Equal(t, "[::1]:5432", dsn.Host)
	}
}
