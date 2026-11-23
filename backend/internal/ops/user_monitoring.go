package ops

import (
	"context"
	"time"
)

// user_monitoring.go defines aggregate service observations safe for user dashboards.
// Group authorization is supplied by the application before querying storage.
// Responses contain counts, latency summaries, and model names without provider
// identities, request bodies, user identifiers, or internal error messages.
// Display metadata includes public descriptions and visibility-controlled price
// multipliers. The window declares its UTC bucket resolution so every consumer
// can show missing samples without duplicating points into smaller intervals.

// MonitoringPoint contains one observed success and failure time bucket.
type MonitoringPoint struct {
	Time    time.Time `json:"time"`    // UTC bucket boundary
	Success int64     `json:"success"` // successful user requests
	Errors  int64     `json:"errors"`  // final failed user requests
}

// MonitoringModel describes observed throughput and latency for one pricing model.
type MonitoringModel struct {
	Model         string   `json:"model"`           // recorded public model name
	Requests      int64    `json:"requests"`        // completed successful requests
	AvgDurationMS *float64 `json:"avg_duration_ms"` // absent when latency was not observed
	AvgTTFTMS     *float64 `json:"avg_ttft_ms"`     // absent when first-token timing was not observed
	AvgTPS        *float64 `json:"avg_tps"`         // mean eligible stream output tokens per second
}

// MonitoringGroup combines one accessible group's observations and display metadata.
type MonitoringGroup struct {
	ID             int64    `json:"id"`              // visible group identifier
	Name           string   `json:"name"`            // visible group display name
	Description    string   `json:"description"`     // publicly displayed group description
	RateMultiplier *float64 `json:"rate_multiplier"` // nil when the group hides its price multiplier
	Success        int64    `json:"success"`         // successful requests in selected window
	Errors         int64    `json:"errors"`          // failed requests in selected window
	SuccessRate    *float64 `json:"success_rate"`    // percentage; nil means no observations
	AvgDurationMS  *float64 `json:"avg_duration_ms"` // mean measured request duration
	AvgTTFTMS      *float64 `json:"avg_ttft_ms"`     // mean measured first-token delay
	AvgTPS         *float64 `json:"avg_tps"`         // mean eligible stream output tokens per second
	ActiveModels   int64    `json:"active_models"`   // distinct models with successful requests
	// CacheHitRate uses summed input-side tokens in the selected observation window.
	// Nil means no input tokens; measured zero remains an observed cache miss rate.
	CacheHitRate        *float64          `json:"cache_hit_rate"`
	InputTokens         int64             `json:"input_tokens"`          // ordinary input token count
	CacheCreationTokens int64             `json:"cache_creation_tokens"` // cached input write count
	CacheReadTokens     int64             `json:"cache_read_tokens"`     // cached input read count
	Trend               []MonitoringPoint `json:"trend"`                 // observed time buckets
	Models              []MonitoringModel `json:"models"`                // most-used model observations
}

// UserMonitoring records a bounded observation window and authorized group summaries.
type UserMonitoring struct {
	Enabled       bool              `json:"enabled"`        // monitoring collection and queries are enabled
	Start         time.Time         `json:"start"`          // inclusive observation start
	End           time.Time         `json:"end"`            // exclusive observation end and query time
	BucketMinutes int               `json:"bucket_minutes"` // UTC-aligned trend bucket duration
	Groups        []MonitoringGroup `json:"groups"`         // only currently authorized groups
}

// UserMonitoringReader queries aggregates for an already authorized group set.
type UserMonitoringReader interface {
	// ReadUserMonitoring returns only observations within the supplied groups and range.
	// Callers validate access before invocation; empty groups produce empty output.
	ReadUserMonitoring(context.Context, []MonitoringGroup, time.Time, time.Time) ([]MonitoringGroup, error)
}

// UserMonitoringBucketMinutes selects trend resolution for a bounded observation window.
// The one-hour view uses five minutes, daily views use thirty minutes, and
// longer supported windows use four hours. Readers and clients share this value
// so absent intervals remain gaps instead of repeated or interpolated observations.
func UserMonitoringBucketMinutes(start, end time.Time) int {
	window := end.Sub(start)
	if window <= time.Hour {
		return 5
	}
	if window <= 24*time.Hour {
		return 30
	}
	return 240
}
