package postgres

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// profit.go reads request-snapshot margins and current provider cost reminders.
// Financial buckets are grouped once by provider and calendar date, then summed
// into the same response's provider, day, and overall views. Unknown or partial
// costs retain request counts and revenue while staying outside confirmed margin.
// System requests carry separate overhead and never appear as customer revenue.
//
// Configuration joins supply only current names, website origins, and masked key
// suffixes. They do not supply historical prices or rewrite earlier confidence.
// This separation means changing a provider rate updates its reminder immediately
// without changing recorded profit. Raw credentials remain outside this report;
// administrators use the existing protected export operation for explicit access.
// GetProfitReport reads bounded usage buckets and all live provider reminders.
// The range is half open and day labels use zone. SQL errors abort the entire
// response so partial scans cannot masquerade as complete financial statistics.
//
// Historical writers left confidence NULL; their nondefault recorded rate is
// accepted as configuration evidence. Default-one and missing-rate rows remain
// unknown, and incomplete system usage never becomes zero-cost profit. Current
// provider metadata is read separately because it can change without changing the
// immutable financial buckets. Missing deleted providers keep their numeric identity.
func (r *Store) GetProfitReport(ctx context.Context, start, end time.Time, zone string) (*usage.ProfitReport, error) {
	report := &usage.ProfitReport{Providers: []usage.ProfitProvider{}, Days: []usage.ProfitDay{}}
	providers, err := r.profitProviders(ctx)
	if err != nil {
		return nil, err
	}
	days := map[string]*usage.ProfitDay{}
	rows, err := r.sql.QueryContext(ctx, `WITH scoped AS (
	 SELECT provider_id,TO_CHAR(created_at AT TIME ZONE $3,'YYYY-MM-DD') AS day,
	 request_source='user' AS customer,actual_cost,
	 COALESCE(provider_stats_cost,total_cost)*COALESCE(provider_rate_multiplier,1) AS upstream,
	 COALESCE(provider_cost_confirmed,provider_rate_multiplier<>1,false)
	 AND provider_rate_multiplier>=0 AND provider_rate_multiplier::text<>'NaN'
	 AND (COALESCE(provider_stats_cost,total_cost)>0 OR provider_rate_multiplier=0 OR provider_stats_cost IS NOT NULL)
	 AND COALESCE(system_usage->>'cost_state','calculated')='calculated' AS confirmed
	 FROM usage_logs WHERE created_at >= $1 AND created_at < $2
	)
	SELECT provider_id,day,
	 COUNT(*) FILTER(WHERE customer),COUNT(*) FILTER(WHERE customer AND confirmed),
	 COUNT(*) FILTER(WHERE customer AND NOT confirmed),COUNT(*) FILTER(WHERE NOT customer),
	 COUNT(*) FILTER(WHERE NOT customer AND NOT confirmed),
	 COALESCE(SUM(actual_cost) FILTER(WHERE customer),0),
	 COALESCE(SUM(actual_cost) FILTER(WHERE customer AND confirmed),0),
	 COALESCE(SUM(upstream) FILTER(WHERE customer AND confirmed),0),
	 COALESCE(SUM(upstream) FILTER(WHERE NOT customer AND confirmed),0)
	FROM scoped GROUP BY provider_id,day`, start, end, zone)
	if err != nil {
		return nil, fmt.Errorf("query profit snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var day string
		var amounts usage.ProfitAmounts
		if err := rows.Scan(&id, &day, &amounts.Requests, &amounts.ConfirmedRequests,
			&amounts.MissingRequests, &amounts.SystemRequests, &amounts.MissingSystemRequests,
			&amounts.Revenue, &amounts.CoveredRevenue, &amounts.UpstreamCost, &amounts.SystemCost); err != nil {
			return nil, fmt.Errorf("scan profit snapshots: %w", err)
		}
		amounts.GrossProfit = amounts.CoveredRevenue - amounts.UpstreamCost
		if providers[id] == nil {
			providers[id] = &usage.ProfitProvider{ID: id, Name: fmt.Sprintf("#%d", id), Deleted: true}
		}
		if days[day] == nil {
			days[day] = &usage.ProfitDay{Date: day}
		}
		// Buckets are disjoint provider/day pairs. Each view receives each bucket
		// once, avoiding double counting between customer and internal usage summaries.
		providers[id].Add(amounts)
		days[day].Add(amounts)
		report.Add(amounts)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profit snapshots: %w", err)
	}
	for _, row := range providers {
		if !row.Deleted && !row.Configured {
			report.MissingProviders++
		}
		report.Providers = append(report.Providers, *row)
	}
	for _, day := range days {
		report.Days = append(report.Days, *day)
	}
	sort.Slice(report.Providers, func(i, j int) bool {
		a, b := report.Providers[i], report.Providers[j]
		missingA, missingB := !a.Configured && !a.Deleted, !b.Configured && !b.Deleted
		if missingA != missingB {
			return missingA
		}
		if a.Revenue != b.Revenue {
			return a.Revenue > b.Revenue
		}
		return a.ID < b.ID
	})
	sort.Slice(report.Days, func(i, j int) bool { return report.Days[i].Date < report.Days[j].Date })
	return report, nil
}

// profitProviders reads nondeleted providers without exporting authentication values.
// SQL returns only a masked suffix for API keys. Website URLs are reduced to
// HTTP(S) origins so embedded usernames, paths, query secrets, and fragments cannot leak.
func (r *Store) profitProviders(ctx context.Context) (map[int64]*usage.ProfitProvider, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id,name,COALESCE(rate_multiplier,1),
	 COALESCE(rate_multiplier>=0 AND rate_multiplier::text<>'NaN' AND (rate_multiplier<>1 OR extra->'profit_cost_rate'=to_jsonb(rate_multiplier)),false),
	 COALESCE(credentials->>'base_url',''),
	 CASE WHEN length(COALESCE(credentials->>'api_key',''))>4 THEN '••••'||right(credentials->>'api_key',4) ELSE '' END,
	 length(COALESCE(credentials->>'api_key',''))>0
	 FROM providers WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query profit provider reminders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	providers := map[int64]*usage.ProfitProvider{}
	for rows.Next() {
		row := &usage.ProfitProvider{}
		var rawURL string
		if err := rows.Scan(&row.ID, &row.Name, &row.Rate, &row.Configured, &rawURL, &row.KeyHint, &row.HasKey); err != nil {
			return nil, fmt.Errorf("scan profit provider reminder: %w", err)
		}
		row.Site = profitSite(rawURL)
		providers[row.ID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profit provider reminders: %w", err)
	}
	return providers, nil
}

// profitSite returns a credential-free HTTP(S) origin for an upstream website.
func profitSite(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}
