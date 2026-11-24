package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// nowpayments_test.go exercises NOWPayments payment creation, status queries,
// and IPN verification against local TLS API fixtures. Creation cases
// distinguish explicit rejections from uncertain acceptance, query cases pin
// the lifecycle mapping, and callback cases require a valid HMAC signature
// over the canonical key-sorted body plus agreement with a queried finished
// payment before any success is reported.

// TestNOWPaymentsConfigValidation rejects incomplete or malformed instance configuration.
// It also confirms the documented defaults for the API base, fiat currency, and settlement asset.
func TestNOWPaymentsConfigValidation(t *testing.T) {
	valid := map[string]string{"apiKey": "api-key", "ipnSecret": "ipn-secret"}
	for name, mutate := range map[string]func(map[string]string){
		"missing API key":    func(c map[string]string) { delete(c, "apiKey") },
		"missing IPN secret": func(c map[string]string) { delete(c, "ipnSecret") },
		"plain HTTP base":    func(c map[string]string) { c["apiBase"] = "http://api.example.com/v1" },
		"base with query":    func(c map[string]string) { c["apiBase"] = "https://api.example.com/v1?x=1" },
		"unsupported fiat":   func(c map[string]string) { c["currency"] = "EUR" },
		"bad asset code":     func(c map[string]string) { c["payCurrency"] = "usdt-trc20" },
		"bad notify URL":     func(c map[string]string) { c["notifyUrl"] = "http://site.example/hook" },
	} {
		t.Run(name, func(t *testing.T) {
			config := maps.Clone(valid)
			mutate(config)
			_, err := NewNOWPayments("fixture", config)
			require.Error(t, err)
		})
	}
	adapter, err := NewNOWPayments("fixture", valid)
	require.NoError(t, err)
	require.Equal(t, "https://api.nowpayments.io/v1", adapter.baseURL)
	require.Equal(t, "usdttrc20", adapter.payCurrency)
	require.Equal(t, payment.TypeNOWPayments, adapter.ProviderKey())
	require.Equal(t, []payment.PaymentType{payment.TypeNOWPayments}, adapter.SupportedTypes())
}

// TestNOWPaymentsCreatePayment sends one creation request and inspects the API call.
// It checks the key header, the quoted fiat and asset fields, and the returned deposit address payload.
func TestNOWPaymentsCreatePayment(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/payment", r.URL.Path)
		require.Equal(t, "api-key", r.Header.Get("x-api-key"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, float64(100), body["price_amount"])
		require.Equal(t, "usd", body["price_currency"])
		require.Equal(t, "usdttrc20", body["pay_currency"])
		require.Equal(t, "tr_fixture_order", body["order_id"])
		require.Equal(t, "https://site.example/api/v1/payment/webhook/nowpayments", body["ipn_callback_url"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"payment_id":123456,"payment_status":"waiting","pay_address":"TXyzExampleAddress","price_amount":100,"price_currency":"usd","pay_amount":100.25,"pay_currency":"usdttrc20","order_id":"tr_fixture_order"}`))
	}))
	defer server.Close()
	adapter := newNOWPaymentsFixture(t, server)
	result, err := adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "tr_fixture_order", Amount: "100.00", Subject: "Balance recharge",
		NotifyURL: "https://site.example/api/v1/payment/webhook/nowpayments",
	})
	require.NoError(t, err)
	require.Equal(t, "123456", result.TradeNo)
	require.Equal(t, "TXyzExampleAddress", result.QRCode)
	require.Equal(t, "USD", result.Currency)
	require.Equal(t, 1, calls)
}

// TestNOWPaymentsCreatePaymentRejected covers explicit client-error rejections from the API.
// A rejection issued before any payment exists returns an ordinary error so checkout can release the order.
func TestNOWPaymentsCreatePaymentRejected(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":false,"statusCode":400,"code":"BAD_REQUEST","message":"amount is below the minimum"}`))
	}))
	defer server.Close()
	adapter := newNOWPaymentsFixture(t, server)
	result, err := adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "tr_fixture_order", Amount: "1.00",
	})
	require.Nil(t, result)
	require.Error(t, err)
	var appErr *apperror.ApplicationError
	require.NotErrorAs(t, err, &appErr)
}

// TestNOWPaymentsCreatePaymentUnconfirmed covers server failures during creation.
// Because the payment may already exist under the order number, the error keeps the local order recoverable.
func TestNOWPaymentsCreatePaymentUnconfirmed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	adapter := newNOWPaymentsFixture(t, server)
	result, err := adapter.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "tr_fixture_order", Amount: "100.00",
	})
	require.Nil(t, result)
	var appErr *apperror.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, payment.CodePaymentCreateUnconfirmed, appErr.Reason)
}

// TestNOWPaymentsQueryOrderStatuses maps each provider lifecycle label to the shared status vocabulary.
// It also requires the queried payment identity to repeat the requested reference before any state is trusted.
func TestNOWPaymentsQueryOrderStatuses(t *testing.T) {
	for status, expected := range map[string]string{
		"finished":       payment.ProviderStatusSuccess,
		"waiting":        payment.ProviderStatusPending,
		"confirming":     payment.ProviderStatusPending,
		"confirmed":      payment.ProviderStatusPending,
		"sending":        payment.ProviderStatusPending,
		"partially_paid": payment.ProviderStatusPending,
		"failed":         payment.ProviderStatusFailed,
		"expired":        payment.ProviderStatusFailed,
		"refunded":       payment.ProviderStatusRefunded,
	} {
		t.Run(status, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/payment/123456", r.URL.Path)
				_, _ = fmt.Fprintf(w, `{"payment_id":123456,"payment_status":%q,"pay_address":"TXyzExampleAddress","price_amount":100,"price_currency":"usd","pay_amount":100.25,"pay_currency":"usdttrc20","order_id":"tr_fixture_order"}`, status)
			}))
			defer server.Close()
			adapter := newNOWPaymentsFixture(t, server)
			result, err := adapter.QueryOrder(context.Background(), "123456")
			require.NoError(t, err)
			require.Equal(t, expected, result.Status)
			require.Equal(t, float64(100), result.Amount)
			require.Equal(t, "123456", result.TradeNo)
		})
	}
}

// TestNOWPaymentsQueryOrderNotFound covers a payment reference the provider does not know.
// The classified not-found error lets connectivity probes distinguish a reachable service from a failed query.
func TestNOWPaymentsQueryOrderNotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	adapter := newNOWPaymentsFixture(t, server)
	_, err := adapter.QueryOrder(context.Background(), "123456")
	require.ErrorIs(t, err, payment.ErrProviderOrderNotFound)
	_, err = adapter.QueryOrder(context.Background(), "not-a-number")
	require.Error(t, err)
	require.NotErrorIs(t, err, payment.ErrProviderOrderNotFound)
}

// TestNOWPaymentsVerifyNotificationFinished accepts a correctly signed finished IPN callback.
// The signature is computed over a literal canonical rendering, and the queried payment must agree before success.
func TestNOWPaymentsVerifyNotificationFinished(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/payment/123456", r.URL.Path)
		_, _ = w.Write([]byte(`{"payment_id":123456,"payment_status":"finished","pay_address":"TXyzExampleAddress","price_amount":100,"price_currency":"usd","pay_amount":100.25,"pay_currency":"usdttrc20","order_id":"tr_fixture_order"}`))
	}))
	defer server.Close()
	adapter := newNOWPaymentsFixture(t, server)
	canonical := `{"actually_paid":100.25,"order_id":"tr_fixture_order","pay_address":"TXyzExampleAddress","pay_amount":100.25,"pay_currency":"usdttrc20","payment_id":123456,"payment_status":"finished","price_amount":100,"price_currency":"usd","purchase_id":987}`
	rawBody := `{"payment_id":123456,"purchase_id":987,"price_amount":100,"payment_status":"finished","pay_address":"TXyzExampleAddress","order_id":"tr_fixture_order","pay_currency":"usdttrc20","price_currency":"usd","pay_amount":100.25,"actually_paid":100.25}`
	notification, err := adapter.VerifyNotification(context.Background(), rawBody, map[string]string{
		"x-nowpayments-sig": nowPaymentsTestSignature(canonical, "ipn-secret"),
	})
	require.NoError(t, err)
	require.Equal(t, "tr_fixture_order", notification.OrderID)
	require.Equal(t, "123456", notification.TradeNo)
	require.Equal(t, float64(100), notification.Amount)
	require.Equal(t, payment.ProviderStatusSuccess, notification.Status)
}

// TestNOWPaymentsVerifyNotificationRejectsBadSignature covers forged and tampered IPN callbacks.
// A digest computed over different content, or a missing header, fails before any field is trusted.
func TestNOWPaymentsVerifyNotificationRejectsBadSignature(t *testing.T) {
	adapter, err := NewNOWPayments("fixture", map[string]string{"apiKey": "api-key", "ipnSecret": "ipn-secret", "currency": "USD"})
	require.NoError(t, err)
	rawBody := `{"payment_id":123456,"payment_status":"finished","order_id":"tr_fixture_order","price_amount":100,"price_currency":"usd","pay_currency":"usdttrc20"}`
	_, err = adapter.VerifyNotification(context.Background(), rawBody, map[string]string{
		"x-nowpayments-sig": nowPaymentsTestSignature(`{"payment_id":999}`, "ipn-secret"),
	})
	require.Error(t, err)
	_, err = adapter.VerifyNotification(context.Background(), rawBody, map[string]string{})
	require.Error(t, err)
}

// TestNOWPaymentsVerifyNotificationPending reports an in-flight transfer without settling it.
// The confirming observation returns pending progress and never reaches the finished-payment query check.
func TestNOWPaymentsVerifyNotificationPending(t *testing.T) {
	adapter, err := NewNOWPayments("fixture", map[string]string{"apiKey": "api-key", "ipnSecret": "ipn-secret", "currency": "USD"})
	require.NoError(t, err)
	canonical := `{"order_id":"tr_fixture_order","pay_currency":"usdttrc20","payment_id":123456,"payment_status":"confirming","price_amount":100,"price_currency":"usd"}`
	rawBody := `{"payment_status":"confirming","price_currency":"usd","payment_id":123456,"price_amount":100,"order_id":"tr_fixture_order","pay_currency":"usdttrc20"}`
	notification, err := adapter.VerifyNotification(context.Background(), rawBody, map[string]string{
		"x-nowpayments-sig": nowPaymentsTestSignature(canonical, "ipn-secret"),
	})
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPending, notification.Status)
	require.Equal(t, "tr_fixture_order", notification.OrderID)
}

// TestNOWPaymentsCanonicalJSON pins the canonical rendering used for IPN signatures.
// Nested objects are sorted recursively, arrays keep their order, and markup characters stay unescaped.
func TestNOWPaymentsCanonicalJSON(t *testing.T) {
	rendered, err := nowPaymentsCanonicalJSON(`{"b":{"z":1,"a":[3,2]},"a":"x<&>y","n":100.50}`)
	require.NoError(t, err)
	require.Equal(t, `{"a":"x<&>y","b":{"a":[3,2],"z":1},"n":100.5}`, string(rendered))
	_, err = nowPaymentsCanonicalJSON(`{"a":1} {"b":2}`)
	require.Error(t, err)
}

// TestNOWPaymentsRefundUnsupported confirms automatic refunds stay unavailable for this adapter.
// Refund attempts fail with an explicit unsupported error instead of contacting the provider.
func TestNOWPaymentsRefundUnsupported(t *testing.T) {
	adapter, err := NewNOWPayments("fixture", map[string]string{"apiKey": "api-key", "ipnSecret": "ipn-secret"})
	require.NoError(t, err)
	_, err = adapter.Refund(context.Background(), payment.RefundRequest{TradeNo: "123456", OrderID: "tr_fixture_order", Amount: "100.00"})
	require.ErrorContains(t, err, "not supported")
}

// newNOWPaymentsFixture constructs an adapter trusting only the supplied local TLS server.
// The production timeout remains attached to the fixture client.
func newNOWPaymentsFixture(t *testing.T, server *httptest.Server) *NOWPayments {
	t.Helper()
	adapter, err := NewNOWPayments("fixture", map[string]string{
		"apiBase": server.URL, "apiKey": "api-key", "ipnSecret": "ipn-secret", "currency": "USD",
	})
	require.NoError(t, err)
	adapter.httpClient.Transport = server.Client().Transport
	return adapter
}

// nowPaymentsTestSignature computes the hex HMAC-SHA512 digest the provider would send.
// Tests pass a literal canonical body so the signing algorithm is pinned independently of the adapter renderer.
func nowPaymentsTestSignature(canonical, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	// The hash.Hash contract guarantees a nil error for every input.
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// TestNOWPaymentsCreatePaymentTrimsConfigValues guards configuration normalization behavior.
// Surrounding whitespace and asset-code case do not change the constructed adapter's effective settings.
func TestNOWPaymentsCreatePaymentTrimsConfigValues(t *testing.T) {
	adapter, err := NewNOWPayments("fixture", map[string]string{
		"apiKey": " api-key ", "ipnSecret": " ipn-secret ", "currency": "usd", "payCurrency": "USDTBSC",
		"apiBase": "https://api-sandbox.nowpayments.io/v1/",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api-sandbox.nowpayments.io/v1", adapter.baseURL)
	require.Equal(t, "USD", adapter.currency)
	require.Equal(t, "usdtbsc", adapter.payCurrency)
	require.True(t, strings.HasPrefix(adapter.Name(), "NOWPayments"))
}
