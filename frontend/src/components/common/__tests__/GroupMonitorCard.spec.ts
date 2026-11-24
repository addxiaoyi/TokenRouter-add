import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupMonitorCard from '../GroupMonitorCard.vue'
import type { MonitoringGroup } from '@/api/monitoring'

// GroupMonitorCard.spec.ts protects measured cache rates and missing-hour presentation.
// Fixed observation endpoints make the current empty hour distinguishable from
// the last successful request, while zero cache shares remain measured misses.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

// observed creates one successful earlier hour with an observed zero cache share.
function observed(): MonitoringGroup {
  return {
    id: 1,
    name: 'Group',
    success: 1,
    errors: 0,
    success_rate: 100,
    avg_duration_ms: 1000,
    avg_ttft_ms: 200,
    cache_hit_rate: 0,
    input_tokens: 10,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    models: [],
    trend: [{ time: '2026-11-23T10:00:00Z', success: 1, errors: 0 }],
  }
}

describe('monitored group cards', () => {
  // A group last received traffic before the current observation hour ended.
  // The newest bar stays empty and a zero cache share remains visible as zero percent.
  it('anchors empty recent buckets to the observation endpoint', () => {
    const view = mount(GroupMonitorCard, {
      props: { name: 'Group', stats: observed(), windowEnd: '2026-11-23T12:30:00Z' },
    })
    const bars = view.findAll('.group-monitor-bars span')
    expect(bars).toHaveLength(24)
    expect(bars.at(-1)!.attributes('data-state')).toBe('empty')
    expect(bars.at(-3)!.attributes('data-state')).toBe('healthy')
    expect(view.text()).toContain('0.00%')
    expect(view.text()).toContain('1.00s')
  })

  // Group metadata arrives without any measured requests or input-token totals.
  // Its cache and availability labels stay unavailable instead of inventing healthy data.
  it('keeps absent observations distinct from healthy samples', () => {
    const view = mount(GroupMonitorCard, { props: { name: 'Idle group' } })
    expect(view.text()).toContain('monitoring.noCacheSamples')
    expect(view.get('.group-monitor-status').attributes('data-status')).toBe('idle')
    expect(view.findAll('[data-state="healthy"]')).toHaveLength(0)
  })

  // Selectors keep one proportional availability bar without exposing a number or
  // the detailed metrics shown on the full monitoring card.
  it('renders only a proportional compact availability bar', () => {
    const view = mount(GroupMonitorCard, {
      props: { name: 'Compact group', stats: observed(), compact: true },
    })
    expect(view.classes()).toContain('group-monitor-card-compact')
    expect(view.find('.group-monitor-timings').exists()).toBe(false)
    expect(view.find('.group-monitor-footer').exists()).toBe(false)
    expect(view.find('.group-monitor-bars').exists()).toBe(false)
    expect(view.get('.group-monitor-availability-fill').attributes('style')).toContain(
      'width: 100%',
    )
    expect(view.text()).not.toContain('%')
    expect(view.text()).not.toContain('1.00s')
  })

  // Narrow or anomalous percentages stay within the responsive track's 0–100 range.
  it('clamps compact bar width to the percentage range', () => {
    const view = mount(GroupMonitorCard, {
      props: {
        name: 'Partial group',
        stats: { ...observed(), success: 1, errors: 1, success_rate: 50 },
        compact: true,
      },
    })
    expect(view.get('.group-monitor-availability-fill').attributes('style')).toContain('width: 50%')
    expect(view.text()).not.toContain('50%')
  })

  // Missing samples keep the track empty so absence cannot appear as zero-percent uptime.
  it('leaves the compact availability track empty without samples', () => {
    const view = mount(GroupMonitorCard, { props: { name: 'Idle group', compact: true } })
    expect(view.find('.group-monitor-availability-fill').exists()).toBe(false)
    expect(view.get('.group-monitor-availability-bar').attributes('aria-label')).toContain(
      'monitoring.noSamples',
    )
  })
})
