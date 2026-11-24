import { apiClient } from '../client'

// profit.ts reads administrator margin records and confirms upstream cost rates.
// Reports expose masked key hints only. Full credential access stays with the
// existing provider export endpoint and its configured step-up verification policy.

// ProfitAmounts keeps customer margin separate from internal model overhead.
export interface ProfitAmounts {
  requests: number // Customer request count.
  confirmed_requests: number // Requests with confirmed cost.
  missing_requests: number // Requests with unknown cost.
  system_requests: number // Internal model request count.
  missing_system_requests: number // Internal requests with unknown cost.
  revenue: number // USD recorded customer charges.
  covered_revenue: number // USD charges with confirmed cost.
  upstream_cost: number // USD confirmed upstream cost.
  gross_profit: number // USD confirmed customer margin.
  system_cost: number // USD confirmed internal overhead.
}

// ProfitProvider connects retained amounts with current upstream configuration reminders.
export interface ProfitProvider extends ProfitAmounts {
  id: number // Provider record identifier.
  name: string // Provider display name.
  site: string // Sanitized upstream website origin.
  key_hint: string // Masked credential suffix.
  has_key: boolean // API key credential exists.
  rate: number // Current upstream multiplier.
  configured: boolean // Current rate confirmation state.
  deleted: boolean // Provider cannot be edited.
}

// ProfitReport carries the selected range totals and date/provider breakdowns.
export interface ProfitReport extends ProfitAmounts {
  providers: ProfitProvider[] // Provider amounts and reminders.
  days: (ProfitAmounts & { date: string })[] // Calendar date buckets.
  missing_providers: number // Providers needing rate confirmation.
}

// readProfit reads one date-only range in the supplied browser timezone.
// The caller owns cancellation and must discard stale responses after changing
// the range or leaving the administrator page.
export async function readProfit(
  start: string,
  end: string,
  zone: string,
  signal?: AbortSignal,
): Promise<ProfitReport> {
  const { data } = await apiClient.get<ProfitReport>('/admin/dashboard/profit', {
    params: { start_date: start, end_date: end, timezone: zone },
    signal,
  })
  return data
}

// confirmCostRate saves a verified provider multiplier for subsequent requests.
// Explicit zero means free upstream usage. This operation leaves historical
// records intact and refreshes the provider through its existing administrative writer.
export async function confirmCostRate(id: number, rate: number): Promise<void> {
  await apiClient.put(`/admin/providers/${id}/cost-rate`, { rate })
}
