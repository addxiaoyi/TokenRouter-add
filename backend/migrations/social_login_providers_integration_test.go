//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// social_login_providers_integration_test.go runs the complete embedded migration
// chain on PostgreSQL and verifies social identity constraints afterward. A
// second runner pass protects repeat startup while inserts prove that preserved
// and new provider labels remain accepted by the production database.

// TestSocialLoginProviderMigration upgrades an empty PostgreSQL database twice.
// Telegram and Discord users and identities remain insertable afterward, while
// unsupported labels and duplicate external subjects are rejected.
func TestSocialLoginProviderMigration(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:18.1-alpine3.23", tcpostgres.WithDatabase("social"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), tcpostgres.BasicWaitStrategies())
	// Register termination before checking startup because a failed startup can
	// still own a container that needs a separate cleanup context and deadline.
	if container != nil {
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.NoError(t, container.Terminate(cleanup))
		})
	}
	require.NoError(t, err)
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, postgres.ApplyMigrations(ctx, db, migrations.FS))
	require.NoError(t, postgres.ApplyMigrations(ctx, db, migrations.FS))
	for _, provider := range []string{"telegram", "discord", "qq", "phone"} {
		var userID int64
		err := db.QueryRowContext(ctx, "INSERT INTO users (email,password_hash,signup_source) VALUES ($1,'hash',$2) RETURNING id", provider+"@example.com", provider).Scan(&userID)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "INSERT INTO auth_identities (user_id,provider_type,provider_key,provider_subject) VALUES ($1,$2,$3,'123456789')", userID, provider, provider)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "INSERT INTO auth_identities (user_id,provider_type,provider_key,provider_subject) VALUES ($1,$2,$3,'123456789')", userID, provider, provider)
		require.Error(t, err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO users (email,password_hash,signup_source) VALUES ('invalid@example.com','hash','untrusted')")
	require.Error(t, err)
}
