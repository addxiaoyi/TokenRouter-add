package support

import (
	"context"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// models.go defines support conversations, access scope, and persistence contracts.
// Ticket ownership comes from authenticated identity, while staff access comes
// from protected administrator routes. Messages preserve their authors and order;
// versioned status changes avoid silently replacing another operator's decision.

// Ticket failures retain stable HTTP categories without exposing another user's records.
var (
	ErrNotFound = apperror.New(apperror.CategoryNotFound, "SUPPORT_NOT_FOUND", "Ticket not found")
	ErrConflict = apperror.New(apperror.CategoryConflict, "SUPPORT_CONFLICT", "The ticket changed. Refresh it and try again.")
	ErrClosed   = apperror.New(apperror.CategoryConflict, "SUPPORT_CLOSED", "Reopen the ticket before replying.")
	ErrLimit    = apperror.New(apperror.CategoryTooManyRequests, "SUPPORT_LIMIT", "Resolve an existing ticket before opening another.")
)

// Actor identifies the authenticated account and its route-authorized staff scope.
type Actor struct {
	UserID int64 // authenticated account identifier
	Staff  bool  // administrator route authorization
}

// Ticket describes an owned support conversation and its latest workflow state.
type Ticket struct {
	ID           int64      `json:"id"`                    // conversation database identifier
	UserID       int64      `json:"user_id"`               // owning account identifier
	Username     string     `json:"username"`              // owning account display name
	Subject      string     `json:"subject"`               // user supplied conversation title
	Status       string     `json:"status"`                // current workflow state
	Version      int64      `json:"version"`               // optimistic status update revision
	CreatedAt    time.Time  `json:"created_at"`            // original submission instant
	UpdatedAt    time.Time  `json:"updated_at"`            // latest reply or status instant
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"` // completion instant when resolved or closed
	LegacyID     *int64     `json:"legacy_id,omitempty"`   // original station ticket identifier
	MessageCount int        `json:"message_count"`         // persisted thread message count
}

// Message preserves one immutable reply and the author's role at submission.
type Message struct {
	ID        int64     `json:"id"`                  // persisted message identifier
	AuthorID  *int64    `json:"author_id,omitempty"` // original account when retained
	Author    string    `json:"author"`              // public author display name
	Staff     bool      `json:"is_staff"`            // staff identity at submission
	Body      string    `json:"body"`                // plain text message content
	CreatedAt time.Time `json:"created_at"`          // message submission instant
}

// Thread combines conversation state with a bounded chronological message page.
type Thread struct {
	Ticket   Ticket    `json:"ticket"`   // current conversation metadata
	Messages []Message `json:"messages"` // messages following requested cursor
	HasMore  bool      `json:"has_more"` // another chronological page exists
	NextID   int64     `json:"next_id"`  // final message cursor for continuation
}

// Page is a bounded ticket list with its complete filtered count.
type Page struct {
	Items    []Ticket `json:"items"`     // matching conversation records
	Total    int      `json:"total"`     // total matching conversation count
	Page     int      `json:"page"`      // one-based current page number
	PageSize int      `json:"page_size"` // maximum records per page
}

// Filter selects a bounded ticket page within the caller's authorized scope.
type Filter struct {
	Status   string // optional exact workflow state
	Search   string // literal title or identifier search
	Page     int    // one-based requested page
	PageSize int    // records per page
}

// CreateInput supplies a new conversation and a stable retry identity.
type CreateInput struct {
	Subject  string `json:"subject"`   // conversation title in plain text
	Message  string `json:"message"`   // initial plain text description
	ClientID string `json:"client_id"` // UUID retained across submission retries
}

// ReplyInput appends one message without replacing previous conversation history.
type ReplyInput struct {
	Message  string `json:"message"`   // plain text reply content
	ClientID string `json:"client_id"` // UUID retained across reply retries
}

// StatusInput changes workflow state only against the displayed revision.
type StatusInput struct {
	Status  string `json:"status"`  // requested workflow destination
	Version int64  `json:"version"` // last observed ticket revision
}

// Store persists authorized conversations and notification intents atomically.
type Store interface {
	// List returns a bounded page scoped to the supplied authenticated actor.
	List(context.Context, Actor, Filter) (Page, error)
	// Get returns one accessible ticket and messages after the given cursor.
	Get(context.Context, Actor, int64, int64) (Thread, error)
	// Create atomically inserts the conversation, first message, and notification intents.
	// Reusing a client identity returns its original ticket without creating another.
	Create(context.Context, Actor, CreateInput) (int64, error)
	// Reply appends an immutable message under the ticket row lock.
	// Duplicate retry identities do not append again, and closed tickets reject new replies.
	Reply(context.Context, Actor, int64, ReplyInput) error
	// SetStatus checks ownership and revision before updating one ticket's state.
	SetStatus(context.Context, Actor, int64, StatusInput) error
	// Recipients reads configured staff notification addresses without contacting SMTP.
	Recipients(context.Context) ([]string, error)
	// SetRecipients atomically replaces the validated staff notification address list.
	SetRecipients(context.Context, []string) error
}
