package usage

import (
	"context"
	"errors"
	"time"
)

// profit.go defines retained-usage margin reports for administrators. Customer
// revenue is the recorded charge; confirmed upstream costs use the rate snapshot
// saved with each request. Missing cost evidence remains visible and cannot
// inflate confirmed profit. Internal model calls contribute separate overhead.

// ProfitAmounts separates recorded revenue, covered margins, and missing evidence.
type ProfitAmounts struct {
	Requests              int64   `json:"requests"`                // customer request count
	ConfirmedRequests     int64   `json:"confirmed_requests"`      // customer requests with confirmed cost
	MissingRequests       int64   `json:"missing_requests"`        // customer requests with unknown cost
	SystemRequests        int64   `json:"system_requests"`         // internal model request count
	MissingSystemRequests int64   `json:"missing_system_requests"` // internal requests with unknown cost
	Revenue               float64 `json:"revenue"`                 // USD recorded customer charges
	CoveredRevenue        float64 `json:"covered_revenue"`         // USD charges with confirmed cost
	UpstreamCost          float64 `json:"upstream_cost"`           // USD confirmed customer upstream cost
	GrossProfit           float64 `json:"gross_profit"`            // USD covered revenue minus upstream cost
	SystemCost            float64 `json:"system_cost"`             // USD confirmed internal upstream cost
}

// ProfitProvider combines historical amounts with current configuration reminders.
type ProfitProvider struct {
	ProfitAmounts
	ID         int64   `json:"id"`         // upstream provider identifier
	Name       string  `json:"name"`       // current or deleted provider label
	Site       string  `json:"site"`       // sanitized upstream website origin
	KeyHint    string  `json:"key_hint"`   // masked API key suffix
	HasKey     bool    `json:"has_key"`    // API key credential present
	Rate       float64 `json:"rate"`       // current provider cost multiplier
	Configured bool    `json:"configured"` // current rate confirmation state
	Deleted    bool    `json:"deleted"`    // provider unavailable for configuration
}

// ProfitDay aggregates the selected calendar date without changing request snapshots.
type ProfitDay struct {
	ProfitAmounts
	Date string `json:"date"` // selected timezone calendar date
}

// ProfitReport contains retained usage and all current missing-rate reminders.
type ProfitReport struct {
	ProfitAmounts
	Providers        []ProfitProvider `json:"providers"`         // provider costs and configuration
	Days             []ProfitDay      `json:"days"`              // daily margins in ascending order
	MissingProviders int              `json:"missing_providers"` // current providers needing confirmation
}

// ProfitRepository reads immutable usage costs and current provider reminders.
type ProfitRepository interface {
	// GetProfitReport aggregates a validated half-open range in the supplied timezone.
	// The implementation must exclude unknown costs from confirmed margins and must
	// never return raw credentials alongside configuration reminders.
	GetProfitReport(context.Context, time.Time, time.Time, string) (*ProfitReport, error)
}

// GetProfitReport loads a bounded retained-usage margin report without caching.
// Refreshes must immediately reflect saved rate reminders; historical amounts
// continue to use their recorded confidence and multipliers after configuration changes.
func (s *DashboardService) GetProfitReport(ctx context.Context, start, end time.Time, zone string) (*ProfitReport, error) {
	if !start.Before(end) || end.Sub(start) > 93*24*time.Hour {
		return nil, errors.New("profit date range must be positive and at most 93 days")
	}
	repository, ok := s.usageRepo.(ProfitRepository)
	if !ok {
		return nil, errors.New("profit reporting repository unavailable")
	}
	return repository.GetProfitReport(ctx, start, end, zone)
}

// Add accumulates one disjoint report bucket into an enclosing total.
func (a *ProfitAmounts) Add(bucket ProfitAmounts) {
	a.Requests += bucket.Requests
	a.ConfirmedRequests += bucket.ConfirmedRequests
	a.MissingRequests += bucket.MissingRequests
	a.SystemRequests += bucket.SystemRequests
	a.MissingSystemRequests += bucket.MissingSystemRequests
	a.Revenue += bucket.Revenue
	a.CoveredRevenue += bucket.CoveredRevenue
	a.UpstreamCost += bucket.UpstreamCost
	a.GrossProfit += bucket.GrossProfit
	a.SystemCost += bucket.SystemCost
}
