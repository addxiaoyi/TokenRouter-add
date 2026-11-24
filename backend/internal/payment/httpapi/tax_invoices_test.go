package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// tax_invoices_test.go checks authenticated order routing and buyer input bounds.
// A disabled service fixture still resolves ownership, ensuring that visibility
// cannot reveal another account's invoice and malformed submissions never reach
// application persistence or obtain a successful creation response.

// invoiceHTTPStore records reads while providing a single authenticated fixture order.
type invoiceHTTPStore struct {
	reads int // observed owned-order read attempts
}

// Read hides absent and foreign orders behind the same not-found response.
// The successful fixture has no application and no financial approval, allowing
// disabled-state serialization to be checked without a configured invoice provider.
func (s *invoiceHTTPStore) Read(_ context.Context, id, user int64) (*payment.TaxInvoiceOrder, *payment.TaxInvoice, error) {
	s.reads++
	if id != 1 || user != 2 {
		return nil, nil, apperror.NotFound("NOT_FOUND", "Order not found")
	}
	return &payment.TaxInvoiceOrder{Order: &payment.Order{ID: 1, UserID: 2}}, nil, nil
}

// Reserve rejects every fixture write because this test service remains disabled.
func (*invoiceHTTPStore) Reserve(context.Context, *payment.TaxInvoice, func(*payment.TaxInvoiceOrder) error) (*payment.TaxInvoice, error) {
	panic("unexpected reservation")
}

// Claim rejects every fixture recovery because no upstream provider is configured.
func (*invoiceHTTPStore) Claim(context.Context, string, time.Time) (*payment.TaxInvoice, bool, error) {
	panic("unexpected claim")
}

// Save rejects every fixture observation because no request has been admitted.
func (*invoiceHTTPStore) Save(context.Context, *payment.TaxInvoice) error { panic("unexpected save") }

// Due rejects fixture scheduling because disabled services never launch recovery workers.
func (*invoiceHTTPStore) Due(context.Context, payment.TaxInvoiceConfig, time.Time, int) ([]*payment.TaxInvoice, error) {
	panic("unexpected scheduling")
}

// TestTaxInvoiceHTTPRequiresOwnedOrder exercises all buyer-facing digital invoice routes.
// Missing authentication, another user's order and malformed identifiers cannot
// reveal saved details, while the owned disabled view remains explicitly readable.
func TestTaxInvoiceHTTPRequiresOwnedOrder(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path string
		user               int64
		status             int
	}{
		{"anonymous", http.MethodGet, "/orders/1/tax-invoice", 0, 401},
		{"foreign", http.MethodGet, "/orders/1/tax-invoice", 99, 404},
		{"foreign_file", http.MethodGet, "/orders/1/tax-invoice/file", 99, 404},
		{"invalid", http.MethodGet, "/orders/-1/tax-invoice", 2, 400},
		{"disabled", http.MethodGet, "/orders/1/tax-invoice", 2, 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := &invoiceHTTPStore{}
			service, err := payment.NewTaxInvoices(payment.TaxInvoiceConfig{}, store, nil)
			if err != nil {
				t.Fatal(err)
			}
			handler := NewPaymentHandler(nil, nil, nil, service)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if scenario.user != 0 {
					c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: scenario.user})
				}
			})
			router.GET("/orders/:id/tax-invoice", handler.GetTaxInvoice)
			router.GET("/orders/:id/tax-invoice/file", handler.GetTaxInvoiceFile)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(scenario.method, scenario.path, nil))
			if recorder.Code != scenario.status {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			if scenario.name == "disabled" && !strings.Contains(recorder.Body.String(), `"enabled":false`) {
				t.Fatal("disabled capability omitted")
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("buyer response may be cached")
			}
		})
	}
}

// TestTaxInvoiceHTTPRejectsUnexpectedInput protects server-owned invoice values.
// Unknown amount fields, trailing JSON and oversized payloads fail before any
// order read, so clients cannot override financial approval or monetary snapshots.
func TestTaxInvoiceHTTPRejectsUnexpectedInput(t *testing.T) {
	for _, body := range []string{`{"amount_cents":1}`, `{} {}`, `{"title":"` + strings.Repeat("x", 5000) + `"}`} {
		store := &invoiceHTTPStore{}
		service, err := payment.NewTaxInvoices(payment.TaxInvoiceConfig{}, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		handler := NewPaymentHandler(nil, nil, nil, service)
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 2}) })
		router.POST("/orders/:id/tax-invoice", handler.CreateTaxInvoice)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/orders/1/tax-invoice", strings.NewReader(body)))
		if recorder.Code != 400 || store.reads != 0 {
			t.Fatalf("unsafe buyer input reached storage: %d", recorder.Code)
		}
	}
}
