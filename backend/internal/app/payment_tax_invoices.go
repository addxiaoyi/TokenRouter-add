package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymentpostgres "github.com/TokenFlux/TokenRouter/internal/payment/postgres"
	paymentprovider "github.com/TokenFlux/TokenRouter/internal/payment/provider"
)

// payment_tax_invoices.go assembles optional digital invoicing and its recovery
// worker. The deployment switch defaults to false and credentials belong to a
// main-site invoice API account rather than the EasyPay payment account. Enabling
// the transport does not approve recharge orders: the payment service also requires
// a trusted financial eligibility record for each individual sale.
//
// Startup registers one cancellable worker before serving the corresponding HTTP
// handlers. Each pass processes a bounded page using database leases, making
// duplicate process scheduling safe at the persistence boundary. Shutdown cancels
// new work and waits for the current bounded request and observation save before
// database shutdown. Log messages contain fixed operation descriptions without
// buyer details, credentials or full upstream responses.

// provideTaxInvoices validates deployment settings and registers bounded recovery.
// False or absent enablement retains read-only access to existing records without
// starting a worker. Explicit invalid settings fail initialization before requests.
//
// The caller supplies the application's database and lifecycle manager before
// lifecycle startup begins. Start publishes one cancellation function and one
// completion channel for the worker. Stop cancels new network work and waits for
// that channel, bounded by the manager's shutdown context, so database shutdown
// can follow the same worker's final observation save.
func provideTaxInvoices(db *sql.DB, manager *lifecycle.Manager) (*payment.TaxInvoices, error) {
	enabled := false
	if value := strings.TrimSpace(os.Getenv("TOKENROUTER_TAX_INVOICE_ENABLED")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errors.New("TOKENROUTER_TAX_INVOICE_ENABLED must be a boolean")
		}
		enabled = parsed
	}
	cfg := payment.TaxInvoiceConfig{Enabled: enabled, Endpoint: strings.TrimSpace(os.Getenv("TOKENROUTER_TAX_INVOICE_ENDPOINT")), MerchantID: strings.TrimSpace(os.Getenv("TOKENROUTER_TAX_INVOICE_MERCHANT_ID")), EpayPID: strings.TrimSpace(os.Getenv("TOKENROUTER_TAX_INVOICE_EPAY_PID")), ProviderInstanceID: strings.TrimSpace(os.Getenv("TOKENROUTER_TAX_INVOICE_PROVIDER_INSTANCE_ID"))}
	var gateway payment.TaxInvoiceGateway
	if enabled {
		var err error
		gateway, err = paymentprovider.NewPerPayInvoices(cfg.Endpoint, cfg.MerchantID, os.Getenv("TOKENROUTER_TAX_INVOICE_API_ID"), os.Getenv("TOKENROUTER_TAX_INVOICE_API_SECRET"), nil)
		if err != nil {
			return nil, err
		}
	}
	service, err := payment.NewTaxInvoices(cfg, paymentpostgres.NewTaxInvoiceStore(db), gateway)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return service, nil
	}
	var cancel context.CancelFunc
	var done chan struct{}
	manager.Register(lifecycle.Hook{Name: "TaxInvoiceRecovery", StartOrder: 981, StopOrder: shutdownStageProducers, Start: func(context.Context) error {
		ctx, stop := context.WithCancel(context.Background())
		cancel = stop
		done = make(chan struct{})
		// Publish the completion channel before launching so shutdown always waits
		// for the same worker and its in-flight observation persistence.
		go func() { defer close(done); runTaxInvoiceRecovery(ctx, service) }()
		return nil
	}, Stop: func(ctx context.Context) error {
		if cancel == nil {
			return nil
		}
		// Cancel before waiting so the current network call can return and the
		// worker can finish its bounded observation save before database shutdown proceeds.
		cancel()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	return service, nil
}

// runTaxInvoiceRecovery observes due original applications until shutdown cancellation.
// Pass failures are logged without private details and retain durable application
// state. The next scheduled pass can recover work left by failed network operations.
func runTaxInvoiceRecovery(ctx context.Context, service *payment.TaxInvoices) {
	// The ticker bounds background wakeups independently of each application's next
	// due time; stopping it releases the scheduling resource when shutdown wins.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := service.Sync(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("Tax invoice recovery pass failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
