package ops

import (
	"context"
	"time"

	logger "github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// request_activity_runtime.go publishes aggregate activity samples from an
// optional in-memory tracker and exposes administrator history reads. Assembly
// binds the store before concurrent request observation begins. Monitoring
// preferences control whether sampling is enabled; the observer keeps keyed
// fingerprints in memory while this worker owns database writes and retention.
//
// The worker advances the sampling bucket only after a successful write, so a
// failed sample is retried on a later tick. Each iteration uses a bounded context
// derived from application shutdown, and the caller joins this worker before
// closing repositories. Cancellation disables the tracker and releases its
// ticker; daily pruning removes historical samples outside the retained window.

// Native history is kept for ninety days and persistence work receives its own
// deadline within the cancellable worker loop.
const (
	requestActivityRetention    = 90 * 24 * time.Hour
	requestActivityWriteTimeout = 5 * time.Second
)

// BindRequestActivity supplies an aggregate store before the service starts.
func (s *OpsService) BindRequestActivity(store RequestActivityStore) {
	s.requestActivity = NewRequestActivity(store)
}

// ObserveRequestActivity hands one decoded gateway body to the optional tracker.
// The observer performs no Redis or database calls and retains only its keyed
// fingerprint in memory to refresh a near-duplicate cluster.
func (s *OpsService) ObserveRequestActivity(userID int64, body []byte) {
	if s != nil {
		s.requestActivity.Observe(userID, body)
	}
}

// ReadRequestActivity exposes current and stored counts when monitoring permits.
// The caller must authenticate as administrator and validate supplied page
// bounds. Missing setup returns unavailable instead of an empty history.
func (s *OpsService) ReadRequestActivity(ctx context.Context, userID, beforeID int64) (RequestActivityView, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return RequestActivityView{}, err
	}
	if s.requestActivity == nil {
		return RequestActivityView{}, apperror.New(apperror.CategoryServiceUnavailable, "REQUEST_ACTIVITY_UNAVAILABLE", "Request activity is unavailable")
	}
	return s.requestActivity.Read(ctx, userID, beforeID)
}

// RunRequestActivity refreshes the monitoring gate and stores threshold samples.
// Call it once with an application-owned context. Cancel and join before the
// repository closes. The first sampled bucket starts after the first complete
// window, and capture retries on the next tick if a write fails. Persistent
// rows use a distinct bucket key so restarts cannot duplicate recent samples.
func (s *OpsService) RunRequestActivity(ctx context.Context) {
	if s == nil || s.requestActivity == nil {
		return
	}
	tracker := s.requestActivity
	defer tracker.SetEnabled(false)
	ticker := time.NewTicker(requestActivityRefresh)
	defer ticker.Stop()

	var nextSample time.Time
	var lastPruneDay time.Time
	for ctx.Err() == nil {
		cycle, cancel := context.WithTimeout(ctx, requestActivityWriteTimeout)
		enabled := s.IsMonitoringEnabled(cycle) && s.IsRealtimeMonitoringEnabled(cycle)
		tracker.SetEnabled(enabled)

		now := time.Now().UTC()
		bucket := now.Truncate(RequestActivityWindow)
		switch {
		case !enabled:
			nextSample = time.Time{}
		case nextSample.IsZero():
			nextSample = bucket.Add(RequestActivityWindow)
		case !bucket.Before(nextSample):
			snapshot := tracker.Snapshot(0)
			snapshot.ObservedAt = bucket
			if err := tracker.store.Save(cycle, snapshot); err != nil {
				logger.LegacyPrintf("service.ops", "[Ops] request activity snapshot failed: %v", err)
			} else {
				nextSample = bucket.Add(RequestActivityWindow)
			}
		default:
			// The same retention window applies even when no requests arrive
			// between snapshots, so a timer removes expired ephemeral data.
			tracker.pruneMemory()
		}
		day := now.Truncate(24 * time.Hour)
		if enabled && day.After(lastPruneDay) {
			if err := tracker.store.Prune(cycle, now.Add(-requestActivityRetention)); err != nil {
				logger.LegacyPrintf("service.ops", "[Ops] request activity retention failed: %v", err)
			} else {
				lastPruneDay = day
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pruneMemory removes expired profiles after an idle collection timer tick.
func (a *RequestActivity) pruneMemory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expire(time.Now())
}
