package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/payment"
)

// tax_invoices_test.go validates the signed transport against local TLS servers.
// The tests inspect the exact request body and attempts while simulating merchant
// confusion, malformed pending success, redirects and uncertain server failures.

// TestPerPayInvoiceSignsOriginalRequest protects the merchant-purpose HMAC boundary.
// A local TLS server verifies the exact body signature and observes one original
// application, without a retry or a newly generated request identity.
func TestPerPayInvoiceSignsOriginalRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// The local TLS fixture receives a bounded client-created body; ignoring
		// its read error leaves the signature assertion to reject an incomplete body.
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("synthetic-secret"))
		_, _ = mac.Write([]byte(r.Header.Get("x-lightpay-timestamp") + "\n"))
		_, _ = mac.Write(body)
		if r.URL.Path != "/tax/applications" || r.Header.Get("x-lightpay-signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) || !strings.Contains(string(body), `"requestId":"persistent-request-1"`) || !strings.Contains(string(body), `"operation":"starpay.invoice.create.v1"`) {
			t.Error("signed command changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		if _, err := io.WriteString(w, `{"ok":true,"data":{"merchantId":"main-1","invoice":{"status":"UNKNOWN"}}}`); err != nil {
			t.Error(err)
		}
	}))
	// Close the fixture after the synchronous request completes so its listener
	// and pooled test connections do not survive into another transport scenario.
	defer server.Close()
	client, err := NewPerPayInvoices(server.URL+"/tax/", "main-1", "api-1", "synthetic-secret", server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	invoice, err := client.Create(context.Background(), &payment.TaxInvoice{RequestID: "persistent-request-1", TradeNo: "platform-1", BuyerType: "INDIVIDUAL", Title: "个人", Email: "buyer@example.test"})
	if err != nil || invoice.Status != "UNKNOWN" || calls.Load() != 1 {
		t.Fatalf("request: %v", err)
	}
}

// TestPerPayInvoiceRejectsUnsafeResponses exercises responses that cannot establish outcome.
// No scenario reaches a redirect target or causes an implicit second request,
// including HTTP 503 after potential issuance and mismatched merchant responses.
func TestPerPayInvoiceRejectsUnsafeResponses(t *testing.T) {
	for _, scenario := range []string{"redirect", "merchant", "accepted_issued", "server_error", "html", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("credential redirected") }))
			// Keep the redirect target listening for the entire scenario, then release
			// it after the tested client's request has returned or rejected the redirect.
			defer target.Close()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch scenario {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "merchant":
					if _, err := io.WriteString(w, `{"ok":true,"data":{"merchantId":"other"}}`); err != nil {
						t.Error(err)
					}
				case "accepted_issued":
					w.WriteHeader(202)
					if _, err := io.WriteString(w, `{"ok":true,"data":{"merchantId":"main-1","invoice":{"status":"ISSUED","invoiceNo":"123"}}}`); err != nil {
						t.Error(err)
					}
				case "server_error":
					w.WriteHeader(503)
					if _, err := io.WriteString(w, `{"ok":false,"code":"invoice_unavailable"}`); err != nil {
						t.Error(err)
					}
				case "html":
					w.Header().Set("Content-Type", "text/html")
					if _, err := io.WriteString(w, "login"); err != nil {
						t.Error(err)
					}
				case "oversized":
					if _, err := io.WriteString(w, strings.Repeat(" ", taxInvoiceResponseLimit+1)); err != nil {
						t.Error(err)
					}
				}
			}))
			// Release each scenario's TLS listener and test connections after the
			// synchronous call, including malformed-response cases that exit through validation errors.
			defer server.Close()
			client, err := NewPerPayInvoices(server.URL+"/tax/", "main-1", "api-1", "synthetic-secret", server.Client().Transport)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Create(context.Background(), &payment.TaxInvoice{}); err == nil || calls.Load() != 1 {
				t.Fatal("unsafe response accepted or retried")
			}
		})
	}
}
