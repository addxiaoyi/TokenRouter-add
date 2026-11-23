import type { MonitoringGroup, MonitoringPoint } from '@/api/monitoring'

// monitoring.ts interprets public service observations without inventing healthy samples.
// Empty windows remain idle. Thresholds describe observed request success in the
// selected range, and chart segments preserve missing hours as gaps rather than
// drawing uninterrupted availability across periods with no evidence. Group
// classification uses request counts and nullable rates from the same snapshot.
//
// Charts map elapsed time to a fixed viewport and measured request success to
// vertical coordinates. The first valid point starts a segment, and a gap larger
// than one hour starts another; this avoids implying availability across hours
// without observations. Invalid timestamps and empty buckets contribute no point,
// while isolated valid samples remain visible through the view's circle markers.

// Thresholds and SVG coordinates govern observed health classification and hourly
// chart positioning; they do not define service guarantees or billable measurements.
const HEALTHY_RATE = 99
const DEGRADED_RATE = 95
const HOUR_MS = 3600000
const CHART_LEFT = 8
const CHART_WIDTH = 584
const CHART_BOTTOM = 112
const CHART_HEIGHT = 96

// MonitoringStatus identifies observed availability or an absence of request samples.
export type MonitoringStatus = 'healthy' | 'degraded' | 'incident' | 'idle'

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

// trendSegments maps hourly success samples to a fixed SVG viewport.
// Missing hours break the polyline so no connection implies observed health.
// Single samples remain visible through circles supplied by the chart consumer.
// Valid timestamps are positioned within the requested half-open window and
// clamped to the viewport. Empty or invalid observations never contribute a point,
// so combining saved buckets cannot draw through a period with no measured
// requests. Callers supply chronologically ordered buckets from one snapshot;
// invalid windows return an empty segment list for the explicit no-sample state.
export function trendSegments(points: MonitoringPoint[], start: string, end: string): string[] {
  const from = new Date(start).getTime()
  const range = new Date(end).getTime() - from
  if (!Number.isFinite(range) || range <= 0) return []
  const segments: string[][] = []
  let previous = Number.NEGATIVE_INFINITY
  for (const point of points) {
    const count = point.success + point.errors
    const time = new Date(point.time).getTime()
    if (!count || !Number.isFinite(time)) continue
    if (time - previous > HOUR_MS) segments.push([])
    // Clamp timestamps to the viewport so edge buckets stay visible when a
    // partially observed hour overlaps the selected window boundary supplied by storage.
    const x = CHART_LEFT + Math.max(0, Math.min(1, (time - from) / range)) * CHART_WIDTH
    const y = CHART_BOTTOM - (point.success / count) * CHART_HEIGHT
    segments[segments.length - 1].push(`${x.toFixed(2)},${y.toFixed(2)}`)
    previous = time
  }
  return segments.map((segment) => segment.join(' '))
}
