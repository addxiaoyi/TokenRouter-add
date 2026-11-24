import { apiClient } from './client'

// modelQuality.ts exposes sanitized model assessment reports and administrator
// execution controls. Public history contains no provider identifiers or raw
// answers; only target discovery is administrator-scoped. Submission UUIDs make
// retries safe when the browser loses a successful queue response.

// QualityProbe records one sanitized fixed-question outcome and its duration.
export interface QualityProbe {
  id: string // Stable question identifier.
  title: string // Public question label.
  category: string // Public assessment category.
  status: 'pass' | 'fail' | 'error' // Grading or operational outcome.
  duration_ms: number // Elapsed question milliseconds.
  error_code?: string // Sanitized operational failure category.
}

// QualityReport describes safe progress and optional complete assessment scores.
export interface QualityReport {
  id: string // Opaque assessment identifier.
  series_id: string // Opaque comparable configuration identifier.
  model: string // Tested public model name.
  label: string // Public administrator supplied label.
  suite_version: string // Fixed suite version.
  checked_at: string // Assessment start timestamp.
  status: 'queued' | 'running' | 'complete' | 'incomplete' // Persisted execution state.
  score: number | null // Complete score percentage.
  previous_score: number | null // Comparable previous score percentage.
  delta: number | null // Change in percentage points.
  baseline_at: string | null // Previous comparable assessment time.
  signal: string // Comparison or incomplete state.
  passed: number // Correct answer count.
  answered: number // Complete answer count.
  total: number // Required question count.
  duration_ms: number // Elapsed assessment milliseconds.
  probes: QualityProbe[] // Ordered sanitized question outcomes.
}

// QualityTarget identifies an administrator-visible provider and its configured models.
export interface QualityTarget {
  id: number // Provider record identifier.
  name: string // Provider display label.
  models: string[] // Eligible configured model names.
}

// listQuality reads bounded public reports without exposing private execution metadata.
export async function listQuality(): Promise<QualityReport[]> {
  return (await apiClient.get<{ reports: QualityReport[] }>('/model-quality')).data.reports
}

// qualityTargets reads eligible choices and recovers the currently active assessment.
export async function qualityTargets(): Promise<{
  targets: QualityTarget[]
  active: QualityReport | null
}> {
  return (await apiClient.get('/admin/model-quality/targets')).data
}

// startQuality queues a scored test with a stable browser submission identity.
export async function startQuality(
  provider_id: number,
  model: string,
  label: string,
  client_id: string,
): Promise<QualityReport> {
  return (
    await apiClient.post('/admin/model-quality/tests', { provider_id, model, label, client_id })
  ).data
}

// readQuality obtains current progress for an administrator's known assessment identifier.
export async function readQuality(id: string): Promise<QualityReport> {
  return (await apiClient.get(`/admin/model-quality/tasks/${encodeURIComponent(id)}`)).data
}
