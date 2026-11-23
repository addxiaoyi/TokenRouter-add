import { apiClient } from './client'

// monitoring.ts defines user-visible aggregate service observations.
// Nullable rates and timings distinguish missing samples from measured zeros.
// The server derives accessible groups from authentication and never returns
// provider credentials, request bodies, or internal error diagnostics here.

// MonitoringPoint records observed successes and failures in one hourly bucket.
export interface MonitoringPoint {
  time: string // UTC observation bucket boundary.
  success: number // Successful request count.
  errors: number // Final failed request count.
}

// MonitoringModel describes measured latency for a group's most-used models.
export interface MonitoringModel {
  model: string // Public model identifier.
  requests: number // Successful request observations.
  avg_duration_ms: number | null // Mean measured duration, or unavailable.
  avg_ttft_ms: number | null // Mean first-token delay, or unavailable.
}

// MonitoringGroup combines authorized group identity and observed service metrics.
export interface MonitoringGroup {
  id: number // Authorized group identifier.
  name: string // Current group display name.
  success: number // Successful request observations.
  errors: number // Final failed request observations.
  success_rate: number | null // Percentage; null means no samples.
  avg_duration_ms: number | null // Mean measured request duration.
  avg_ttft_ms: number | null // Mean measured first-token delay.
  // Cache rates share the administrator token denominator; null marks windows
  // without any observed input-side tokens and never substitutes an observed zero.
  // Values are fractions from zero to one, converted to percentages by the view.
  cache_hit_rate: number | null
  input_tokens: number // Ordinary input token count.
  cache_creation_tokens: number // Cached input write count.
  cache_read_tokens: number // Cached input read count.
  trend: MonitoringPoint[] // Ordered observed hourly buckets.
  models: MonitoringModel[] // Bounded model-level summary.
}

// MonitoringSnapshot describes a bounded observation range and its availability.
export interface MonitoringSnapshot {
  enabled: boolean // Whether monitoring is enabled.
  start: string // Inclusive observation start.
  end: string // Exclusive observation end and refresh time.
  groups: MonitoringGroup[] // Groups accessible to the authenticated user.
}

// readMonitoring obtains a permission-scoped service snapshot for a supported window.
// Supply 24, 72, or 168 hours. HTTP failures reject the read so the caller can
// retain its previous snapshot and display an explicit refresh warning.
export async function readMonitoring(hours: number): Promise<MonitoringSnapshot> {
  return (await apiClient.get<MonitoringSnapshot>('/monitoring', { params: { hours } })).data
}
