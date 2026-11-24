package payment

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// tax_invoices.go admits financially approved orders and recovers their original
// invoice applications. Buyer ownership and payment snapshots are checked before
// storage reserves the application under the payment order lock. That reservation
// supplies a stable request identity before any network operation, allowing the
// same request to survive browser cancellation, process restart and an upstream
// response that was lost after the tax invoice was issued.
//
// Workers acquire a bounded database lease and publish only through that lease.
// Responses must match the saved order, title and exact CNY amount. A transport
// failure preserves uncertainty and schedules another bounded observation; no
// failure path creates a replacement application. The deployment switch stops
// submission and synchronization while leaving owned saved records readable.

// These bounds limit one recovery pass and prevent frequent queries from moving
// unresolved applications ahead of older work indefinitely.
const (
	taxInvoiceBatch = 20
	taxInvoiceDelay = 30 * time.Second
)

// Invoice identifiers and buyer values follow the existing merchant API contract;
// validating them locally prevents persistently unprocessable applications.
var (
	taxInvoiceIdentity = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	taxInvoiceTrade    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	taxInvoiceTaxID    = regexp.MustCompile(`^[A-Z0-9]{15,20}$`)
)

// TaxInvoices owns admission and recovery for one configured invoicing merchant.
type TaxInvoices struct {
	config TaxInvoiceConfig // immutable deployment merchant binding
	store  TaxInvoiceStore  // durable order and application persistence
	// gateway may be nil only in disabled deployments. Entry points check
	// enablement before creation, synchronization or file retrieval can use it.
	gateway TaxInvoiceGateway // authenticated invoice transport boundary
	clock   func() time.Time  // current UTC observation clock
}

// NewTaxInvoices validates enabled deployment identity before accepting work.
// Disabled services need only storage, so historical applications remain readable
// without secrets or a reachable provider; an enabled service requires all bindings.
func NewTaxInvoices(config TaxInvoiceConfig, store TaxInvoiceStore, gateway TaxInvoiceGateway) (*TaxInvoices, error) {
	if store == nil {
		return nil, errors.New("tax invoice storage is required")
	}
	if config.Enabled {
		endpoint, err := url.Parse(config.Endpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || !strings.HasSuffix(endpoint.Path, "/") {
			return nil, errors.New("invalid tax invoice endpoint")
		}
		if gateway == nil || !taxInvoiceIdentity.MatchString(config.MerchantID) || !taxInvoiceIdentity.MatchString(config.EpayPID) || !taxInvoiceIdentity.MatchString(config.ProviderInstanceID) {
			return nil, errors.New("complete tax invoice merchant binding is required")
		}
	}
	return &TaxInvoices{config: config, store: store, gateway: gateway, clock: time.Now}, nil
}

// View returns an owned application's saved state and current admission reason.
// It performs no upstream request, so polling remains bounded by the background
// worker schedule and disabled deployments can still display saved buyer records.
func (s *TaxInvoices) View(ctx context.Context, orderID, userID int64) (*TaxInvoiceView, error) {
	order, invoice, err := s.store.Read(ctx, orderID, userID)
	if err != nil {
		return nil, err
	}
	reason := s.refusal(order)
	return &TaxInvoiceView{Enabled: s.config.Enabled, Eligible: reason == "", Reason: reason, Invoice: invoice}, nil
}

// Apply reserves immutable buyer details and attempts the original invoice request.
// Existing applications accept only the same buyer fields, even after a provider
// failure. Storage rechecks payment eligibility while holding the order lock before
// the first reservation becomes visible to the provider or another requester.
//
// The persisted UNKNOWN record is the recovery boundary. Once it exists, a network
// error returns that unresolved record instead of manufacturing a new request or
// claiming definite failure. Database errors remain explicit so callers know the
// latest observation could not be saved and can reread the durable record.
func (s *TaxInvoices) Apply(ctx context.Context, orderID, userID int64, buyer TaxInvoiceBuyer) (*TaxInvoiceView, error) {
	if err := validateTaxInvoiceBuyer(buyer); err != nil {
		return nil, err
	}
	order, existing, err := s.store.Read(ctx, orderID, userID)
	if err != nil {
		return nil, err
	}
	if !s.config.Enabled {
		return nil, apperror.Conflict("TAX_INVOICE_DISABLED", "Invoice applications are not enabled")
	}
	if existing != nil {
		if existing.BuyerType != buyer.BuyerType || existing.Title != buyer.Title || existing.TaxID != buyer.TaxID || existing.Email != buyer.Email {
			return nil, apperror.Conflict("TAX_INVOICE_BUYER_CONFLICT", "Invoice buyer details cannot be changed after submission")
		}
		if err = s.syncOne(ctx, existing); err != nil {
			return nil, err
		}
		return s.View(ctx, orderID, userID)
	}
	if reason := s.refusal(order); reason != "" {
		return nil, apperror.Conflict("TAX_INVOICE_INELIGIBLE", "This order is not approved for invoicing")
	}
	amount, _ := taxInvoiceCents(order.Order.PayAmount)
	invoice := &TaxInvoice{
		ID: uuid.NewString(), RequestID: uuid.NewString(), OrderID: orderID, UserID: userID, TradeNo: order.Order.PaymentTradeNo, AmountCents: amount,
		BuyerType: buyer.BuyerType, Title: buyer.Title, TaxID: buyer.TaxID, Email: buyer.Email, Endpoint: s.config.Endpoint, MerchantID: s.config.MerchantID, EpayPID: s.config.EpayPID, ProviderInstanceID: s.config.ProviderInstanceID, Status: "UNKNOWN", UpdatedAt: s.clock().UTC(), NextSyncAt: s.clock().UTC(),
	}
	// Reserve repeats admission against the locked order so an intervening refund
	// or eligibility revocation cannot race this initial durable application.
	saved, err := s.store.Reserve(ctx, invoice, func(current *TaxInvoiceOrder) error {
		cents, valid := taxInvoiceCents(current.Order.PayAmount)
		if s.refusal(current) != "" || !valid || cents != amount || current.Order.PaymentTradeNo != invoice.TradeNo {
			return apperror.Conflict("TAX_INVOICE_INELIGIBLE", "Order eligibility changed before invoicing")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err = s.syncOne(ctx, saved); err != nil {
		return nil, err
	}
	return s.View(ctx, orderID, userID)
}

// Sync processes one bounded page of due applications using their original IDs.
// Callers schedule passes and cancel their context during shutdown. Individual
// persistence errors are collected while other due applications can still progress.
func (s *TaxInvoices) Sync(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}
	rows, err := s.store.Due(ctx, s.config, s.clock().UTC(), taxInvoiceBatch)
	if err != nil {
		return err
	}
	var failures []error
	for _, invoice := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = s.syncOne(ctx, invoice); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// File requests a short-lived document only for an owned issued application.
// The upstream revalidates issuance before returning its URL. A disabled or
// changed merchant binding prevents requests using the wrong provider account.
func (s *TaxInvoices) File(ctx context.Context, orderID, userID int64) (string, error) {
	_, invoice, err := s.store.Read(ctx, orderID, userID)
	if err != nil {
		return "", err
	}
	if invoice == nil || invoice.Status != "ISSUED" || invoice.InvoiceNo == "" || invoice.RemoteID == "" {
		return "", apperror.Conflict("TAX_INVOICE_NOT_ISSUED", "The invoice is not available for download")
	}
	if !s.config.Enabled || !s.sameBinding(invoice) {
		return "", apperror.Conflict("TAX_INVOICE_DISABLED", "Invoice downloads are not enabled for this merchant")
	}
	link, err := s.gateway.Files(ctx, invoice.RemoteID)
	if err != nil {
		// Download failures use a fixed public error because upstream diagnostics
		// can contain short-lived document tokens or customer information.
		return "", apperror.ServiceUnavailable("TAX_INVOICE_UNAVAILABLE", "The invoice file is temporarily unavailable")
	}
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || len(link) > 4096 {
		return "", apperror.ServiceUnavailable("TAX_INVOICE_UNAVAILABLE", "The invoice file is temporarily unavailable")
	}
	return link, nil
}

// syncOne claims a due record before contacting the captured merchant binding.
// Creation recovery reuses the exact request when no remote ID was received;
// otherwise it performs only a query. Every response must match the immutable
// buyer, order, amount and application identities before local publication.
//
// A failed or cancelled call keeps the previous status and records a safe error.
// Saving uses a separate bounded context because cancellation may occur after
// upstream issuance. If that save fails, the lease expires and a later worker
// recovers the unchanged request instead of guessing that nothing was issued.
func (s *TaxInvoices) syncOne(ctx context.Context, invoice *TaxInvoice) error {
	if !s.config.Enabled || !s.sameBinding(invoice) || !taxInvoicePending(invoice.Status) {
		return nil
	}
	claimedInvoice, claimed, err := s.store.Claim(ctx, invoice.ID, s.clock().UTC())
	if err != nil || !claimed {
		return err
	}
	invoice = claimedInvoice
	var remote *TaxInvoiceRemote
	// An absent remote ID means the create response was never accepted locally.
	// Replaying its saved request lets upstream idempotency recover that same invoice.
	if invoice.RemoteID == "" {
		remote, err = s.gateway.Create(ctx, invoice)
	} else {
		remote, err = s.gateway.Query(ctx, invoice.RemoteID)
	}
	// Failed transport or mismatched response identity leaves the saved status
	// unchanged, preventing an uncertain request from becoming an apparent definite failure.
	invoice.ErrorCode = "invoice_response_unavailable"
	if err == nil && validTaxInvoiceRemote(invoice, remote) {
		invoice.RemoteID = remote.ID
		invoice.Status = remote.Status
		invoice.InvoiceNo = remote.InvoiceNo
		invoice.ErrorCode = remote.ErrorCode
	}
	invoice.UpdatedAt = s.clock().UTC()
	invoice.NextSyncAt = invoice.UpdatedAt.Add(taxInvoiceDelay)
	// Preserve the provider observation even when the browser disconnects after
	// issuance; the bounded save still yields promptly during application shutdown.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = s.store.Save(saveCtx, invoice); err != nil {
		return fmt.Errorf("save tax invoice observation: %w", err)
	}
	return nil
}

// sameBinding reports whether recovery still targets the captured seller account.
func (s *TaxInvoices) sameBinding(invoice *TaxInvoice) bool {
	return invoice.Endpoint == s.config.Endpoint && invoice.MerchantID == s.config.MerchantID && invoice.EpayPID == s.config.EpayPID && invoice.ProviderInstanceID == s.config.ProviderInstanceID
}

// refusal derives admission from finance approval and verified payment facts.
// Missing snapshot values are rejected rather than assigned compatibility defaults.
// A completed recharge alone is insufficient because its taxable sale timing has
// to be confirmed independently by the trusted approval record.
//
// The method also requires the configured EasyPay instance, PID and CNY currency.
// This prevents a payment processed by another account from being sent to the
// configured seller, even if its visible method is also called WeChat Pay.
func (s *TaxInvoices) refusal(observation *TaxInvoiceOrder) string {
	if !s.config.Enabled {
		return "disabled"
	}
	if observation == nil || observation.Order == nil {
		return "order_unavailable"
	}
	o := observation.Order
	if o.Status != OrderStatusCompleted || o.PaidAt == nil {
		return "payment_unconfirmed"
	}
	if o.RefundAmount != 0 || o.RefundAt != nil || o.RefundRequestedAt != nil {
		return "refund_conflict"
	}
	if !observation.Approved {
		return "financial_approval_required"
	}
	snapshot := PsOrderProviderSnapshot(o)
	if GetBasePaymentType(o.PaymentType) != TypeWxpay || snapshot == nil || snapshot.ProviderKey != TypeEasyPay {
		return "payment_source_unsupported"
	}
	if o.ProviderKey == nil || *o.ProviderKey != TypeEasyPay || o.ProviderInstanceID == nil || *o.ProviderInstanceID != s.config.ProviderInstanceID {
		return "payment_merchant_mismatch"
	}
	if snapshot.ProviderInstanceID != s.config.ProviderInstanceID || snapshot.MerchantID != s.config.EpayPID {
		return "payment_merchant_mismatch"
	}
	if snapshot.Currency != "CNY" {
		return "currency_unsupported"
	}
	if !taxInvoiceTrade.MatchString(o.PaymentTradeNo) {
		return "payment_mapping_missing"
	}
	if _, valid := taxInvoiceCents(o.PayAmount); !valid {
		return "payment_amount_invalid"
	}
	return ""
}

// taxInvoiceCents converts exact two-decimal gateway money into bounded CNY cents.
// Nonfinite, fractional-cent and out-of-range values fail instead of silently
// rounding the customer's credited balance into an invoice amount.
func taxInvoiceCents(amount float64) (int64, bool) {
	// Gateway currency uses one hundred minor units per yuan.
	cents := amount * 100
	rounded := math.Round(cents)
	// Float storage may retain binary representation noise, but never an extra
	// decimal cent; the tolerance only covers arithmetic on two-decimal money.
	// The upper bound also remains exact in the upstream JavaScript number format.
	if math.IsNaN(cents) || math.IsInf(cents, 0) || cents < 1 || cents > 9007199254740991 || math.Abs(cents-rounded) > 0.000001 {
		return 0, false
	}
	return int64(rounded), true
}

// validateTaxInvoiceBuyer rejects incomplete or noncanonical immutable customer details.
// It does not trim supplied values because silently changing them would weaken
// later comparison with the exact saved application and signed upstream request.
func validateTaxInvoiceBuyer(buyer TaxInvoiceBuyer) error {
	invalid := apperror.BadRequest("TAX_INVOICE_BUYER_INVALID", "Enter valid invoice buyer details")
	if !utf8.ValidString(buyer.Title) || strings.TrimSpace(buyer.Title) != buyer.Title || buyer.Title == "" || utf8.RuneCountInString(buyer.Title) > 160 || strings.IndexFunc(buyer.Title, unicode.IsControl) >= 0 {
		return invalid
	}
	if buyer.BuyerType == "ORGANIZATION" {
		if !taxInvoiceTaxID.MatchString(buyer.TaxID) {
			return invalid
		}
	} else if buyer.BuyerType != "INDIVIDUAL" || buyer.TaxID != "" {
		return invalid
	}
	// The upstream encrypts the email in a bounded field; count UTF-8 bytes
	// rather than runes so a locally accepted address fits that storage contract.
	address, err := mail.ParseAddress(buyer.Email)
	if err != nil || address.Address != buyer.Email || len(buyer.Email) > 214 || strings.TrimSpace(buyer.Email) != buyer.Email {
		return invalid
	}
	_, domain, _ := strings.Cut(buyer.Email, "@")
	if !strings.Contains(domain, ".") || strings.HasSuffix(domain, ".") {
		return invalid
	}
	return nil
}

// taxInvoicePending identifies observations requiring bounded recovery of the original request.
func taxInvoicePending(status string) bool {
	return status == "UNKNOWN" || status == "PROCESSING" || status == "REVERSING"
}

// validTaxInvoiceRemote checks correspondence before trusting an upstream observation.
// Issued invoices require a number, and unfinished invoices cannot contain one.
// A saved remote identity must never change during polling or recovery. Request
// ID, trade number, buyer title and exact CNY cents must also match the reserved
// application before any status can advance. Definite failure requires a known
// error code because accepting an unfamiliar failure would release refund
// protection while the original issuance could still be unresolved upstream.
func validTaxInvoiceRemote(saved *TaxInvoice, remote *TaxInvoiceRemote) bool {
	if remote == nil {
		return false
	}
	if _, err := uuid.Parse(remote.ID); err != nil {
		return false
	}
	if saved.RemoteID != "" && saved.RemoteID != remote.ID {
		return false
	}
	if saved.RequestID != remote.RequestID || saved.TradeNo != remote.TradeNo || saved.Title != remote.Title || saved.AmountCents != remote.AmountCents {
		return false
	}
	if remote.ErrorCode != "" && !taxInvoiceIdentity.MatchString(remote.ErrorCode) {
		return false
	}
	switch remote.Status {
	case "UNKNOWN", "PROCESSING":
		return remote.InvoiceNo == ""
	case "FAILED":
		// Only documented pre-submission or verified issuer failures release the
		// refund lock; an unfamiliar failure label remains an uncertain outcome.
		knownFailure := remote.ErrorCode == "invoice_not_submitted" || remote.ErrorCode == "wechat_param_error" || remote.ErrorCode == "wechat_no_auth" || remote.ErrorCode == "wechat_rule_limit" || remote.ErrorCode == "wechat_issue_failed"
		return remote.InvoiceNo == "" && knownFailure
	case "ISSUED":
		return taxInvoiceTrade.MatchString(remote.InvoiceNo)
	case "REVERSING", "REVERSED":
		return remote.InvoiceNo == "" || taxInvoiceTrade.MatchString(remote.InvoiceNo)
	default:
		return false
	}
}
