package support

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/notification"
	"github.com/TokenFlux/TokenRouter/internal/notification/smtp"
	mailtest "github.com/TokenFlux/TokenRouter/internal/notification/testkit"
)

// delivery_test.go exercises the support worker through a real loopback SMTP
// transport. A refused connection remains retryable, and a later accepted
// message completes only after transport acknowledgement. The in-memory outbox
// exposes worker decisions independently of PostgreSQL lease tests.

// deliveryOutbox supplies one notice repeatedly until its delivery is acknowledged.
type deliveryOutbox struct {
	Outbox              // unused administrative method contract
	notice   Notice     // current immutable mail contents
	accepted []bool     // recorded delivery completion outcomes
	after    func(bool) // synchronous completion callback
}

// Claim returns the fixture notice with an incremented delivery attempt.
func (o *deliveryOutbox) Claim(context.Context) (*Notice, error) {
	o.notice.Attempt++
	return &o.notice, nil
}

// Complete records delivery and invokes the synchronous fixture recovery action.
func (o *deliveryOutbox) Complete(ctx context.Context, _ Notice, accepted bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.accepted = append(o.accepted, accepted)
	o.after(accepted)
	return nil
}

// TestDeliverSMTPRecovery sends through a refused loopback port, then a working peer.
// Only the accepted retry completes successfully, and HTML from user content
// remains escaped in the actual MIME message received by the SMTP fixture.
func TestDeliverSMTPRecovery(t *testing.T) {
	peer := mailtest.StartSMTPServer(t)
	settings := mailtest.NewMemorySettings()
	config := peer.Settings()
	config[notification.SettingKeySMTPPort] = "0"
	require.NoError(t, settings.SetMultiple(t.Context(), config))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	outbox := &deliveryOutbox{notice: Notice{ID: 1, TicketID: 2, Recipient: "support@example.com", Subject: "<title>", Body: "<script>danger</script>\nsecond line"}}
	outbox.after = func(accepted bool) {
		if accepted {
			cancel()
			return
		}
		require.Len(t, outbox.accepted, 1)
		require.NoError(t, settings.SetMultiple(t.Context(), peer.Settings()))
	}
	Deliver(ctx, outbox, notification.NewMailer(settings, smtp.New()))
	require.Equal(t, []bool{false, true}, outbox.accepted)
	require.Equal(t, int64(1), peer.MessageCount())
	body := peer.LastMessageBody(t)
	require.Contains(t, body, "&lt;script&gt;danger&lt;/script&gt;<br>second line")
	require.NotContains(t, body, "<script>")
}
