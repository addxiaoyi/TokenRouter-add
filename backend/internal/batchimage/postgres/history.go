package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/TokenFlux/TokenRouter/internal/batchimage"
)

// history.go reads safe console metadata from durable batch jobs using a
// projection separate from the gateway's API-key-scoped resource. Filters are
// parameterized and user-deleted rows remain hidden across all panel views.

// HistoryStore owns the bounded PostgreSQL query for console history.
type HistoryStore struct {
	db SQLExecutor // batch metadata query executor
}

// NewHistoryStore retains the application pool without starting background work.
func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// List reads the current page of visible task summaries. The caller supplies
// verified scope and a positive cursor when continuing an earlier page.
// The exclusive primary-key boundary keeps newly inserted jobs from moving
// entries between the remaining pages. One extra row establishes whether a
// next page exists without counting all matching jobs. Deleted jobs are
// excluded on every page and may disappear from later reads.
//
// The user-facing model comes only from requested_model because old jobs may
// lack this value and the stored execution model can identify an upstream route.
// @project-doc docs/domains/batch_image_jobs.md#panel_task_history
func (s *HistoryStore) List(ctx context.Context, filter batchimage.HistoryFilter) (_ batchimage.HistoryPage, err error) {
	page := batchimage.HistoryPage{Items: make([]batchimage.HistoryEntry, 0, batchimage.HistoryPageSize)}
	query := `SELECT id, batch_id, user_id, COALESCE(requested_model, ''), status,
	item_count, success_count, fail_count, cancelled_count, created_at, finished_at
FROM batch_image_jobs WHERE user_deleted_at IS NULL`
	args := make([]any, 0, 4)
	if filter.UserID > 0 {
		args = append(args, filter.UserID)
		query += " AND user_id = $" + strconv.Itoa(len(args))
	}
	if filter.BeforeID > 0 {
		args = append(args, filter.BeforeID)
		query += " AND id < $" + strconv.Itoa(len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += " AND status = $" + strconv.Itoa(len(args))
	}
	if filter.BatchID != "" {
		args = append(args, filter.BatchID)
		query += " AND batch_id = $" + strconv.Itoa(len(args))
	}
	// The extra row determines pagination without scanning or counting the
	// remaining matching rows, which may include the full administrator history.
	args = append(args, batchimage.HistoryPageSize+1)
	query += " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return page, fmt.Errorf("list batch task history: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	var lastID int64
	for rows.Next() {
		// Return a cursor for the last delivered row so the lookahead row appears
		// at the start of the following page without being read into the response.
		if len(page.Items) == batchimage.HistoryPageSize {
			page.NextCursor = strconv.FormatInt(lastID, 10)
			break
		}
		var item batchimage.HistoryEntry
		if err := rows.Scan(&lastID, &item.ID, &item.UserID, &item.Model, &item.Status, &item.ItemCount,
			&item.SuccessCount, &item.FailureCount, &item.CancelledCount, &item.CreatedAt, &item.FinishedAt); err != nil {
			return page, fmt.Errorf("scan batch task history: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}
