//go:build integration

package postgres

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/batchimage"
)

// history_integration_test.go exercises owner filters and stable pagination
// against PostgreSQL's actual batch schema. Fixtures include deleted records,
// a different owner, and a newly inserted job between the two page reads.

// TestHistoryStoreScopes lists the first page for one owner, inserts a newer
// job, then resumes using the original cursor. It also checks administrator
// search and a historical job with only an internal model name.
func TestHistoryStoreScopes(t *testing.T) {
	ctx := t.Context()
	tx := historyTestTx(t)
	_, owner := newBatchImageRepositoryWithSQL(t, tx)
	var other int64
	err := tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status)
VALUES ('history-other@example.com','test-password-hash','user','active')
RETURNING id`).Scan(&other)
	require.NoError(t, err)
	for i := range batchimage.HistoryPageSize + 2 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO batch_image_jobs (batch_id, user_id, billing_user_id, platform, model, requested_model, status, item_count)
VALUES ($1, $2, $2, 'gemini_api', 'upstream-model', 'public-model', 'completed', 1)
`, fmt.Sprintf("history-owner-%02d", i), owner)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO batch_image_jobs (batch_id, user_id, billing_user_id, platform, model, status, item_count, user_deleted_at)
VALUES ('history-other', $1, $1, 'gemini_api', 'private-model', 'completed', 1, NULL),
       ('history-hidden', $2, $2, 'gemini_api', 'private-model', 'completed', 1, NOW())
`, other, owner)
	require.NoError(t, err)
	store := &HistoryStore{db: tx}
	page, err := store.List(ctx, batchimage.HistoryFilter{UserID: owner, Status: "completed"})
	require.NoError(t, err)
	require.Len(t, page.Items, batchimage.HistoryPageSize)
	require.Equal(t, "history-owner-26", page.Items[0].ID)
	require.Equal(t, "public-model", page.Items[0].Model)
	cursor, err := strconv.ParseInt(page.NextCursor, 10, 64)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
INSERT INTO batch_image_jobs (batch_id, user_id, billing_user_id, platform, model, status, item_count)
VALUES ('history-arrived-late', $1, $1, 'gemini_api', 'private-model', 'completed', 1)
`, owner)
	require.NoError(t, err)
	next, err := store.List(ctx, batchimage.HistoryFilter{UserID: owner, BeforeID: cursor, Status: "completed"})
	require.NoError(t, err)
	require.Len(t, next.Items, 2)
	require.Equal(t, "history-owner-01", next.Items[0].ID)
	require.Equal(t, "history-owner-00", next.Items[1].ID)
	require.Empty(t, next.NextCursor)

	for _, id := range []string{"history-other", "history-hidden"} {
		visible, err := store.List(ctx, batchimage.HistoryFilter{UserID: owner, BatchID: id})
		require.NoError(t, err)
		require.Empty(t, visible.Items)
	}
	admin, err := store.List(ctx, batchimage.HistoryFilter{BatchID: "history-other", UserID: other})
	require.NoError(t, err)
	require.Len(t, admin.Items, 1)
	require.Equal(t, other, admin.Items[0].UserID)
	require.Empty(t, admin.Items[0].Model)
}

// historyTestTx uses the repository's normal migrated database fixture unless
// release verification supplies an already-migrated isolated test instance.
// The caller must never point the override at a production database. Its
// rollback is registered after pool cleanup so the test releases rows first.
func historyTestTx(t *testing.T) *sql.Tx {
	t.Helper()
	dsn := os.Getenv("TOKENROUTER_TEST_HISTORY_DSN")
	if dsn == "" {
		return testTx(t)
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}
