//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	infra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/support"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// store_integration_test.go verifies support ownership and atomic persistence
// against PostgreSQL. The ordinary suite creates an isolated database; an
// explicitly supplied support_test database can run compiled tests inside a
// network-isolated release probe. Neither mode accesses application production data.

// ticketDatabase creates an isolated migrated database and owns its cleanup.
// External DSNs must identify support_test, which the release probe creates
// inside its private container. Ordinary runs use the repository's container suite.
func ticketDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SUPPORT_TEST_DSN")
	if dsn == "" {
		suite := &postgrescontainer.Suite{}
		t.Cleanup(func() { require.NoError(t, suite.Close()) })
		return suite.New(t)
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var name string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))
	require.Equal(t, "support_test", name)
	require.NoError(t, infra.ApplyMigrations(context.Background(), db, migrations.FS))
	return db
}

// TestSupportWorkflow persists retries, replies, owner isolation, and notification intents.
// Concurrent submissions must produce one ticket; concurrent status changes
// cannot overwrite the same revision. Failed mail and explicit retry preserve
// the entire thread, and transaction failure leaves no partial conversation.
func TestSupportWorkflow(t *testing.T) {
	db := ticketDatabase(t)
	ctx := context.Background()
	store := NewStore(db)
	var owner, other, staff int64
	for i, target := range []*int64{&owner, &other, &staff} {
		require.NoError(t, db.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES($1,'test-hash',$2) RETURNING id`, uuid.NewString()+"@example.com", []string{"owner", "other", "staff"}[i]).Scan(target))
	}
	user := support.Actor{UserID: owner}
	admin := support.Actor{UserID: staff, Staff: true}
	input := support.CreateInput{Subject: "A support issue", Message: "Details remain private", ClientID: uuid.NewString()}
	var wg sync.WaitGroup
	ids := make(chan int64, 5)
	failures := make(chan error, 5)
	for range 5 {
		wg.Go(func() { id, err := store.Create(ctx, user, input); ids <- id; failures <- err })
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	var id int64
	for value := range ids {
		if id == 0 {
			id = value
		}
		require.Equal(t, id, value)
	}
	thread, err := store.Get(ctx, user, id, 0)
	require.NoError(t, err)
	require.Len(t, thread.Messages, 1)
	_, err = store.Get(ctx, support.Actor{UserID: other}, id, 0)
	require.ErrorIs(t, err, support.ErrNotFound)
	require.ErrorIs(t, store.Reply(ctx, support.Actor{UserID: other}, id, support.ReplyInput{Message: "intrusion", ClientID: uuid.NewString()}), support.ErrNotFound)
	require.ErrorIs(t, store.SetStatus(ctx, support.Actor{UserID: other}, id, support.StatusInput{Status: "closed", Version: 1}), support.ErrNotFound)
	require.NoError(t, store.SetRecipients(ctx, []string{"support@example.com"}))
	reply := support.ReplyInput{Message: "Please investigate", ClientID: uuid.NewString()}
	require.NoError(t, store.Reply(ctx, user, id, reply))
	require.NoError(t, store.Reply(ctx, user, id, reply))
	thread, err = store.Get(ctx, admin, id, 0)
	require.NoError(t, err)
	require.Len(t, thread.Messages, 2)
	notice, err := store.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, notice)
	require.Equal(t, "support@example.com", notice.Recipient)
	require.NoError(t, store.Complete(ctx, *notice, false))
	state, err := store.DeliveryState(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, state.Pending)
	stale := *notice
	_, err = db.Exec(`UPDATE support_ticket_notifications SET attempts=10 WHERE id=$1`, notice.ID)
	require.NoError(t, err)
	require.NoError(t, store.RetryFailed(ctx))
	notice, err = store.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, notice)
	// A delayed acknowledgement from before manual retry must not complete the
	// new delivery lease, even when the retry counter has returned to one.
	require.NoError(t, store.Complete(ctx, stale, true))
	state, err = store.DeliveryState(ctx)
	require.NoError(t, err)
	require.Zero(t, state.Delivered)
	require.NoError(t, store.Complete(ctx, *notice, true))
	state, err = store.DeliveryState(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, state.Delivered)
	require.NoError(t, store.SetStatus(ctx, admin, id, support.StatusInput{Status: "resolved", Version: thread.Ticket.Version}))
	require.ErrorIs(t, store.SetStatus(ctx, admin, id, support.StatusInput{Status: "closed", Version: thread.Ticket.Version}), support.ErrConflict)
	require.NoError(t, store.Reply(ctx, user, id, support.ReplyInput{Message: "Still need help", ClientID: uuid.NewString()}))
	thread, err = store.Get(ctx, user, id, 0)
	require.NoError(t, err)
	require.Equal(t, "open", thread.Ticket.Status)
	require.Nil(t, thread.Ticket.ResolvedAt)
	require.NoError(t, store.SetStatus(ctx, user, id, support.StatusInput{Status: "closed", Version: thread.Ticket.Version}))
	require.ErrorIs(t, store.Reply(ctx, user, id, support.ReplyInput{Message: "new", ClientID: uuid.NewString()}), support.ErrClosed)
	// Corrupt notification settings force an error after message insertion. The
	// same transaction must discard both the ticket and initial message.
	_, err = db.Exec(`UPDATE settings SET value='invalid' WHERE key=$1`, recipientsKey)
	require.NoError(t, err)
	_, err = store.Create(ctx, user, support.CreateInput{Subject: "Rollback proof", Message: "Must not remain", ClientID: uuid.NewString()})
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM support_tickets WHERE subject='Rollback proof'`).Scan(&count))
	require.Zero(t, count)
}
