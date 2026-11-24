package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
)

// webhook_test.go checks StarPAY's public callback boundary independently of
// fulfillment storage. The handler rejects oversized JSON before verification,
// and unsigned business-number extraction remains only a candidate-selection hint
// while malformed callback text yields no usable identity.

// TestStarPayNotifyRejectsOversizedBody submits JSON exceeding the callback body limit.
// The public route must reject it before provider lookup so signature verification never receives a truncated prefix.
func TestStarPayNotifyRejectsOversizedBody(t *testing.T) {
	router := gin.New()
	handler := &PaymentWebhookHandler{}
	router.POST("/notify", handler.StarPayNotify)
	request := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(strings.Repeat(" ", WebhookMaxWebhookBodySize+1)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

// TestWebhookExtractStarPayOrder reads the SDK business number from unsigned JSON.
// Its exact text remains available for lookup, while invalid JSON cannot substitute a form field for a balance callback.
func TestWebhookExtractStarPayOrder(t *testing.T) {
	require.Equal(t, "tr_sdk_001", WebhookExtractOutTradeNo(`{"merchantOrderId":"tr_sdk_001","orderId":"platform_42"}`, payment.TypeStarPay))
	require.Empty(t, WebhookExtractOutTradeNo(`merchantOrderId=tr_sdk_001`, payment.TypeStarPay))
}
