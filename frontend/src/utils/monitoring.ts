import type { MonitoringGroup, MonitoringPoint } from '@/api/monitoring'

// monitoring.ts interprets public service observations without inventing healthy samples.
// Empty windows remain idle. Thresholds describe observed request success in the
// selected range, and chart segments preserve missing intervals as gaps rather than
// drawing uninterrupted availability across periods with no evidence. Group
// classification uses request counts and nullable rates from the same snapshot.
//
// Charts map elapsed time to a fixed viewport and measured request success to
// vertical coordinates. The first valid point starts a segment, and a gap larger
// than the declared bucket duration starts another; this preserves intervals
// without observations. Invalid timestamps and empty buckets contribute no point,
// while isolated valid samples remain visible through the view's circle markers.

// Thresholds and SVG coordinates govern observed health classification and interval
// chart positioning; they do not define service guarantees or billable measurements.
const HEALTHY_RATE = 99
const DEGRADED_RATE = 95
const HOUR_MS = 3600000
const MAX_WINDOW_MS = 7 * 24 * HOUR_MS
const MAX_BUCKETS = 2017
const CHART_LEFT = 8
const CHART_WIDTH = 584
const CHART_BOTTOM = 112
const CHART_HEIGHT = 96

// MonitoringStatus identifies observed availability or an absence of request samples.
export type MonitoringStatus = 'healthy' | 'degraded' | 'incident' | 'idle'

// MonitoringBucket retains measured counts beside an aligned timeline position.
export interface MonitoringBucket {
  time: number // UTC interval boundary in milliseconds.
  rate: number | null // Success fraction; null means no requests.
  requests: number // Total observed final requests.
}

// monitoringBuckets fills a bounded timeline without manufacturing request observations.
// Start and end describe one half-open window; a bucket touching either boundary
// appears once, and an exact end boundary excludes the next interval. The server's
// declared resolution determines placement. Missing timestamps remain empty, while
// repeated samples in one bucket combine their request counts before calculating
// success, so a small sample cannot outweigh a busier interval by averaging rates.
export function monitoringBuckets(
  points: MonitoringPoint[],
  start: string,
  end: string,
  minutes = 60,
): MonitoringBucket[] {
  const from = Date.parse(start)
  const until = Date.parse(end)
  const step = minutes * 60000
  // Reject malformed or unbounded ranges before allocating any visual history.
  if (
    !Number.isFinite(from + until + step) ||
    step <= 0 ||
    until <= from ||
    until - from > MAX_WINDOW_MS
  )
    return []
  const first = Math.floor(from / step) * step
  const last = Math.floor((until - 1) / step) * step
  const count = Math.round((last - first) / step) + 1
  if (count > MAX_BUCKETS) return []
  const observations = new Map<number, { success: number; requests: number }>()
  for (const point of points) {
    const time = Date.parse(point.time)
    if (!Number.isFinite(time) || time < first || time >= until) continue
    const boundary = Math.floor(time / step) * step
    const previous = observations.get(boundary) ?? { success: 0, requests: 0 }
    observations.set(boundary, {
      success: previous.success + point.success,
      requests: previous.requests + point.success + point.errors,
    })
  }
  return Array.from({ length: count }, (_, index) => {
    const time = first + index * step
    const sample = observations.get(time)
    return {
      time,
      requests: sample?.requests ?? 0,
      rate: sample?.requests ? sample.success / sample.requests : null,
    }
  })
}

// monitoringStatus classifies observed success without treating missing samples as healthy.
// At least 99 percent is healthy; 95 to 99 is degraded. Lower success is an
// incident, while empty or invalid rates remain idle for honest empty-state display.
export function monitoringStatus(group: MonitoringGroup): MonitoringStatus {
  if (
    (!group.success && !group.errors) ||
    group.success_rate == null ||
    !Number.isFinite(group.success_rate)
  )
    return 'idle'
  if (group.success_rate >= HEALTHY_RATE) return 'healthy'
  return group.success_rate >= DEGRADED_RATE ? 'degraded' : 'incident'
}

// trendSegments maps aligned success samples to a fixed SVG viewport.
// Missing intervals break the polyline so no connection implies observed health.
// Single samples remain visible through circles supplied by the chart consumer.
// Valid timestamps are positioned within the requested half-open window and
// clamped to the viewport. Empty or invalid observations never contribute a point,
// so combining saved buckets cannot draw through a period with no measured
// requests. Callers supply chronologically ordered buckets from one snapshot;
// invalid windows return an empty segment list for the explicit no-sample state.
export function trendSegments(
  points: MonitoringPoint[],
  start: string,
  end: string,
  bucketMinutes = 60,
): string[] {
  const from = new Date(start).getTime()
  const range = new Date(end).getTime() - from
  if (!Number.isFinite(range) || range <= 0) return []
  const segments: string[][] = []
  let previous = Number.NEGATIVE_INFINITY
  for (const point of points) {
    const count = point.success + point.errors
    const time = new Date(point.time).getTime()
    if (!count || !Number.isFinite(time)) continue
    if (time - previous > bucketMinutes * 60000) segments.push([])
    // Clamp timestamps to the viewport so edge buckets stay visible when a
    // partially observed hour overlaps the selected window boundary supplied by storage.
    const x = CHART_LEFT + Math.max(0, Math.min(1, (time - from) / range)) * CHART_WIDTH
    const y = CHART_BOTTOM - (point.success / count) * CHART_HEIGHT
    segments[segments.length - 1].push(`${x.toFixed(2)},${y.toFixed(2)}`)
    previous = time
  }
  return segments.map((segment) => segment.join(' '))
}
