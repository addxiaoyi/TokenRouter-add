import { describe, expect, it } from 'vitest'
import { monitoringStatus, trendSegments } from '../monitoring'
import type { MonitoringGroup } from '@/api/monitoring'

// monitoring.spec.ts protects no-sample semantics and missing-hour chart gaps.
// Fixtures separate measured failures from absent observations and verify that
// a visual line cannot imply continuous availability across unobserved hours.

// group supplies only metrics consumed by status classification in these fixtures.
function group(success: number, errors: number, rate: number | null): MonitoringGroup {
  return {
    id: 1,
    name: 'group',
    success,
    errors,
    success_rate: rate,
    cache_hit_rate: null,
    input_tokens: 0,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    avg_duration_ms: null,
    avg_ttft_ms: null,
    trend: [],
    models: [],
  }
}

describe('service monitoring observations', () => {
  // Empty traffic must remain idle even if a malformed response carries a rate.
  // Measured zero-percent success remains an incident instead of becoming an empty state.
  it('distinguishes empty observations from healthy and failed traffic', () => {
    expect(monitoringStatus(group(0, 0, 100))).toBe('idle')
    expect(monitoringStatus(group(1, 0, 100))).toBe('healthy')
    expect(monitoringStatus(group(96, 4, 96))).toBe('degraded')
    expect(monitoringStatus(group(0, 2, 0))).toBe('incident')
    expect(monitoringStatus(group(1, 1, null))).toBe('idle')
  })
  // Two consecutive samples and a later isolated sample have an unobserved hour.
  // The chart produces separate segments rather than connecting across missing evidence.
  it('breaks the line across hours without samples', () => {
    const segments = trendSegments(
      [
        { time: '2026-11-21T01:00:00Z', success: 1, errors: 0 },
        { time: '2026-11-21T02:00:00Z', success: 0, errors: 1 },
        { time: '2026-11-21T04:00:00Z', success: 1, errors: 0 },
      ],
      '2026-11-21T00:00:00Z',
      '2026-11-21T06:00:00Z',
    )
    expect(segments).toHaveLength(2)
    expect(segments[0].split(' ')).toHaveLength(2)
    expect(segments[1].split(' ')).toHaveLength(1)
    expect(trendSegments([], 'invalid', 'invalid')).toEqual([])
  })
})
