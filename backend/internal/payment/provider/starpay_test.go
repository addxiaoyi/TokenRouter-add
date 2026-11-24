package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// starpay_test.go exercises the supplied balance SDK's wire contract against
// local TLS endpoints. Tests inspect exact signed request bytes and independently
// construct raw callback signatures, covering ambiguous creation, identity binding,
// replay windows, and full-refund recovery without using real merchant funds.

const (
	// Fixture identities are deliberately distinct so platform order references,
	// business numbers, and receiving merchant credentials cannot be interchanged unnoticed.
	starPayTestSecret = "fixture-merchant-key"
	starPayTestOrder  = `{"id":"platform_42","merchantOrderId":"tr_fixture_001","receiverMerchantId":1001,"amountCents":1000,"status":"paid"}`
	starPayTestEvent  = "{\n  \"event\":\"payment.paid\",\"type\":\"balance\",\"orderId\":\"platform_42\",\"merchantOrderId\":\"tr_fixture_001\",\"amount\":\"10.00\"\n}"
)

// TestStarPayCreatePayment inspects the SDK operation, credentials, and exact signed body.
// A lost first response must retry identical parameters and return the platform ID before the cashier URL is exposed.
func TestStarPayCreatePayment(t *testing.T) {
	var attempts atomic.Int32
	var firstBody string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/pay/epay/balance/orders", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "1001", r.Header.Get("x-starpay-pid"))
		require.Equal(t, starPayTestDigest(r.Header.Get("x-starpay-timestamp"), string(body)), r.Header.Get("x-starpay-signature"))
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		require.Equal(t, "starpay.balance.create.v1", payload["operation"])
		require.Equal(t, map[string]any{
			"merchantOrderId": "tr_fixture_001", "amount": "10.00", "subject": "余额 & <membership>",
			"notifyUrl": "https://shop.example/api/v1/payment/webhook/starpay", "returnUrl": "https://shop.example:443/payment/result?order_id=7",
		}, payload["order"])
		if attempts.Add(1) == 1 {
			firstBody = string(body)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		require.Equal(t, firstBody, string(body))
		_, err = io.WriteString(w, `{"ok":true,"data":{"order":`+strings.Replace(starPayTestOrder, `"paid"`, `"pending"`, 1)+`,"checkoutUrl":"https://`+r.Host+`/pay/checkout?token=fixture"}}`)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	adapter := starPayTestClient(t, server)
	created, err := adapter.CreatePayment(t.Context(), payment.CreatePaymentRequest{
		OrderID: "tr_fixture_001", Amount: "10.00", Subject: "余额 & <membership>",
		ReturnURL: "https://shop.example:443/payment/result?order_id=7",
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
	require.Equal(t, "platform_42", created.TradeNo)
	require.Equal(t, "CNY", created.Currency)
	require.Equal(t, server.URL+"/pay/checkout?token=fixture", created.PayURL)
}

// TestStarPayCreatePaymentRetainsUncertainty models HTML errors, unsafe checkout URLs, and corrupt replies.
// None can prove creation failed, and unsafe cashier destinations must never reach the browser.
func TestStarPayCreatePaymentRetainsUncertainty(t *testing.T) {
	for _, body := range []string{
		`<!doctype html><title>404</title>`,
		`{"ok":false,"code":"unavailable"}`,
		`{"ok":true,"data":{"order":` + starPayTestOrder + `,"checkoutUrl":"https://attacker.example/pay"}}`,
		strings.Repeat("x", starPayMaxBody+1),
	} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, body)
			require.NoError(t, err)
		}))
		adapter := starPayTestClient(t, server)
		created, err := adapter.CreatePayment(t.Context(), payment.CreatePaymentRequest{OrderID: "tr_fixture_001", Amount: "10.00", Subject: "Purchase"})
		require.Nil(t, created)
		var appErr *apperror.ApplicationError
		require.ErrorAs(t, err, &appErr)
		require.Equal(t, payment.CodePaymentCreateUnconfirmed, appErr.Reason)
		server.Close()
	}
}

// TestStarPayVerifyNotification checks original JSON text, freshness, and current payment state.
// Forged or stale events and mismatched queried identities must fail before the runtime can grant any entitlement.
func TestStarPayVerifyNotification(t *testing.T) {
	// This fixed digest was checked against the user-supplied Node SDK, keeping
	// the fixture independent of future changes to the Go signing implementation.
	require.Equal(t, "sha256=eead9c2be407b288f4df0a552d280fd0a7275b2da1c7edc251d70ef51c6cfbed", starPayTestDigest("1700000000", starPayTestEvent))
	cases := []struct {
		name   string                  // scenario display label
		body   string                  // original signed callback bytes
		order  string                  // queried platform order JSON
		mutate func(map[string]string) // optional signature header alteration
		valid  bool                    // expected accepted paid observation
	}{
		{name: "valid raw JSON", body: starPayTestEvent, order: starPayTestOrder, valid: true},
		{name: "changed raw bytes", body: starPayTestEvent + " ", order: starPayTestOrder, mutate: func(h map[string]string) {
			h["x-lightpay-signature"] = starPayTestDigest(h["x-lightpay-timestamp"], starPayTestEvent)
		}},
		{name: "wrong key", body: starPayTestEvent, order: starPayTestOrder, mutate: func(h map[string]string) { h["x-lightpay-signature"] = "sha256=" + strings.Repeat("0", 64) }},
		{name: "missing delivery ID", body: starPayTestEvent, order: starPayTestOrder, mutate: func(h map[string]string) { delete(h, "x-idempotency-key") }},
		{name: "expired signature", body: starPayTestEvent, order: starPayTestOrder, mutate: func(h map[string]string) {
			h["x-lightpay-timestamp"] = "1699999699"
			h["x-lightpay-signature"] = starPayTestDigest(h["x-lightpay-timestamp"], starPayTestEvent)
		}},
		{name: "pending order", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "paid", "pending", 1)},
		{name: "refund already completed", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "paid", "refunded", 1)},
		{name: "different receiver", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "1001", "1002", 1)},
		{name: "different platform ID", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "platform_42", "platform_43", 1)},
		{name: "different business number", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "tr_fixture_001", "tr_fixture_002", 1)},
		{name: "one cent mismatch", body: starPayTestEvent, order: strings.Replace(starPayTestOrder, "1000", "1001", 1)},
		{name: "wrong event", body: strings.Replace(starPayTestEvent, "payment.paid", "payment.refunded", 1), order: starPayTestOrder},
		{name: "wrong method", body: strings.Replace(starPayTestEvent, "balance", "alipay", 1), order: starPayTestOrder},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var queries atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				queries.Add(1)
				require.Equal(t, "/pay/epay/balance/query", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"operation":"starpay.balance.query.v1","orderId":"platform_42"}`, string(body))
				require.Equal(t, starPayTestDigest(r.Header.Get("x-starpay-timestamp"), string(body)), r.Header.Get("x-starpay-signature"))
				_, err = io.WriteString(w, `{"ok":true,"data":{"order":`+tc.order+`}}`)
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			adapter := starPayTestClient(t, server)
			headers := map[string]string{
				"x-lightpay-timestamp": "1700000000", "x-lightpay-signature": starPayTestDigest("1700000000", tc.body), "x-idempotency-key": "callback_42",
			}
			if tc.mutate != nil {
				tc.mutate(headers)
			}
			notice, err := adapter.VerifyNotification(t.Context(), tc.body, headers)
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, notice)
				if tc.mutate != nil {
					require.Zero(t, queries.Load())
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, "platform_42", notice.TradeNo)
			require.Equal(t, "tr_fixture_001", notice.OrderID)
			require.Equal(t, "1000", notice.Metadata["amount_cents"])
			require.Equal(t, "callback_42", notice.Metadata["callback_id"])
			require.Equal(t, tc.body, notice.RawData)
		})
	}
}

// TestStarPayRefundRecovery models a committed transfer with an unreadable response.
// The adapter retains the persisted request ID and accepts only a matching full refund during later status recovery.
func TestStarPayRefundRecovery(t *testing.T) {
	var refunded atomic.Bool
	var submissions atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/refund") {
			submissions.Add(1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"operation":"starpay.balance.refund.v1","orderId":"platform_42","requestId":"saved_refund_42"}`, string(body))
			require.Equal(t, starPayTestDigest(r.Header.Get("x-starpay-timestamp"), string(body)), r.Header.Get("x-starpay-signature"))
			refunded.Store(true)
			_, err = io.WriteString(w, `{"ok":true`)
			require.NoError(t, err)
			return
		}
		order := starPayTestOrder
		if refunded.Load() {
			order = strings.Replace(order, "paid", "refunded", 1)
		}
		_, err := io.WriteString(w, `{"ok":true,"data":{"order":`+order+`}}`)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	adapter := starPayTestClient(t, server)
	request := payment.RefundRequest{RequestID: "saved_refund_42", TradeNo: "platform_42", OrderID: "tr_fixture_001", Amount: "5.00"}
	_, err := adapter.Refund(t.Context(), request)
	require.Error(t, err)
	require.Zero(t, submissions.Load())
	request.Amount = "10.00"
	_, err = adapter.QueryRefund(t.Context(), payment.RefundQueryRequest{
		TradeNo: request.TradeNo, OrderID: request.OrderID, Amount: "10.00", Currency: "CNY",
	})
	require.ErrorIs(t, err, payment.ErrRefundOutcomeUnknown)
	response, err := adapter.Refund(t.Context(), request)
	require.ErrorIs(t, err, payment.ErrRefundOutcomeUnknown)
	require.Equal(t, request.RequestID, response.RefundID)
	recovered, err := adapter.QueryRefund(t.Context(), payment.RefundQueryRequest{
		TradeNo: request.TradeNo, OrderID: request.OrderID, RefundID: request.RequestID, Amount: "10.00", Currency: "CNY",
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, recovered.Status)
	response, err = adapter.Refund(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusSuccess, response.Status)
	require.Equal(t, int32(1), submissions.Load())
}

// TestStarPayBoundaries rejects invalid credentials, non-CNY charges, and unsafe destinations.
// Validation runs before submission, and missing platform references never become merchant-number queries.
func TestStarPayBoundaries(t *testing.T) {
	config := map[string]string{"pid": "1001", "pkey": starPayTestSecret, "notifyUrl": "https://shop.example/notify"}
	for field, badValue := range map[string]string{"pid": "-1", "pkey": " ", "notifyUrl": "http://shop.example/notify", "apiBase": "https://user:pass@pay.example", "currency": "USD"} {
		broken := maps.Clone(config)
		broken[field] = badValue
		_, err := NewStarPay("1", broken)
		require.Error(t, err)
	}
	adapter, err := NewStarPay("1", config)
	require.NoError(t, err)
	for _, amount := range []string{"0.00", "-1.00", "1e1", "10.001", "NaN", "10.00 ", "90071992547409.92"} {
		_, err := adapter.CreatePayment(t.Context(), payment.CreatePaymentRequest{OrderID: "tr_fixture_001", Amount: amount, Subject: "Purchase"})
		require.Error(t, err)
	}
	_, err = adapter.CreatePayment(t.Context(), payment.CreatePaymentRequest{OrderID: "tr_fixture_001", Amount: "10.00", Subject: "Purchase", ReturnURL: "https://other.example/result"})
	require.ErrorContains(t, err, "origin")
	_, err = adapter.QueryOrder(t.Context(), "")
	require.ErrorContains(t, err, "missing")
}

// starPayTestClient creates the production adapter with trust for one local TLS fixture.
// Its client retains production timeout and redirect policy, and its fixed clock
// makes callback freshness independent of the host's date and timezone.
func starPayTestClient(t *testing.T, server *httptest.Server) *StarPay {
	t.Helper()
	adapter, err := NewStarPay("1", map[string]string{
		"apiBase": server.URL + "/pay", "pid": "1001", "pkey": starPayTestSecret,
		"notifyUrl": "https://shop.example/api/v1/payment/webhook/starpay",
	})
	require.NoError(t, err)
	adapter.httpClient.Transport = server.Client().Transport
	adapter.clock = func() time.Time { return time.Unix(1700000000, 0) }
	return adapter
}

// starPayTestDigest independently signs timestamp and original JSON using the SDK contract.
func starPayTestDigest(timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(starPayTestSecret))
	_, _ = io.WriteString(mac, timestamp+"\n"+body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
