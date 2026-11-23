package app

import (
	"context"
	"database/sql"
	"time"

	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	opshttp "github.com/TokenFlux/TokenRouter/internal/ops/httpapi"
	opspostgres "github.com/TokenFlux/TokenRouter/internal/ops/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routingpostgres "github.com/TokenFlux/TokenRouter/internal/routing/postgres"
)

// user_monitoring.go assembles user-visible service health with current group grants.
// Public active groups and explicitly granted exclusive groups are the only
// inputs to aggregate queries. Administrator identity does not bypass user-view
// authorization, and disabled monitoring returns an explicit unavailable state.
// Description and multiplier fields expose the same group metadata as the model
// catalog; the multiplier visibility flag applies before observations are read.

// userMonitoring joins permission readers to the bounded Ops aggregate store.
type userMonitoring struct {
	access  marketplaceGroupAccess   // active-account exclusive group reader
	groups  routing.GroupRepository  // current active group configuration
	metrics ops.UserMonitoringReader // permission-scoped traffic aggregates
	ops     *ops.OpsService          // monitoring availability gate
}

// Read resolves permissions before querying a bounded observation window.
// Group grants are read for every request so revoked access cannot survive a
// cached dashboard. Missing monitoring returns an explicit disabled response,
// while inaccessible exclusive groups never enter the storage query inputs.
func (s userMonitoring) Read(ctx context.Context, userID int64, hours int) (ops.UserMonitoring, error) {
	end := time.Now().UTC()
	view := ops.UserMonitoring{Enabled: s.ops.IsMonitoringEnabled(ctx), Start: end.Add(-time.Duration(hours) * time.Hour), End: end, Groups: []ops.MonitoringGroup{}}
	view.BucketMinutes = ops.UserMonitoringBucketMinutes(view.Start, view.End)
	allowed, err := s.access.AllowedGroupIDs(ctx, userID)
	if err != nil {
		return view, err
	}
	if !view.Enabled {
		return view, nil
	}
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return view, err
	}
	grants := map[int64]bool{}
	for _, id := range allowed {
		grants[id] = true
	}
	for _, group := range groups {
		if group.IsExclusive && !grants[group.ID] {
			continue
		}
		// Retain configured groups during provider outages so the monitoring page
		// can still show their failures and historical traffic to authorized users.
		if group.ProviderCount <= 0 {
			continue
		}
		visible := ops.MonitoringGroup{ID: group.ID, Name: group.Name, Description: group.Description}
		hideRate, _ := group.RoutingPolicy.FeaturesConfig["hide_marketplace_rate_multiplier"].(bool)
		if !hideRate {
			rate := group.RateMultiplier
			visible.RateMultiplier = &rate
		}
		view.Groups = append(view.Groups, visible)
	}
	queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	view.Groups, err = s.metrics.ReadUserMonitoring(queryCtx, view.Groups, view.Start, view.End)
	return view, err
}

// provideUserMonitoringHTTP binds current grants and active groups to aggregate reads.
func provideUserMonitoringHTTP(db *sql.DB, users *identitypostgres.UserStore, groups *routingpostgres.GroupStore, service *ops.OpsService) *opshttp.UserMonitoringHandler {
	return opshttp.NewUserMonitoringHandler(userMonitoring{access: marketplaceGroupAccess{users: users}, groups: groups, metrics: opspostgres.NewUserMonitoringStore(db), ops: service})
}
