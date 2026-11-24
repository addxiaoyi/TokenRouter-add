package payment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// refund_prepare_test.go verifies StarPAY's full-refund boundary before persistent
// preparation. An order-only store exposes no mutation implementation, so partial
// or unidentified payments must fail during planning while complete requests retain
// their full gateway charge and local entitlement quantity.

// starPayRefundOrderStore supplies a recorded order for pure eligibility checks.
type starPayRefundOrderStore struct {
	RefundStore        // unimplemented mutation operations
	order       *Order // saved eligibility order fixture
}

// Order returns a copy so refund planning cannot mutate the shared fixture.
func (s starPayRefundOrderStore) Order(context.Context, int64) (*Order, error) {
	return s.order.Clone(), nil
}

// TestPrepareRefundStarPayFullOnly checks both purchase kinds before any rights mutation.
// Partial amounts and missing platform references fail, while a full request retains fees in the gateway amount.
func TestPrepareRefundStarPayFullOnly(t *testing.T) {
	for _, kind := range []string{OrderTypeBalance, OrderTypeSubscription} {
		t.Run(kind, func(t *testing.T) {
			order := &Order{ID: 42, Status: OrderStatusCompleted, OrderType: kind, Amount: 10, PayAmount: 12, PaymentTradeNo: "platform_42"}
			workflow := NewRefundWorkflow(starPayRefundOrderStore{order: order}, RefundRuntime{
				Instance: func(context.Context, *Order) (*ProviderInstance, error) {
					return &ProviderInstance{ProviderKey: TypeStarPay, RefundEnabled: true}, nil
				},
			})
			_, _, err := workflow.PrepareRefund(t.Context(), order.ID, 5, "partial", false, false)
			require.Equal(t, "REFUND_FULL_ONLY", apperror.Reason(err))
			plan, early, err := workflow.PrepareRefund(t.Context(), order.ID, 0, "full", false, false)
			require.NoError(t, err)
			require.Nil(t, early)
			require.Equal(t, 10.0, plan.RefundAmount)
			require.Equal(t, 12.0, plan.GatewayAmount)
			order.PaymentTradeNo = ""
			_, _, err = workflow.PrepareRefund(t.Context(), order.ID, 0, "full", false, false)
			require.Equal(t, "REFUND_TRADE_REFERENCE_MISSING", apperror.Reason(err))
		})
	}
}
