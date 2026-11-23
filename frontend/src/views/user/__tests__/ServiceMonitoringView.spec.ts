import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { MonitoringSnapshot } from '@/api/monitoring'
import ServiceMonitoringView from '../ServiceMonitoringView.vue'

// ServiceMonitoringView.spec.ts exercises permission-scoped snapshot replacement
// and recovery after failed refreshes. Mounted views retain their real request
// generation and lifecycle logic while shell components are replaced, keeping
// the assertions independent of navigation and dialog animation.

// API and identity doubles expose delayed responses without contacting real accounts.
const read = vi.hoisted(() => vi.fn())
const auth = reactive({ user: { id: 7 } })
vi.mock('@/api/monitoring', () => ({ readMonitoring: read }))
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

// snapshot builds one idle group without manufacturing observations or timing values.
function snapshot(name: string): MonitoringSnapshot {
  return {
    enabled: true,
    start: '2026-11-20T00:00:00Z',
    end: '2026-11-21T00:00:00Z',
    groups: [
      {
        id: 1,
        name,
        success: 0,
        errors: 0,
        success_rate: null,
        cache_hit_rate: null,
        input_tokens: 0,
        cache_creation_tokens: 0,
        cache_read_tokens: 0,
        avg_duration_ms: null,
        avg_ttft_ms: null,
        trend: [],
        models: [],
      },
    ],
  }
}

// mountMonitoring preserves request state while replacing unrelated presentation components.
function mountMonitoring() {
  return mount(ServiceMonitoringView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: true,
        Select: true,
        Icon: true,
        RouterLink: true,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  auth.user = { id: 7 }
})

describe('service monitoring', () => {
  // Two visible groups carry a measured hit rate and an absent token sample.
  // Each card must show its own rate without turning missing observations into zero.
  it('shows group cache hit rates and preserves no-sample states', async () => {
    const observed = snapshot('Cached group')
    observed.groups[0]!.cache_hit_rate = 0.6
    const idle = snapshot('Idle group').groups[0]!
    idle.id = 2
    idle.cache_hit_rate = null
    observed.groups.push(idle)
    read.mockResolvedValueOnce(observed)
    const view = mountMonitoring()
    await flushPromises()
    expect(view.findAll('article')[0]!.text()).toContain('60.00%')
    expect(view.findAll('article')[1]!.text()).toContain('monitoring.noCacheSamples')
    view.unmount()
  })

  // A failed refresh must retain the last observed snapshot with an explicit warning.
  // A successful retry replaces the snapshot and clears that warning.
  it('retains observations on refresh failure and recovers on retry', async () => {
    read
      .mockResolvedValueOnce(snapshot('Observed group'))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(snapshot('Updated group'))
    const view = mountMonitoring()
    await flushPromises()
    expect(view.text()).toContain('monitoring.noSamples')
    await view
      .findAll('button')
      .find((button) => button.text() === 'monitoring.refresh')!
      .trigger('click')
    await flushPromises()
    expect(view.text()).toContain('Observed group')
    expect(view.get('[role="alert"]').text()).toContain('monitoring.refreshFailed')
    await view
      .findAll('button')
      .find((button) => button.text() === 'monitoring.retry')!
      .trigger('click')
    await flushPromises()
    expect(view.text()).toContain('Updated group')
    expect(view.find('[role="alert"]').exists()).toBe(false)
    view.unmount()
  })

  // An account change occurs while the previous account's request remains in flight.
  // Its eventual response cannot replace the current account's authorized groups.
  it('rejects responses from a previous account and clears its refresh timer', async () => {
    let resolveOld!: (value: MonitoringSnapshot) => void
    read
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve
          }),
      )
      .mockResolvedValueOnce(snapshot('Current account'))
    const stopTimer = vi.spyOn(globalThis, 'clearInterval')
    const view = mountMonitoring()
    await flushPromises()
    auth.user = { id: 8 }
    await flushPromises()
    resolveOld(snapshot('Old private group'))
    await flushPromises()
    expect(view.text()).toContain('Current account')
    expect(view.text()).not.toContain('Old private group')
    view.unmount()
    expect(stopTimer).toHaveBeenCalled()
    stopTimer.mockRestore()
  })
})
