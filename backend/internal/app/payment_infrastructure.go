package app

import (
	"database/sql"
	"log/slog"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymenthttp "github.com/TokenFlux/TokenRouter/internal/payment/httpapi"
	paymentpostgres "github.com/TokenFlux/TokenRouter/internal/payment/postgres"
	paymentadapter "github.com/TokenFlux/TokenRouter/internal/payment/provider"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	settingscore "github.com/TokenFlux/TokenRouter/internal/settings"
)

// payment_infrastructure.go constructs payment configuration, provider
// selection, encryption keys, and HTTP handlers. Encrypted payment
// configuration uses a configured persistent TOTP key, so an automatically
// generated key does not become a payment encryption identity. Payment handlers
// share the registry, instance selector, configuration, and plans assembled for
// checkout and administration.

// providePaymentHTTP binds buyer payment and optional digital invoice workflows.
// Invalid enabled invoice configuration fails assembly before HTTP is served.
// The lifecycle manager owns recovery worker startup and shutdown with shared storage.
func providePaymentHTTP(runtime *payment.Runtime, cfg *payment.ConfigService, plans *billing.Plans, db *sql.DB, manager *lifecycle.Manager) (*paymenthttp.PaymentHandler, error) {
	invoices, err := provideTaxInvoices(db, manager)
	if err != nil {
		return nil, err
	}
	return paymenthttp.NewPaymentHandler(runtime, cfg, plans, invoices), nil
}

// providePaymentAdminHTTP binds payment administration with the shared
// application calendar.
func providePaymentAdminHTTP(runtime *payment.Runtime, cfg *payment.ConfigService, plans *billing.Plans, calendar timezone.Calendar) *paymenthttp.AdminHandler {
	return paymenthttp.NewAdminHandler(runtime, cfg, plans, calendar)
}

// providePaymentWebhookHTTP binds the shared runtime for callback verification
// and payment fulfillment.
func providePaymentWebhookHTTP(runtime *payment.Runtime) *paymenthttp.PaymentWebhookHandler {
	return paymenthttp.NewPaymentWebhookHandler(runtime)
}

// providePaymentEncryptionKey returns the configured persistent TOTP key for
// payment encryption. Missing cfg or an unconfigured generated key yields nil,
// validation errors reach startup, and configuration warnings are logged from
// ConfiguredEncryptionKey.
func providePaymentEncryptionKey(cfg *config.Config) (payment.EncryptionKey, error) {
	if cfg == nil {
		slog.Warn("payment encryption key not configured — encrypted payment config and resume signing will be unavailable")
		return nil, nil
	}
	key, warning, err := payment.ConfiguredEncryptionKey(cfg.Security.SecretEncryptionKey, cfg.Security.SecretEncryptionKeyConfigured)
	if warning != "" {
		slog.Warn(warning)
	}
	return key, err
}

// providePaymentRegistry constructs the provider registry populated by payment
// runtime bindings.
func providePaymentRegistry() *payment.Registry {
	return payment.NewRegistry()
}

// providePaymentLoadBalancer binds stored payment instances and the configured
// encryption key.
func providePaymentLoadBalancer(store *paymentpostgres.InstanceStore, key payment.EncryptionKey) *payment.DefaultLoadBalancer {
	return payment.NewDefaultLoadBalancer(store, []byte(key), payment.SelectionRuntime{Observe: paymentSelectionLog})
}

// paymentSelectionLog reports payment selection warnings or uses info for other
// levels.
func paymentSelectionLog(level, message string, attrs ...any) {
	if level == "warn" {
		slog.Warn(message, attrs...)
	} else {
		slog.Info(message, attrs...)
	}
}

// providePaymentConfigCore combines stored instances, encryption, plan
// validation, and environment-backed provider creation.
func providePaymentConfigCore(store *paymentpostgres.InstanceStore, settings settingscore.Repository, key payment.EncryptionKey, plans *billing.Plans) *payment.ConfigService {
	return payment.NewConfigService(store, settings, []byte(key), plans, payment.ConfigurationRuntime{CreateProvider: paymentadapter.CreateProvider, Warn: slog.Warn})
}
