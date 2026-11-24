package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/support"
)

// store.go persists support conversations with ownership and retry protection.
// Reads scope every ticket to its owner unless the authenticated route grants
// staff access. New submissions serialize on the owning user so concurrent
// requests cannot exceed the open-ticket limit, and unique client identities
// make a lost HTTP response safe to retry without duplicate conversations.
//
// Replies and status changes lock the ticket before testing state or revision.
// Each message and its email intents commit together; SMTP is handled after
// commitment by a separate delivery worker. Failed delivery never removes a
// conversation or hides its reply from either participant.

const (
	// Staff notification addresses use one settings record and stay outside user responses.
	recipientsKey = "support_ticket_recipients"
	// ticketColumns keeps list and detail projections consistent across scanning callers.
	ticketColumns = `t.id,t.user_id,COALESCE(u.username,''),t.subject,t.status,t.version,t.created_at,t.updated_at,t.resolved_at,t.legacy_id,(SELECT count(*) FROM support_ticket_messages m WHERE m.ticket_id=t.id)`
)

// Store implements PostgreSQL conversation access and transactional message persistence.
type (
	Store struct{ db *sql.DB } // externally managed application connection pool
	// scanner allows row and row-set projections to share ticket decoding.
	scanner interface{ Scan(...any) error }
)

// NewStore binds the application's connection pool without executing any queries.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// scanTicket decodes a complete conversation projection and preserves nullable history.
func scanTicket(row scanner) (support.Ticket, error) {
	var ticket support.Ticket
	err := row.Scan(&ticket.ID, &ticket.UserID, &ticket.Username, &ticket.Subject, &ticket.Status, &ticket.Version, &ticket.CreatedAt, &ticket.UpdatedAt, &ticket.ResolvedAt, &ticket.LegacyID, &ticket.MessageCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ticket, support.ErrNotFound
	}
	return ticket, err
}

// List reads a filtered page within one authorization predicate.
// Search is a literal substring, not an SQL wildcard expression; ordering uses
// update time and ID so records with equal timestamps remain deterministic.
func (s *Store) List(ctx context.Context, actor support.Actor, filter support.Filter) (_ support.Page, returnErr error) {
	page := support.Page{Items: []support.Ticket{}, Page: filter.Page, PageSize: filter.PageSize}
	where := ` WHERE ($1::boolean OR t.user_id=$2) AND ($3='' OR t.status=$3) AND ($4='' OR strpos(lower(t.subject),lower($4))>0 OR t.id::text=$4)`
	args := []any{actor.Staff, actor.UserID, filter.Status, filter.Search}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM support_tickets t`+where, args...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count support tickets: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+ticketColumns+` FROM support_tickets t JOIN users u ON u.id=t.user_id`+where+` ORDER BY t.updated_at DESC,t.id DESC LIMIT $5 OFFSET $6`, append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...)
	if err != nil {
		return page, fmt.Errorf("list support tickets: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, ticket)
	}
	return page, rows.Err()
}

// Get checks ticket ownership before reading its bounded chronological replies.
// The first fifty messages are returned after the cursor, with one lookahead
// record detecting continuation. Unknown and unauthorized identifiers share the
// same not-found result so callers cannot enumerate another user's conversations.
func (s *Store) Get(ctx context.Context, actor support.Actor, id, after int64) (_ support.Thread, returnErr error) {
	thread := support.Thread{Messages: []support.Message{}}
	ticket, err := scanTicket(s.db.QueryRowContext(ctx, `SELECT `+ticketColumns+` FROM support_tickets t JOIN users u ON u.id=t.user_id WHERE t.id=$1 AND ($2::boolean OR t.user_id=$3)`, id, actor.Staff, actor.UserID))
	if err != nil {
		return thread, err
	}
	thread.Ticket = ticket
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.author_id,COALESCE(u.username,''),m.is_staff,m.body,m.created_at FROM support_ticket_messages m LEFT JOIN users u ON u.id=m.author_id WHERE m.ticket_id=$1 AND m.id>$2 ORDER BY m.id LIMIT 51`, id, after)
	if err != nil {
		return thread, err
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	for rows.Next() {
		var message support.Message
		if err := rows.Scan(&message.ID, &message.AuthorID, &message.Author, &message.Staff, &message.Body, &message.CreatedAt); err != nil {
			return thread, err
		}
		if len(thread.Messages) == 50 {
			thread.HasMore = true
			break
		}
		thread.Messages = append(thread.Messages, message)
		thread.NextID = message.ID
	}
	return thread, rows.Err()
}

// Create commits a new ticket and notification intents as one transaction.
// The owner lock serializes open-ticket limits and retry lookups. Existing client
// identities return their committed ticket, while a changed title with the same
// identity is rejected. Every failure rolls back both the conversation and its
// first message; successful commits alone make the ticket visible to readers.
func (s *Store) Create(ctx context.Context, actor support.Actor, input support.CreateInput) (_ int64, returnErr error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { returnErr = rollback(tx, returnErr) }()
	var owner int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, actor.UserID).Scan(&owner); err != nil {
		return 0, err
	}
	var id int64
	var subject, body string
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.subject,m.body FROM support_tickets t JOIN support_ticket_messages m ON m.ticket_id=t.id AND m.client_id=t.client_id WHERE t.user_id=$1 AND t.client_id=$2`, actor.UserID, input.ClientID).Scan(&id, &subject, &body)
	if err == nil {
		if subject != input.Subject || body != input.Message {
			return 0, support.ErrConflict
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var open int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM support_tickets WHERE user_id=$1 AND status IN ('open','in_progress')`, actor.UserID).Scan(&open); err != nil {
		return 0, err
	}
	if open >= 20 {
		return 0, support.ErrLimit
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO support_tickets(user_id,subject,client_id) VALUES($1,$2,$3) RETURNING id`, actor.UserID, input.Subject, input.ClientID).Scan(&id); err != nil {
		return 0, err
	}
	if err = insertMessage(ctx, tx, actor, id, input.Message, input.ClientID); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Reply locks an accessible ticket before checking retry identity and state.
// A retry of a previously committed message succeeds even if staff closed the
// ticket afterward. New owner replies reopen resolved conversations; new staff
// replies move open conversations into progress. Closed tickets reject new
// messages until explicitly reopened, and notification intents share the commit.
func (s *Store) Reply(ctx context.Context, actor support.Actor, id int64, input support.ReplyInput) (returnErr error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { returnErr = rollback(tx, returnErr) }()
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM support_tickets WHERE id=$1 AND ($2::boolean OR user_id=$3) FOR UPDATE`, id, actor.Staff, actor.UserID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return support.ErrNotFound
	}
	if err != nil {
		return err
	}
	var body string
	var author int64
	err = tx.QueryRowContext(ctx, `SELECT body,author_id FROM support_ticket_messages WHERE ticket_id=$1 AND client_id=$2`, id, input.ClientID).Scan(&body, &author)
	if err == nil {
		if body != input.Message || author != actor.UserID {
			return support.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if status == "closed" {
		return support.ErrClosed
	}
	if err = insertMessage(ctx, tx, actor, id, input.Message, input.ClientID); err != nil {
		return err
	}
	next := "open"
	if actor.Staff {
		next = "in_progress"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE support_tickets SET status=$2,version=version+1,updated_at=NOW(),resolved_at=NULL WHERE id=$1`, id, next); err != nil {
		return err
	}
	return tx.Commit()
}

// SetStatus changes an accessible ticket only when its revision still matches.
// Completion states retain a completion timestamp; reopening clears that value.
// A zero-row update distinguishes an inaccessible ticket from a concurrent edit
// without exposing the state of another user's conversation.
func (s *Store) SetStatus(ctx context.Context, actor support.Actor, id int64, input support.StatusInput) error {
	change, err := s.db.ExecContext(ctx, `UPDATE support_tickets SET status=$4::text,version=version+1,updated_at=NOW(),resolved_at=CASE WHEN $4::text IN ('resolved','closed') THEN NOW() ELSE NULL END WHERE id=$1 AND ($2::boolean OR user_id=$3) AND version=$5`, id, actor.Staff, actor.UserID, input.Status, input.Version)
	if err != nil {
		return err
	}
	count, err := change.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var found bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM support_tickets WHERE id=$1 AND ($2::boolean OR user_id=$3))`, id, actor.Staff, actor.UserID).Scan(&found); err != nil {
		return err
	}
	if !found {
		return support.ErrNotFound
	}
	return support.ErrConflict
}

// insertMessage writes the reply and recipient intents on the caller's transaction.
// Staff replies notify the owning account address; owner replies notify the
// configured support addresses. Unconfigured staff delivery leaves the ticket
// available in the dashboard, and migration placeholder addresses are excluded.
func insertMessage(ctx context.Context, tx *sql.Tx, actor support.Actor, id int64, body, clientID string) error {
	var messageID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO support_ticket_messages(ticket_id,author_id,is_staff,body,client_id) VALUES($1,$2,$3,$4,$5) RETURNING id`, id, actor.UserID, actor.Staff, body, clientID).Scan(&messageID); err != nil {
		return err
	}
	var recipients []string
	if actor.Staff {
		var address string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(u.email,'') FROM users u JOIN support_tickets t ON t.user_id=u.id WHERE t.id=$1 AND u.deleted_at IS NULL`, id).Scan(&address); err != nil {
			return err
		}
		if address != "" && !strings.HasSuffix(strings.ToLower(address), ".invalid") {
			recipients = []string{address}
		}
	} else {
		var raw string
		err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, recipientsKey).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if raw != "" {
			if err = json.Unmarshal([]byte(raw), &recipients); err != nil {
				return fmt.Errorf("decode support recipients: %w", err)
			}
		}
	}
	for _, address := range recipients {
		if _, err := tx.ExecContext(ctx, `INSERT INTO support_ticket_notifications(ticket_id,message_id,recipient) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, messageID, address); err != nil {
			return err
		}
	}
	return nil
}

// Recipients returns configured staff addresses, treating an absent record as empty.
// Invalid saved JSON is reported so administrators can repair the configuration;
// it is never silently replaced with a different delivery destination.
func (s *Store) Recipients(ctx context.Context) ([]string, error) {
	var raw string
	recipients := []string{}
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, recipientsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return recipients, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &recipients); err != nil {
		return nil, err
	}
	return recipients, nil
}

// SetRecipients stores the validated list as one atomic settings upsert.
// Existing queued messages keep their intended destinations; new messages use
// this list when their transaction creates notification intents.
func (s *Store) SetRecipients(ctx context.Context, recipients []string) error {
	body, err := json.Marshal(recipients)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=NOW()`, recipientsKey, string(body))
	return err
}

// rollback releases unfinished transactions while preserving the original failure.
// A transaction already committed or rolled back needs no further work; every
// other rollback failure is joined so callers retain cleanup diagnostics.
func rollback(tx *sql.Tx, failure error) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return errors.Join(failure, err)
	}
	return failure
}
