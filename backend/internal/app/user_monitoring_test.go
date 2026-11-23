package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// user_monitoring_test.go verifies the permission boundary before aggregate
// queries reach storage. Mutable account grants model revocation between
// requests, and disabled accounts cannot reuse a previously authorized view.
// Administrator roles receive the same group filtering as ordinary users.

// monitoringUsers exposes mutable account state for permission revocation tests.
type monitoringUsers struct {
	identity.UserRepository               // unused identity repository contract
	user                    identity.User // current authenticated account record
}

// monitoringGroups returns configured group fixtures without accessing external storage.
type monitoringGroups struct {
	routing.GroupRepository                 // unused routing repository contract
	groups                  []routing.Group // active group fixtures
}

// monitoringMetrics records authorized group identifiers passed into aggregate queries.
type monitoringMetrics struct{ ids []int64 } // last authorized query groups

// GetByID reads the current account fixture after any test mutation.
func (s *monitoringUsers) GetByID(context.Context, int64) (*identity.User, error) {
	return &s.user, nil
}

// ListActive returns active group fixtures for the application's access filter.
func (s monitoringGroups) ListActive(context.Context) ([]routing.Group, error) { return s.groups, nil }

// ReadUserMonitoring captures allowed identifiers and returns unchanged empty observations.
func (s *monitoringMetrics) ReadUserMonitoring(_ context.Context, groups []ops.MonitoringGroup, _, _ time.Time) ([]ops.MonitoringGroup, error) {
	s.ids = nil
	for _, group := range groups {
		s.ids = append(s.ids, group.ID)
	}
	return groups, nil
}

// TestUserMonitoringRevokedGrants removes an administrator's exclusive grant between reads.
// The next query excludes that group, excludes groups without providers, and
// rejects a disabled account before requesting any additional metrics.
func TestUserMonitoringRevokedGrants(t *testing.T) {
	users := &monitoringUsers{user: identity.User{ID: 7, Role: "admin", Status: "active", AllowedGroups: []int64{2}}}
	metrics := &monitoringMetrics{}
	service := userMonitoring{access: marketplaceGroupAccess{users: users}, metrics: metrics, ops: &ops.OpsService{}, groups: monitoringGroups{groups: []routing.Group{
		{ID: 1, ProviderCount: 1, ActiveProviderCount: 1, Description: "Public routing information", RateMultiplier: 0.5},
		{ID: 2, IsExclusive: true, ProviderCount: 1, ActiveProviderCount: 1, RateMultiplier: 0.8, RoutingPolicy: routing.GroupRoutingPolicy{FeaturesConfig: map[string]any{"hide_marketplace_rate_multiplier": true}}},
		{ID: 3, IsExclusive: true, ProviderCount: 1, ActiveProviderCount: 1},
		{ID: 4},
	}}}
	view, err := service.Read(t.Context(), 7, 24)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, metrics.ids)
	require.Equal(t, 30, view.BucketMinutes)
	require.Equal(t, "Public routing information", view.Groups[0].Description)
	require.Equal(t, 0.5, *view.Groups[0].RateMultiplier)
	require.Nil(t, view.Groups[1].RateMultiplier)
	users.user.AllowedGroups = nil
	_, err = service.Read(t.Context(), 7, 24)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, metrics.ids)
	metrics.ids = nil
	users.user.Status = "disabled"
	_, err = service.Read(t.Context(), 7, 24)
	require.Error(t, err)
	require.Nil(t, metrics.ids)
}

// TestUserMonitoringRetainsUnavailableGroups reproduces a group losing every
// active provider. Its authorized traffic history remains visible during the
// outage, while an unconfigured group and an unauthorized exclusive group stay hidden.
func TestUserMonitoringRetainsUnavailableGroups(t *testing.T) {
	users := &monitoringUsers{user: identity.User{ID: 7, Role: "user", Status: "active"}}
	metrics := &monitoringMetrics{}
	service := userMonitoring{access: marketplaceGroupAccess{users: users}, metrics: metrics, ops: &ops.OpsService{}, groups: monitoringGroups{groups: []routing.Group{
		{ID: 1, ProviderCount: 2, ActiveProviderCount: 0},
		{ID: 2, IsExclusive: true, ProviderCount: 2, ActiveProviderCount: 0},
		{ID: 3, ProviderCount: 0},
	}}}
	_, err := service.Read(t.Context(), 7, 24)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, metrics.ids)
}
