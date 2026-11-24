//go:build integration

package app

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/ent/paymentauditlog"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingpostgres "github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymentpostgres "github.com/TokenFlux/TokenRouter/internal/payment/postgres"
	paymenttestkit "github.com/TokenFlux/TokenRouter/internal/payment/testkit"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// payment_starpay_scenario_integration_test.go exercises fulfillment.go,
// refund_prepare.go, refund_workflow.go, and payment_refund.go against migrated
// PostgreSQL. Verified provider observations drive real subscription grants and
// wallet deductions, while a channel fixture checks the committed refund receipt
// before simulating a lost response and later recovery.

// starPayRefundProbe checks durable preparation before reporting an unknown refund.
type starPayRefundProbe struct {
	payment.Provider                             // unused base provider operations
	submit           func(payment.RefundRequest) // persisted preparation assertion callback
	requestID        string                      // submitted refund operation identity
	count            int                         // provider refund submission count
}

// ProviderKey identifies the probe as the independent merchant balance channel.
func (*starPayRefundProbe) ProviderKey() string { return payment.TypeStarPay }

// Refund inspects the committed preparation and simulates an accepted request with a lost reply.
// The workflow must retain its deduction and request ID until a later query confirms repayment.
func (p *starPayRefundProbe) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	p.count++
	p.requestID = req.RequestID
	p.submit(req)
	return &payment.RefundResponse{RefundID: req.RequestID}, payment.ErrRefundOutcomeUnknown
}

// QueryRefund returns the known request's successful full repayment without another submission.
func (p *starPayRefundProbe) QueryRefund(_ context.Context, _ payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	return &payment.RefundResponse{RefundID: p.requestID, Status: payment.ProviderStatusSuccess}, nil
}

// TestStarPayRefundPreparationPostgres verifies that full-refund identity survives response loss.
// Partial requests leave funds unchanged, preparation commits before submission, and recovery deducts the wallet only once through production transaction participants.
func TestStarPayRefundPreparationPostgres(t *testing.T) {
	db := postgrescontainer.New(t)
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := t.Context()
	user, err := client.User.Create().
		SetEmail("starpay-refund@example.invalid").
		SetPasswordHash("fixture").
		SetBalance(20).
		SetTotalRecharged(20).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName("fixture").
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("STARPAY-REFUND").
		SetOutTradeNo("tr_starpay_refund").
		SetPaymentType(payment.TypeStarPay).
		SetPaymentTradeNo("platform_42").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(payment.OrderStatusCompleted).
		SetPaidAt(time.Now()).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("shop.example").
		Save(ctx)
	require.NoError(t, err)
	store := paymentpostgres.NewRefundStore(client, func(tx *ent.Tx) payment.RefundRights {
		return newPaymentRefundRights(tx, nil, billing.DateRuntime{})
	})
	probe := &starPayRefundProbe{submit: func(req payment.RefundRequest) {
		audit, readErr := client.PaymentAuditLog.Query().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PREPARED")).
			Only(ctx)
		require.NoError(t, readErr)
		var receipt payment.RefundReceipt
		require.NoError(t, json.Unmarshal([]byte(audit.Detail), &receipt))
		require.NotEmpty(t, req.RequestID)
		require.Equal(t, receipt.OperationID, req.RequestID)
		buyer, readErr := client.User.Get(ctx, user.ID)
		require.NoError(t, readErr)
		require.Equal(t, 10.0, buyer.Balance)
	}}
	workflow := payment.NewRefundWorkflow(store, payment.RefundRuntime{
		Instance: func(context.Context, *payment.Order) (*payment.ProviderInstance, error) {
			return &payment.ProviderInstance{ProviderKey: payment.TypeStarPay, RefundEnabled: true}, nil
		},
		Provider: func(context.Context, *payment.Order) (payment.Provider, error) { return probe, nil },
		User: func(ctx context.Context, id int64) (*payment.RefundUser, error) {
			buyer, readErr := client.User.Get(ctx, id)
			if readErr != nil {
				return nil, readErr
			}
			return &payment.RefundUser{Balance: buyer.Balance}, nil
		},
	})
	_, _, err = workflow.PrepareRefund(ctx, order.ID, 5, "partial", false, true)
	require.Equal(t, "REFUND_FULL_ONLY", apperror.Reason(err))
	unchanged, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 20.0, unchanged.Balance)
	plan, early, err := workflow.PrepareRefund(ctx, order.ID, 10, "full refund", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	_, err = workflow.ExecuteRefund(ctx, plan)
	require.Equal(t, "REFUND_RECOVERY_REQUIRED", apperror.Reason(err))
	pending, err := store.Order(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusRefunding, pending.Status)
	_, _, err = workflow.PrepareRefund(ctx, order.ID, 10, "retry", false, true)
	require.Equal(t, "REFUND_RECOVERY_REQUIRED", apperror.Reason(err))
	confirmed, err := workflow.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, confirmed.Success)
	buyer, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 10.0, buyer.Balance)
	require.Equal(t, 1, probe.count)
	finished, err := store.Order(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusRefunded, finished.Status)
}

// TestStarPayFulfillmentPostgres delivers repeated verified observations for one subscription.
// A cent mismatch cannot change the pending order, and duplicate valid delivery creates exactly one source-order entitlement in PostgreSQL.
func TestStarPayFulfillmentPostgres(t *testing.T) {
	db := postgrescontainer.New(t)
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := t.Context()
	user, err := client.User.Create().
		SetEmail("starpay-grant@example.invalid").
		SetPasswordHash("fixture").
		Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().
		SetName("StarPAY fixture").
		SetPrice(10).
		SetValidityDays(30).
		Save(ctx)
	require.NoError(t, err)
	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStarPay).
		SetName("StarPAY").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeStarPay).
		Save(ctx)
	require.NoError(t, err)
	instanceID := strconv.FormatInt(instance.ID, 10)
	snapshot := payment.BuildPaymentOrderProviderSnapshot(&payment.InstanceSelection{InstanceID: instanceID, ProviderKey: payment.TypeStarPay, Config: map[string]string{"pid": "1001"}}, payment.CreateOrderRequest{})
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName("fixture").
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("STARPAY-GRANT").
		SetOutTradeNo("tr_starpay_grant").
		SetPaymentType(payment.TypeStarPay).
		SetPaymentTradeNo("platform_42").
		SetProviderKey(payment.TypeStarPay).
		SetProviderInstanceID(instanceID).
		SetProviderSnapshot(snapshot).
		SetOrderType(payment.OrderTypeSubscription).
		SetPlanID(plan.ID).
		SetPlanSnapshot(billing.SubscriptionPlanSnapshot{Currency: "CNY", ValidityDays: 30}).
		SetStatus(payment.OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("shop.example").
		Save(ctx)
	require.NoError(t, err)
	subscriptions := billing.NewSubscriptionService(nil, billingpostgres.NewUserSubscriptionRepository(client), billingpostgres.NewSubscriptionMutations(client))
	fulfillment := paymenttestkit.Fulfillment(client, nil, subscriptions, nil)
	notice := &payment.PaymentNotification{
		TradeNo: "platform_42", OrderID: order.OutTradeNo, Amount: 10, Status: payment.NotificationStatusPaid,
		Metadata: map[string]string{"pid": "1001", "currency": "CNY", "merchant_order_id": order.OutTradeNo, "amount_cents": "1001"},
	}
	require.Error(t, fulfillment.HandlePaymentNotification(ctx, notice, payment.TypeStarPay))
	pending, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusPending, pending.Status)
	notice.Metadata["amount_cents"] = "1000"
	for range 3 {
		require.NoError(t, fulfillment.HandlePaymentNotification(ctx, notice, payment.TypeStarPay))
	}
	count, err := client.UserSubscription.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	finished, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderStatusCompleted, finished.Status)
}
