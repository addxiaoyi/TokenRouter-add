package payment

import (
	"context"
	"errors"
	"testing"
	"time"
)

// tax_invoices_test.go exercises durable issuance through an in-memory store.
// The scenarios observe gateway calls and immutable requests, while PostgreSQL
// transaction and refund exclusion behavior belong to the persistence tests.
// These fixtures do not model concurrent workers or real invoice issuance.

// taxInvoiceFixture keeps one owned payment and one durable application.
type taxInvoiceFixture struct {
	// order remains shared so individual scenarios can change admission facts
	// before invoking the service; the fixture is used synchronously by each test.
	order *TaxInvoiceOrder // current financial eligibility observation
	// invoice is nil until reservation and copied on reads and writes, allowing
	// recovery assertions to compare persistent identity without sharing mutable observations.
	invoice *TaxInvoice // retained immutable application record
	calls   int         // observed upstream creation count
	fail    bool        // synthetic uncertain gateway outcome
}

// newTaxInvoiceFixture supplies confirmed CNY payment facts and finance approval.
func newTaxInvoiceFixture() *taxInvoiceFixture {
	now := time.Now()
	key, instance := TypeEasyPay, "3"
	return &taxInvoiceFixture{order: &TaxInvoiceOrder{Approved: true, Order: &Order{ID: 1, UserID: 2, Status: OrderStatusCompleted, PaidAt: &now, PayAmount: 10, PaymentType: "wxpay", PaymentTradeNo: "platform-1", ProviderKey: &key, ProviderInstanceID: &instance, ProviderSnapshot: map[string]any{"provider_key": "easypay", "provider_instance_id": "3", "merchant_id": "1001", "currency": "CNY"}}}}
}

// Read returns shared order facts and a copied application for the fixture owner.
// Unknown order or user identities fail; an owned order without an application
// returns nil so admission can exercise its first reservation.
func (f *taxInvoiceFixture) Read(_ context.Context, id, user int64) (*TaxInvoiceOrder, *TaxInvoice, error) {
	if id != 1 || user != 2 {
		return nil, nil, errors.New("not found")
	}
	if f.invoice == nil {
		return f.order, nil, nil
	}
	invoice := *f.invoice
	return f.order, &invoice, nil
}

// Reserve retains the first fixture application after checking current eligibility.
// Later calls return a copy of that original record. This fixture does not model
// database locking or concurrent conflicting buyer submissions.
func (f *taxInvoiceFixture) Reserve(_ context.Context, invoice *TaxInvoice, check func(*TaxInvoiceOrder) error) (*TaxInvoice, error) {
	if err := check(f.order); err != nil {
		return nil, err
	}
	if f.invoice == nil {
		// Preserve the first reservation across later recovery calls so the test
		// observes stable request identity after a simulated lost provider response.
		saved := *invoice
		f.invoice = &saved
	}
	saved := *f.invoice
	return &saved, nil
}

// Claim assigns a synthetic lease to the fixture's existing application.
// Callers reserve a record first. This helper always grants the claim, leaving
// due-time filtering and concurrent lease exclusion to persistence tests.
func (f *taxInvoiceFixture) Claim(_ context.Context, _ string, now time.Time) (*TaxInvoice, bool, error) {
	invoice := *f.invoice
	// Give the synthetic claim a fixed lifetime without simulating contention.
	invoice.LeaseUntil = now.Add(90 * time.Second)
	return &invoice, true, nil
}

// Save replaces the fixture application with a copied observation from the caller.
// It does not enforce lease identity or expiry; those concurrency rules belong
// to the PostgreSQL persistence tests.
func (f *taxInvoiceFixture) Save(_ context.Context, invoice *TaxInvoice) error {
	saved := *invoice
	f.invoice = &saved
	return nil
}

// Due exposes a copied fixture application or an empty slice when none exists.
// It deliberately ignores scheduling inputs so service tests can trigger recovery
// immediately; persistence tests own due-time and page-limit behavior.
func (f *taxInvoiceFixture) Due(_ context.Context, _ TaxInvoiceConfig, _ time.Time, _ int) ([]*TaxInvoice, error) {
	if f.invoice == nil {
		return nil, nil
	}
	saved := *f.invoice
	return []*TaxInvoice{&saved}, nil
}

// Create counts each gateway invocation and returns a matching issued observation.
// The fail switch instead produces a transport error, letting tests verify that
// the original application remains available for later recovery.
func (f *taxInvoiceFixture) Create(_ context.Context, invoice *TaxInvoice) (*TaxInvoiceRemote, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("provider unavailable")
	}
	return &TaxInvoiceRemote{ID: "00000000-0000-4000-8000-000000000001", RequestID: invoice.RequestID, TradeNo: invoice.TradeNo, Title: invoice.Title, AmountCents: invoice.AmountCents, Status: "ISSUED", InvoiceNo: "12345678901234567890"}, nil
}

// Query returns a matching issued observation from the fixture's saved application.
// Callers must reserve that application first. The helper does not increment the
// creation count, making query recovery distinguishable from another create call.
func (f *taxInvoiceFixture) Query(ctx context.Context, _ string) (*TaxInvoiceRemote, error) {
	return &TaxInvoiceRemote{ID: f.invoice.RemoteID, RequestID: f.invoice.RequestID, TradeNo: f.invoice.TradeNo, Title: f.invoice.Title, AmountCents: f.invoice.AmountCents, Status: "ISSUED", InvoiceNo: "12345678901234567890"}, nil
}

// Files returns a synthetic HTTPS document link without checking buyer ownership.
// The payment service performs ownership and issuance checks before calling this
// fixture, so tests exercise that service boundary rather than the provider.
func (f *taxInvoiceFixture) Files(context.Context, string) (string, error) {
	return "https://example.test/invoice", nil
}

// TestTaxInvoiceRequiresConfirmedFinancialEligibility rejects unsupported payment observations.
// Each refusal leaves both application storage and the upstream creation count untouched.
// Fixtures vary one admission fact at a time.
func TestTaxInvoiceRequiresConfirmedFinancialEligibility(t *testing.T) {
	for _, kind := range []string{"unapproved", "pending", "refund", "currency", "merchant", "source", "amount", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			f := newTaxInvoiceFixture()
			cfg := TaxInvoiceConfig{Enabled: true, Endpoint: "https://star-web.top/pay/epay/tax-invoices/", MerchantID: "main-1", EpayPID: "1001", ProviderInstanceID: "3"}
			switch kind {
			case "unapproved":
				f.order.Approved = false
			case "pending":
				f.order.Order.Status = OrderStatusPending
			case "refund":
				f.order.Order.RefundAmount = 1
			case "currency":
				f.order.Order.ProviderSnapshot["currency"] = "USD"
			case "merchant":
				f.order.Order.ProviderSnapshot["merchant_id"] = "other"
			case "source":
				f.order.Order.ProviderSnapshot["provider_key"] = "stripe"
			case "amount":
				f.order.Order.PayAmount = 1.001
			case "disabled":
				cfg.Enabled = false
			}
			s, err := NewTaxInvoices(cfg, f, f)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Apply(context.Background(), 1, 2, TaxInvoiceBuyer{BuyerType: "INDIVIDUAL", Title: "个人", Email: "buyer@example.test"})
			if err == nil || f.calls != 0 || f.invoice != nil {
				t.Fatalf("unsafe invoice admission: %v", err)
			}
		})
	}
}

// TestTaxInvoiceUnknownRecoversOriginalRequest preserves uncertain application identity.
// Changed buyer input is rejected, and recovery reuses the originally persisted request.
// A foreign user also cannot retrieve the recovered invoice document.
func TestTaxInvoiceUnknownRecoversOriginalRequest(t *testing.T) {
	f := newTaxInvoiceFixture()
	f.fail = true
	s, err := NewTaxInvoices(TaxInvoiceConfig{Enabled: true, Endpoint: "https://star-web.top/pay/epay/tax-invoices/", MerchantID: "main-1", EpayPID: "1001", ProviderInstanceID: "3"}, f, f)
	if err != nil {
		t.Fatal(err)
	}
	buyer := TaxInvoiceBuyer{BuyerType: "INDIVIDUAL", Title: "个人", Email: "buyer@example.test"}
	view, err := s.Apply(context.Background(), 1, 2, buyer)
	if err != nil || view.Invoice.Status != "UNKNOWN" {
		t.Fatalf("uncertain outcome lost: %v", err)
	}
	id := view.Invoice.RequestID
	buyer.Title = "Changed"
	if _, err = s.Apply(context.Background(), 1, 2, buyer); err == nil || f.calls != 1 {
		t.Fatal("changed immutable buyer")
	}
	f.fail = false
	if err = s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.invoice.RequestID != id || f.invoice.Status != "ISSUED" || f.invoice.AmountCents != 1000 {
		t.Fatal("recovery changed payment identity or amount")
	}
	if _, err = s.File(context.Background(), 1, 99); err == nil {
		t.Fatal("foreign buyer downloaded invoice")
	}
}
