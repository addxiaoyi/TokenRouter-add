package app

import (
	"context"
	"database/sql"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/notification"
	"github.com/TokenFlux/TokenRouter/internal/support"
	supporthttp "github.com/TokenFlux/TokenRouter/internal/support/httpapi"
	supportpostgres "github.com/TokenFlux/TokenRouter/internal/support/postgres"
)

// support.go assembles support persistence, HTTP access, and email delivery.
// Conversation routes depend on the assembled handler, so their construction
// registers the delivery lifecycle before application startup. The worker uses
// the same database and SMTP configuration as the rest of the application.
//
// Startup creates one cancellable worker and its completion channel. Shutdown
// cancels admission to further delivery claims and waits for the current send
// and bounded acknowledgement before dependent storage closes. Failed SMTP
// operations remain represented by durable outbox records and can resume after
// restart; conversation writes never rely on an in-memory email queue.

// provideSupportHTTP binds support handlers and registers their delivery worker.
// The lifecycle manager calls Start once before Stop. Stop waits for worker
// completion or its shutdown budget, preserving the outbox for later retries
// when the process cannot finish a delivery within that budget.
func provideSupportHTTP(db *sql.DB, mailer *notification.Mailer, manager *lifecycle.Manager) *supporthttp.Handler {
	store := supportpostgres.NewStore(db)
	service := support.NewService(store)
	var cancel context.CancelFunc
	var done chan struct{}
	manager.Register(lifecycle.Hook{Name: "SupportNotifications", StartOrder: 980, StopOrder: shutdownStageProducers, Start: func(context.Context) error {
		workerCtx, stop := context.WithCancel(context.Background())
		cancel = stop
		done = make(chan struct{})
		// Register the completion signal before launching delivery so shutdown can
		// always observe and wait for the worker it is responsible for stopping.
		go func() { defer close(done); support.Deliver(workerCtx, store, mailer) }()
		return nil
	}, Stop: func(ctx context.Context) error {
		if cancel == nil {
			return nil
		}
		cancel()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	return supporthttp.NewHandler(service, store)
}
