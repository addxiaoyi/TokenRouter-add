package support

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
)

// delivery.go sends committed support notifications from a durable outbox.
// Ticket writes never depend on SMTP availability. A worker claims one recipient
// at a time and records success only after delivery returns; failures keep the
// intent available for bounded retries without rolling back the conversation.
//
// Leases protect concurrent instances, while unique claim identities prevent a
// late completion from acknowledging a newer lease. SMTP acceptance followed by a
// failed acknowledgement can repeat a notification, so delivery is at least once.
// The application owns worker cancellation and waits for its final operation
// before closing the shared database and mail transport.

// Notice contains one leased recipient and its immutable message contents.
type Notice struct {
	ID        int64  // notification intent identifier
	TicketID  int64  // owning conversation identifier
	Attempt   int    // claimed delivery attempt number
	LeaseID   string // unique claim identity
	Recipient string // address captured when the message committed
	Subject   string // ticket title used in email subject
	Body      string // plain text reply content
}

// DeliveryState summarizes queued and exhausted notices without exposing recipients.
type DeliveryState struct {
	Pending   int `json:"pending"`   // notices still eligible for retry
	Failed    int `json:"failed"`    // notices awaiting manual retry
	Delivered int `json:"delivered"` // acknowledged recipient deliveries
}

// Outbox supplies leased delivery work and attempt-bound acknowledgements.
type Outbox interface {
	// Claim returns one leased notice or nil when none is currently due.
	Claim(context.Context) (*Notice, error)
	// Complete acknowledges only the matching lease attempt, retaining failures for retry.
	Complete(context.Context, Notice, bool) error
	// DeliveryState reads aggregate notification status for the support configuration page.
	DeliveryState(context.Context) (DeliveryState, error)
	// RetryFailed resets exhausted attempts without changing already delivered notices.
	RetryFailed(context.Context) error
}

// Sender accepts escaped HTML content using the application's configured mail transport.
type Sender interface {
	SendEmail(context.Context, string, string, string) error
}

// Deliver polls committed notification intents until its owning lifecycle cancels.
// Each message has a twenty-second delivery budget and a separate acknowledgement
// budget. Database failures pause polling, SMTP failures retain their retryable
// intent, and logs identify ticket and notification IDs without message bodies.
func Deliver(ctx context.Context, outbox Outbox, sender Sender) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		for range 10 {
			if ctx.Err() != nil {
				return
			}
			notice, err := outbox.Claim(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "claim support email failed", "error", err)
				break
			}
			if notice == nil {
				break
			}
			messageCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			body := "<h2>" + html.EscapeString(notice.Subject) + "</h2><p>" + strings.ReplaceAll(html.EscapeString(notice.Body), "\n", "<br>") + "</p>"
			err = sender.SendEmail(messageCtx, notice.Recipient, fmt.Sprintf("Support ticket #%d: %s", notice.TicketID, notice.Subject), body)
			cancel()
			if err != nil {
				slog.WarnContext(ctx, "support email delivery failed", "ticket_id", notice.TicketID, "notice_id", notice.ID, "error", err)
			}
			// A cancellation after SMTP acceptance still needs a bounded acknowledgement
			// attempt, otherwise every shutdown would deliberately leave accepted mail pending.
			ackCtx, ackCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			ackErr := outbox.Complete(ackCtx, *notice, err == nil)
			ackCancel()
			if ackErr != nil {
				slog.ErrorContext(ctx, "acknowledge support email failed", "notice_id", notice.ID, "error", ackErr)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
