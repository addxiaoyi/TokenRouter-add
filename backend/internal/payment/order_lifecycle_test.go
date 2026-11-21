package payment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// order_lifecycle_test.go exercises buyer verification while another worker
// holds the fulfillment lease. Staged store reads reproduce persisted payment
// confirmation followed by a lost claim, and preserve errors when the final
// observation cannot establish paid progress or storage is unavailable.

// verificationRaceStore supplies staged order reads without granting entitlements.
type verificationRaceStore struct {
	// FulfillmentStore rejects unconfigured persistence operations through its
	// nil embedded value, making an unexpected grant fail the test immediately.
	FulfillmentStore
	initial  Order     // original buyer order observation
	latest   Order     // final persisted order observation
	paidAt   time.Time // competing payment confirmation time
	readErr  error     // final persisted read failure
	claimErr error     // injected lease storage failure
	lookups  int       // merchant order lookup count
	reads    int       // numeric order read count
	claims   int       // fulfillment lease attempt count
}

// verificationRaceBindings permits only the single fixture's registry fallback.
type verificationRaceBindings struct {
	// BindingStore leaves unused instance operations unconfigured, so tests
	// fail if verification tries to construct a different merchant adapter.
	BindingStore
}

// verificationRaceProvider reports confirmed payment without making network requests.
type verificationRaceProvider struct {
	// Provider leaves unused payment operations unconfigured because this
	// fixture owns only identity, supported methods, and the paid query.
	Provider
}

// OrderByTradeNumber returns pending state before a competing worker confirms
// payment. Subsequent notification lookup sees that paid observation, while
// numeric reads model the worker retaining its fulfillment lease.
func (s *verificationRaceStore) OrderByTradeNumber(context.Context, string) (*Order, error) {
	s.lookups++
	order := s.initial
	if s.lookups > 1 {
		order.Status = OrderStatusPaid
		order.PaidAt = &s.paidAt
	}
	return &order, nil
}

// Order keeps the competing lease visible through the claim's reread, then
// returns the final configured observation. Errors apply only to that last
// verification reread, preserving the earlier confirmed payment fixture.
func (s *verificationRaceStore) Order(context.Context, int64) (*Order, error) {
	s.reads++
	if s.reads > 3 {
		return &s.latest, s.readErr
	}
	order := s.initial
	order.Status = OrderStatusRecharging
	order.PaidAt = &s.paidAt
	return &order, nil
}

// ClaimFulfillment records a lost lease claim without changing order state.
func (s *verificationRaceStore) ClaimFulfillment(context.Context, int64, time.Time, time.Time) (int, error) {
	s.claims++
	return 0, s.claimErr
}

// ListInstances leaves the fixture order unbound for legacy registry resolution.
func (verificationRaceBindings) ListInstances(context.Context, InstanceFilter) ([]*ProviderInstance, error) {
	return nil, nil
}

// CountEnabledInstances reports the single registry merchant used by this fixture.
func (verificationRaceBindings) CountEnabledInstances(context.Context, string) (int, error) {
	return 1, nil
}

// ProviderKey identifies the query as an EasyPay merchant observation.
func (verificationRaceProvider) ProviderKey() string { return TypeEasyPay }

// SupportedTypes exposes Alipay for the fixture's shared registry lookup.
func (verificationRaceProvider) SupportedTypes() []PaymentType { return []PaymentType{TypeAlipay} }

// QueryOrder returns paid money matching the original fixture order.
func (verificationRaceProvider) QueryOrder(context.Context, string) (*QueryOrderResponse, error) {
	return &QueryOrderResponse{Status: ProviderStatusPaid, Amount: 10, TradeNo: "gateway-42"}, nil
}

// TestVerifyOrderByOutTradeNoConcurrentFulfillment reproduces a buyer paid query
// losing its fulfillment claim to another worker. It accepts persisted paid
// progress without a second claim and keeps unconfirmed or storage failures visible.
func TestVerifyOrderByOutTradeNoConcurrentFulfillment(t *testing.T) {
	t.Parallel()
	storageErr := errors.New("storage unavailable")
	for _, test := range []struct {
		name       string // concurrent verification case name
		status     string // final persisted order status
		paid       bool   // final confirmation timestamp presence
		readErr    error  // final observation read failure
		claimErr   error  // lease claim storage failure
		wantReason string // expected application error reason
		wantError  error  // expected retained error identity
	}{
		{name: "active grant", status: OrderStatusRecharging, paid: true},
		{name: "completed grant", status: OrderStatusCompleted, paid: true},
		{name: "recorded payment", status: OrderStatusPaid, paid: true},
		{name: "unconfirmed completion", status: OrderStatusCompleted, wantReason: "CONFLICT"},
		{name: "still pending", status: OrderStatusPending, paid: true, wantReason: "CONFLICT"},
		{name: "reread unavailable", status: OrderStatusCompleted, paid: true, readErr: storageErr, wantError: storageErr},
		{name: "claim unavailable", claimErr: storageErr, wantError: storageErr},
		{name: "other conflict", claimErr: infraerrors.Conflict("OTHER_CONFLICT", "different constraint"), wantReason: "OTHER_CONFLICT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			now := time.Now()
			initial := Order{ID: 42, UserID: 9, OrderType: OrderTypeBalance, PaymentType: TypeAlipay,
				Status: OrderStatusPending, PayAmount: 10, OutTradeNo: "tr_20261119scanrace", PaymentTradeNo: "gateway-42"}
			latest := initial
			latest.Status = test.status
			if test.paid {
				latest.PaidAt = &now
			}
			store := &verificationRaceStore{initial: initial, latest: latest, paidAt: now, readErr: test.readErr, claimErr: test.claimErr}
			registry := NewRegistry()
			registry.Register(verificationRaceProvider{})
			bindings := NewProviderBindings(verificationRaceBindings{}, registry, nil, BindingRuntime{}, true)
			fulfillment := NewFulfillment(store, bindings, registry, nil, nil, FulfillmentRuntime{})
			service := NewOrderLifecycle(fulfillment, nil, nil)
			order, err := service.VerifyOrderByOutTradeNo(t.Context(), initial.OutTradeNo, initial.UserID)
			switch {
			case test.wantError != nil:
				require.ErrorIs(t, err, test.wantError)
				require.Nil(t, order)
			case test.wantReason != "":
				require.Equal(t, test.wantReason, infraerrors.Reason(err))
				require.Nil(t, order)
			default:
				require.NoError(t, err)
				require.Equal(t, test.status, order.Status)
				require.NotNil(t, order.PaidAt)
			}
			require.Equal(t, 1, store.claims)
		})
	}
}
