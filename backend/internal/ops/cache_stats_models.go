package ops

import "time"

// cache_stats_models.go defines filters and response values for cache-hit
// statistics over saved usage rows.
// Cache statistics summarize how much of the input side of completed requests
// was served from an upstream prompt cache. The overall block covers every
// matching request in the window, while group rows repeat the same totals per
// recorded group so administrators can compare cache behavior across groups.

// OpsCacheStatsFilter selects cache-hit statistics for one usage window.
type OpsCacheStatsFilter struct {
	TimeRange string // response time-range display label
	// StartTime and EndTime select a half-open usage window independently of the
	// TimeRange display label. The service requires nonzero, ordered times and
	// accepts an equal start and end.
	StartTime time.Time // inclusive usage observation start
	EndTime   time.Time // exclusive usage observation end

	Platform string // recorded execution platform filter
	// GroupID selects a positive recorded group identity. GetCacheStats rejects
	// a supplied nonpositive value before querying; nil leaves the group
	// dimension unrestricted.
	GroupID *int64
}

// OpsCacheStatsOverall carries request and input-side token totals for every
// usage row matching a cache-statistics filter.
type OpsCacheStatsOverall struct {
	RequestCount int64 `json:"request_count"` // matching usage observation count
	// InputTokens counts tokens billed as fresh input, CacheCreationTokens
	// counts tokens written into an upstream cache, and CacheReadTokens counts
	// tokens served from that cache. TotalInputTokens sums the three counters,
	// so it describes the complete input side of the matching requests.
	InputTokens         int64 `json:"input_tokens"`          // fresh input token total
	CacheCreationTokens int64 `json:"cache_creation_tokens"` // cache creation token total
	CacheReadTokens     int64 `json:"cache_read_tokens"`     // cache read token total
	TotalInputTokens    int64 `json:"total_input_tokens"`    // summed input-side token total
	// HitRate is the cache-read share of TotalInputTokens. Nil means the window
	// contains no input-side tokens, so no rate was observed.
	HitRate *float64 `json:"hit_rate"` // cache-read share of total input tokens
}

// OpsCacheStatsGroupItem carries request and input-side token totals for one
// recorded group, using the same counter meanings as OpsCacheStatsOverall.
type OpsCacheStatsGroupItem struct {
	GroupID   int64  `json:"group_id"`   // recorded group identifier
	GroupName string `json:"group_name"` // current group display name

	RequestCount        int64    `json:"request_count"`         // group usage observation count
	InputTokens         int64    `json:"input_tokens"`          // fresh input token total
	CacheCreationTokens int64    `json:"cache_creation_tokens"` // cache creation token total
	CacheReadTokens     int64    `json:"cache_read_tokens"`     // cache read token total
	TotalInputTokens    int64    `json:"total_input_tokens"`    // summed input-side token total
	HitRate             *float64 `json:"hit_rate"`              // cache-read share of total input tokens
}

// OpsCacheStatsResponse carries overall cache totals and the leading group
// breakdown for one cache-statistics window.
type OpsCacheStatsResponse struct {
	TimeRange string    `json:"time_range"` // response time-range display label
	StartTime time.Time `json:"start_time"` // inclusive usage observation start
	EndTime   time.Time `json:"end_time"`   // exclusive usage observation end

	Platform string `json:"platform,omitempty"` // selected recorded execution platform
	GroupID  *int64 `json:"group_id,omitempty"` // optional selected group identity

	Overall OpsCacheStatsOverall      `json:"overall"` // totals across all matching usage rows
	Groups  []*OpsCacheStatsGroupItem `json:"groups"`  // leading groups by total input tokens
}
