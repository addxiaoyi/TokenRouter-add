package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/TokenFlux/TokenRouter/internal/support"
)

// delivery.go leases persisted support email intents and tracks their outcomes.
// A two-minute lease outlives the worker's send and acknowledgement budgets,
// and SKIP LOCKED permits independent instances to claim different recipients.
// Unique claim identities prevent expired senders from completing newer work.
//
// Failed sends retry after a minute for up to ten claims. Exhausted notices
// remain visible to administrators and can be reset explicitly after SMTP
// repair. Delivery never changes ticket state, and queued recipient addresses
// remain fixed even when support notification settings later change.

// Claim atomically leases one due notification and returns its message contents.
// No due row returns nil without an error. The update and attempt increment
// happen in one statement, so concurrent workers cannot claim the same lease.
func (s *Store) Claim(ctx context.Context) (*support.Notice, error) {
	var notice support.Notice
	err := s.db.QueryRowContext(ctx, `WITH due AS (SELECT id FROM support_ticket_notifications WHERE delivered_at IS NULL AND attempts<10 AND available_at<=NOW() ORDER BY available_at,id FOR UPDATE SKIP LOCKED LIMIT 1), claimed AS (UPDATE support_ticket_notifications n SET available_at=NOW()+INTERVAL '2 minutes',attempts=attempts+1,lease_id=gen_random_uuid() FROM due WHERE n.id=due.id RETURNING n.*) SELECT c.id,c.ticket_id,c.attempts,c.lease_id,c.recipient,t.subject,m.body FROM claimed c JOIN support_tickets t ON t.id=c.ticket_id JOIN support_ticket_messages m ON m.id=c.message_id`).Scan(&notice.ID, &notice.TicketID, &notice.Attempt, &notice.LeaseID, &notice.Recipient, &notice.Subject, &notice.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &notice, nil
}

// Complete records an outcome only while the caller's lease is still current.
// Successful delivery sets its timestamp; failure preserves the intent and a
// nonsensitive diagnostic while delaying another claim for one minute.
func (s *Store) Complete(ctx context.Context, notice support.Notice, delivered bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE support_ticket_notifications SET delivered_at=CASE WHEN $3 THEN NOW() ELSE NULL END,last_error=CASE WHEN $3 THEN '' ELSE 'Email delivery failed; check SMTP settings' END,available_at=NOW()+INTERVAL '1 minute' WHERE id=$1 AND lease_id=$2 AND delivered_at IS NULL`, notice.ID, notice.LeaseID, delivered)
	return err
}

// DeliveryState reports pending, exhausted, and delivered notification totals.
func (s *Store) DeliveryState(ctx context.Context) (support.DeliveryState, error) {
	var state support.DeliveryState
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE delivered_at IS NULL AND attempts<10),count(*) FILTER(WHERE delivered_at IS NULL AND attempts>=10),count(*) FILTER(WHERE delivered_at IS NOT NULL) FROM support_ticket_notifications`).Scan(&state.Pending, &state.Failed, &state.Delivered)
	return state, err
}

// RetryFailed reschedules exhausted notices without repeating successful deliveries.
// Administrators invoke this after repairing mail settings; notification row
// identities and original recipients remain unchanged for later worker claims.
func (s *Store) RetryFailed(ctx context.Context) error {
	// Reset the lease identity with the attempt budget so delayed workers cannot
	// acknowledge manually rescheduled work before or after another worker claims it.
	_, err := s.db.ExecContext(ctx, `UPDATE support_ticket_notifications SET attempts=0,lease_id=gen_random_uuid(),available_at=NOW(),last_error='' WHERE delivered_at IS NULL AND attempts>=10`)
	return err
}
