package payment

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

// starpay_scenario_test.go covers checkout.go, order_snapshot.go, fulfillment.go,
// order_lifecycle.go, and configuration_providers.go as one merchant binding.
// The cases verify that the SDK's platform identity and integer charge survive
// the shared runtime without exposing its credentials or tolerating conflicting money.

// TestStarPayOrderBinding rejects merchant, business number, cent, and platform ID conflicts.
// Missing platform references stay unresolved, while a complete saved binding accepts only its exact verified payment observation.
func TestStarPayOrderBinding(t *testing.T) {
	selection := &InstanceSelection{ProviderKey: TypeStarPay, InstanceID: "12", PaymentMode: "popup", Config: map[string]string{"pid": "1001", "pkey": "secret"}}
	order := &Order{
		ProviderSnapshot: BuildPaymentOrderProviderSnapshot(selection, CreateOrderRequest{}),
		PaymentTradeNo:   "platform_42", OutTradeNo: "tr_fixture_001", PayAmount: 10,
	}
	require.Equal(t, "1001", order.ProviderSnapshot["merchant_id"])
	require.Equal(t, "CNY", order.ProviderSnapshot["currency"])
	require.NotContains(t, order.ProviderSnapshot, "pkey")
	metadata := map[string]string{"pid": "1001", "currency": "CNY", "merchant_order_id": order.OutTradeNo, "amount_cents": "1000"}
	require.NoError(t, ValidateProviderNotificationMetadata(order, TypeStarPay, metadata))
	for key, value := range map[string]string{"pid": "1002", "currency": "USD", "merchant_order_id": "tr_other_001", "amount_cents": "1001"} {
		changed := maps.Clone(metadata)
		changed[key] = value
		require.Error(t, ValidateProviderNotificationMetadata(order, TypeStarPay, changed), key)
	}
	require.Error(t, ValidateProviderNotificationMetadata(order, TypeStarPay, nil))
	require.Error(t, validatePaymentTradeNumber(order, TypeStarPay, "platform_other", metadata))
	require.Equal(t, "platform_42", PaymentOrderQueryReference(order, nil))
	order.PaymentTradeNo = ""
	require.Empty(t, PaymentOrderQueryReference(order, nil))
}

// TestStarPayConfiguration keeps balance credentials private and channel selection explicit.
// Pending payments protect their receiving identity, notification address, and key while the independent method supports ordinary fee settings.
func TestStarPayConfiguration(t *testing.T) {
	service := &ConfigService{}
	masked := service.ConfigDecryptAndMaskConfig(TypeStarPay, `{"pid":"1001","pkey":"secret"}`)
	require.Equal(t, "1001", masked["pid"])
	require.NotContains(t, masked, "pkey")
	for _, key := range []string{"pid", "pkey", "apiBase", "notifyUrl", "currency"} {
		require.True(t, ConfigHasPendingOrderProtectedConfigChange(TypeStarPay, map[string]string{key: "old"}, map[string]string{key: "new"}), key)
	}
	require.NoError(t, ConfigValidateProviderRequest(TypeStarPay, "Merchant balance", TypeStarPay))
	require.Error(t, ConfigValidateProviderRequest(TypeStarPay, "Merchant balance", TypeEasyPay))
	require.True(t, ConfigIsSupportedMethodFeeMethod(TypeStarPay))
	require.Equal(t, "CNY", ProviderConfigCurrency(TypeStarPay, nil))
}
