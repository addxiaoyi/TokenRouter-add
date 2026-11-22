package ops

import (
	"context"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// cache_stats.go validates cache-statistics requests and computes cache hit
// rates from summed usage counters.
// Cache statistics read the same saved usage rows as model token statistics.
// The service validates the requested window and filters before storage groups
// those rows, and the hit-rate helper converts summed counters into a rate
// only when the slice contains input-side tokens.

// CacheHitRate returns the cache-read share of the input side of a request
// slice, computed as cache-read tokens divided by the sum of fresh input,
// cache-creation, and cache-read tokens. The result is nil when that sum is
// zero, because a slice without input-side tokens has no observable rate.
// Callers use the nil result to present an absent rate instead of a misleading
// zero percent.
func CacheHitRate(cacheReadTokens, inputTokens, cacheCreationTokens int64) *float64 {
	totalInputTokens := inputTokens + cacheCreationTokens + cacheReadTokens
	if totalInputTokens <= 0 {
		return nil
	}
	rate := float64(cacheReadTokens) / float64(totalInputTokens)
	return &rate
}

// GetCacheStats returns overall and per-group cache totals after monitoring,
// repository, and ordered nonzero-time validation. A supplied group identity
// must be positive, and an equal start and end select an empty window whose
// hit rates remain unobserved.
func (s *OpsService) GetCacheStats(ctx context.Context, filter *OpsCacheStatsFilter) (*OpsCacheStatsResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
	}
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
	}
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
	}
	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
	}

	return s.opsRepo.GetCacheStats(ctx, filter)
}
