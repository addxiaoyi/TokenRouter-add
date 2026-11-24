package batchimage

import "time"

// history.go defines the read-only panel projection for persisted batch work.
// A panel session may see jobs across its own keys without gaining the
// gateway's separate permissions for inspecting items, downloading outputs,
// or mutating tasks. The projection keeps execution inputs outside the console.

// The history page size bounds both database reads and the console payload.
const (
	HistoryPageSize = 25
)

// HistoryFilter selects one bounded page of visible batch task metadata.
type HistoryFilter struct {
	// UserID limits history to the submitting user across their keys. Zero means
	// administrator scope across all users and must never come from a user route.
	UserID   int64  // submitting account identifier
	BeforeID int64  // exclusive row cursor
	Status   string // exact stored lifecycle filter
	BatchID  string // exact public batch identifier
}

// HistoryEntry describes a batch task without execution inputs or upstream details.
type HistoryEntry struct {
	ID             string     `json:"id"`                    // public batch identifier
	UserID         int64      `json:"user_id,omitempty"`     // administrator-visible submitter ID
	Model          string     `json:"model"`                 // model requested by client
	Status         string     `json:"status"`                // persisted lifecycle state
	ItemCount      int        `json:"item_count"`            // total requested item count
	SuccessCount   int        `json:"success_count"`         // successfully generated item count
	FailureCount   int        `json:"failure_count"`         // failed item count
	CancelledCount int        `json:"cancelled_count"`       // cancelled item count
	CreatedAt      time.Time  `json:"created_at"`            // original submission timestamp
	FinishedAt     *time.Time `json:"finished_at,omitempty"` // recorded completion timestamp
}

// HistoryPage carries newest-first task metadata and an optional next cursor.
type HistoryPage struct {
	Items      []HistoryEntry `json:"items"`                 // visible batch history entries
	NextCursor string         `json:"next_cursor,omitempty"` // exclusive cursor for remaining rows
}

// ValidHistoryStatus reports whether a filter names a persisted batch status.
func ValidHistoryStatus(status string) bool {
	switch status {
	case "", BatchImageJobStatusCreated, BatchImageJobStatusUploading, BatchImageJobStatusSubmitted,
		BatchImageJobStatusRunning, BatchImageJobStatusIndexing, BatchImageJobStatusSettling,
		BatchImageJobStatusCompleted, BatchImageJobStatusFailed, BatchImageJobStatusCancelled,
		BatchImageJobStatusOutputDeleted:
		return true
	default:
		return false
	}
}
