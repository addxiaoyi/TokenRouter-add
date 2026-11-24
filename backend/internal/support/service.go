package support

import (
	"context"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// service.go validates support inputs before calling owned conversation storage.
// Caller scope is supplied by authentication rather than request JSON. Character
// limits count Unicode code points, stable client IDs make retries safe, and
// recipient validation keeps notification configuration separate from ticket creation.

// Service exposes validated conversation operations over the supplied persistent store.
// @project-doc docs/domains/support_conversations.md#ownership_and_workflow
type Service struct{ store Store } // owned ticket persistence contract

// NewService binds persistent support storage without reading or changing records.
func NewService(store Store) *Service { return &Service{store: store} }

// validStatus reports whether the value identifies a supported ticket state.
func validStatus(status string) bool {
	return status == "open" || status == "in_progress" || status == "resolved" || status == "closed"
}

// invalid creates a client-visible support validation error with a stable reason.
func invalid(message string) error {
	return apperror.New(apperror.CategoryBadRequest, "SUPPORT_INVALID", message)
}

// List normalizes pagination and validates filters before querying the caller's scope.
// An empty status selects every state; omitted page settings select twenty records
// on the first page. Excessive search text is rejected without a storage query.
func (s *Service) List(ctx context.Context, actor Actor, filter Filter) (Page, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if utf8.RuneCountInString(filter.Search) > 100 || filter.Status != "" && !validStatus(filter.Status) {
		return Page{}, invalid("Invalid ticket search or status")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 50 {
		filter.PageSize = 20
	}
	if filter.Page > 100000 {
		return Page{}, invalid("Ticket page is too large")
	}
	return s.store.List(ctx, actor, filter)
}

// Get loads one accessible conversation after a chronological message cursor.
// IDs must be positive and the cursor nonnegative; storage uses the caller's
// ownership scope even when the requested identifier belongs to another account.
func (s *Service) Get(ctx context.Context, actor Actor, id, after int64) (Thread, error) {
	if id <= 0 || after < 0 {
		return Thread{}, invalid("Invalid ticket identifier")
	}
	return s.store.Get(ctx, actor, id, after)
}

// Create validates a new subject, message, and retry identity before persistence.
// SMTP transport is deferred until after persistence; malformed stored recipient
// configuration still rejects the transaction. The returned thread reflects the
// committed ticket, including an initial reply visible without email delivery.
func (s *Service) Create(ctx context.Context, actor Actor, input CreateInput) (Thread, error) {
	input.Subject, input.Message = strings.TrimSpace(input.Subject), strings.TrimSpace(input.Message)
	length := utf8.RuneCountInString(input.Subject)
	if length < 2 || length > 160 || strings.ContainsAny(input.Subject, "\r\n\x00") {
		return Thread{}, invalid("Ticket subject must contain 2 to 160 characters")
	}
	if err := validateMessage(input.Message, input.ClientID); err != nil {
		return Thread{}, err
	}
	id, err := s.store.Create(ctx, actor, input)
	if err != nil {
		return Thread{}, err
	}
	return s.store.Get(ctx, actor, id, 0)
}

// validateMessage requires bounded plain text and a UUID that survives retries.
// NUL bytes are rejected before PostgreSQL text encoding; whitespace-only bodies
// cannot create empty entries in the persisted support conversation.
func validateMessage(body, clientID string) error {
	length := utf8.RuneCountInString(body)
	if length < 1 || length > 10000 || strings.ContainsRune(body, 0) {
		return invalid("Ticket message must contain 1 to 10000 characters")
	}
	if _, err := uuid.Parse(clientID); err != nil {
		return invalid("Invalid ticket submission identifier")
	}
	return nil
}

// Reply validates and appends an immutable reply within the actor's ticket scope.
// Closed tickets require an explicit reopen first. A repeated client identifier
// is handled by storage so a lost HTTP response cannot duplicate the message.
func (s *Service) Reply(ctx context.Context, actor Actor, id int64, input ReplyInput) error {
	input.Message = strings.TrimSpace(input.Message)
	if id <= 0 {
		return invalid("Invalid ticket identifier")
	}
	if err := validateMessage(input.Message, input.ClientID); err != nil {
		return err
	}
	return s.store.Reply(ctx, actor, id, input)
}

// SetStatus validates a revision-bound workflow change before persistence.
// Owners may close or reopen their own ticket; progress and resolution states
// require staff authorization supplied by the administrator route.
func (s *Service) SetStatus(ctx context.Context, actor Actor, id int64, input StatusInput) error {
	if id <= 0 || input.Version <= 0 || !validStatus(input.Status) {
		return invalid("Invalid ticket state or revision")
	}
	if !actor.Staff && input.Status != "open" && input.Status != "closed" {
		return invalid("This ticket status requires support staff")
	}
	return s.store.SetStatus(ctx, actor, id, input)
}

// Recipients returns the staff notification configuration to administrator callers.
func (s *Service) Recipients(ctx context.Context) ([]string, error) { return s.store.Recipients(ctx) }

// SetRecipients validates and deduplicates up to twenty plain email addresses.
// An empty list disables staff email delivery while preserving normal ticket
// submission and replies. Display-name addresses and invalid domains are rejected.
func (s *Service) SetRecipients(ctx context.Context, recipients []string) ([]string, error) {
	if len(recipients) > 20 {
		return nil, invalid("At most twenty notification recipients are allowed")
	}
	clean := make([]string, 0, len(recipients))
	seen := map[string]bool{}
	for _, recipient := range recipients {
		address := strings.TrimSpace(recipient)
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.HasSuffix(strings.ToLower(address), ".invalid") {
			return nil, invalid("Enter valid notification email addresses")
		}
		if !seen[strings.ToLower(address)] {
			clean = append(clean, address)
			seen[strings.ToLower(address)] = true
		}
	}
	if err := s.store.SetRecipients(ctx, clean); err != nil {
		return nil, err
	}
	return clean, nil
}
