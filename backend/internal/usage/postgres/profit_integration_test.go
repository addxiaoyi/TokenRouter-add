//go:build integration

package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// profit_integration_test.go verifies persisted profit snapshots against PostgreSQL.
// Fixtures cover unknown default rates, confirmed-one and free upstreams, internal
// costs, and subsequent provider edits. Financial totals must remain tied to the
// recorded request rather than the provider's current configuration.
// TestProfitSnapshots records mixed cost evidence and then edits the provider.
// It verifies missing costs cannot inflate profit and system costs remain separate.
func (s *UsageLogRepoSuite) TestProfitSnapshots() {
	user := mustCreateUser(s.T(), s.client, &identity.User{Email: "profit@test.example"})
	key := mustCreateApiKey(s.T(), s.client, &apikey.APIKey{UserID: user.ID, Key: "sk-profit-test", Name: "profit"})
	upstream := mustCreateProvider(s.T(), s.client, &provider.Record{
		Name: "profit upstream", RateMultiplier: new(1.0),
		Credentials: map[string]any{"base_url": "https://user:secret@profit.example/v1?token=secret", "api_key": "fixture-key-1234"},
	})
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		rate      float64 // captured upstream multiplier
		confirmed *bool   // optional cost confidence
		revenue   float64 // captured customer charge
		source    string  // request source category
	}{
		{0.5, nil, 3, usage.SourceUser},
		{1, nil, 3, usage.SourceUser},
		{0.2, new(false), 1, usage.SourceUser},
		{0, new(true), 2, usage.SourceUser},
		{1, new(true), 5, usage.SourceUser},
		{0.5, new(true), 0, usage.SourceProviderTest},
	} {
		row := &usage.UsageLog{
			UserID: user.ID, APIKeyID: key.ID, ProviderID: upstream.ID,
			RequestID: uuid.NewString(), RequestSource: test.source, Model: "profit-model",
			TotalCost: 4, ActualCost: test.revenue, RateMultiplier: 1,
			ProviderRateMultiplier: &test.rate, ProviderCostConfirmed: test.confirmed,
			CreatedAt: start.Add(time.Hour),
		}
		if test.source != usage.SourceUser {
			row.UserID, row.APIKeyID = 0, 0
			row.SystemUsage = &usage.SystemUsage{Outcome: "success", UsageState: "complete", CostState: "calculated"}
		}
		_, err := s.repo.Create(s.ctx, row)
		s.Require().NoError(err)
	}
	report, err := s.repo.GetProfitReport(s.ctx, start, start.AddDate(0, 0, 1), "UTC")
	s.Require().NoError(err)
	s.Equal(int64(5), report.Requests)
	s.Equal(int64(3), report.ConfirmedRequests)
	s.Equal(int64(2), report.MissingRequests)
	s.InDelta(14, report.Revenue, 1e-9)
	s.InDelta(10, report.CoveredRevenue, 1e-9)
	s.InDelta(6, report.UpstreamCost, 1e-9)
	s.InDelta(4, report.GrossProfit, 1e-9)
	s.InDelta(2, report.SystemCost, 1e-9)
	s.Len(report.Days, 1)
	var reminder usage.ProfitProvider
	for _, row := range report.Providers {
		if row.ID == upstream.ID {
			reminder = row
		}
	}
	s.False(reminder.Configured)
	s.Equal("https://profit.example", reminder.Site)
	s.Equal("••••1234", reminder.KeyHint)
	_, err = s.tx.ExecContext(s.ctx, `UPDATE providers SET rate_multiplier=0.25 WHERE id=$1`, upstream.ID)
	s.Require().NoError(err)
	after, err := s.repo.GetProfitReport(s.ctx, start, start.AddDate(0, 0, 1), "UTC")
	s.Require().NoError(err)
	s.Equal(report.ProfitAmounts, after.ProfitAmounts)
}
