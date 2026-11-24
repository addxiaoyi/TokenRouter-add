import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProfitProvider, ProfitReport } from '@/api/admin/profit'
import { useProfitReport } from '../useProfitReport'

// useProfitReport.spec.ts checks administrator report ownership and sensitive access.
// Deferred responses reproduce out-of-order queries and exports completing after
// unmount. Explicit zero must remain a valid free rate, while an empty draft must
// never silently confirm that an upstream is free.

// Endpoint and notification spies isolate network activity without bypassing the
// controller's actual request generations, rate validation, or credential lifetime.
const { readProfit, confirmCostRate, exportData, showError } = vi.hoisted(() => ({
  readProfit: vi.fn(),
  confirmCostRate: vi.fn(),
  exportData: vi.fn(),
  showError: vi.fn(),
}))
vi.mock('@/api/admin/profit', () => ({ readProfit, confirmCostRate }))
vi.mock('@/api/admin/providers', () => ({ exportData }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
}))

// reportFixture supplies distinct revenue and one missing-rate provider for page tests.
function reportFixture(revenue = 3): ProfitReport {
  const amounts = {
    requests: 1,
    confirmed_requests: 0,
    missing_requests: 1,
    system_requests: 0,
    missing_system_requests: 0,
    revenue,
    covered_revenue: 0,
    upstream_cost: 0,
    gross_profit: 0,
    system_cost: 0,
  }
  return {
    ...amounts,
    missing_providers: 1,
    days: [],
    providers: [
      {
        ...amounts,
        id: 7,
        name: 'upstream',
        site: 'https://example.test',
        has_key: true,
        key_hint: '••••1234',
        rate: 1,
        configured: false,
        deleted: false,
      },
    ],
  }
}

// mountReport creates a real component lifetime for the composable under test.
function mountReport() {
  let controller!: ReturnType<typeof useProfitReport>
  const wrapper = mount(
    defineComponent({
      setup() {
        controller = useProfitReport()
        return () => null
      },
    }),
  )
  return { wrapper, controller }
}

beforeEach(() => {
  vi.clearAllMocks()
  readProfit.mockResolvedValue(reportFixture())
  confirmCostRate.mockResolvedValue(undefined)
})

describe('profit report controller', () => {
  // A missing draft could otherwise become Number('') and silently save zero.
  // Explicit zero must still persist a genuinely free upstream after confirmation.
  it('rejects an empty multiplier and accepts explicit zero', async () => {
    const { wrapper, controller } = mountReport()
    await flushPromises()
    const provider = controller.providers.value[0] as ProfitProvider
    await controller.saveRate(provider)
    expect(confirmCostRate).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.profit.invalidRate')
    controller.drafts.value[7] = '0'
    await controller.saveRate(provider)
    expect(confirmCostRate).toHaveBeenCalledWith(7, 0)
    wrapper.unmount()
  })

  // An earlier range completes after its replacement request. Only the latest
  // selection may publish totals, even if the network ignores the cancellation.
  it('ignores an older range response', async () => {
    let resolveOld!: (report: ProfitReport) => void
    readProfit.mockImplementationOnce(
      () =>
        new Promise<ProfitReport>((resolve) => {
          resolveOld = resolve
        }),
    )
    const { wrapper, controller } = mountReport()
    await controller.refresh()
    resolveOld(reportFixture(999))
    await flushPromises()
    expect(controller.report.value?.revenue).toBe(3)
    wrapper.unmount()
  })

  // Opening a report must never read raw keys. An explicitly requested export
  // that finishes after unmount must not restore credential text in abandoned state.
  it('loads keys only on demand and discards late credentials', async () => {
    let resolveKey!: (archive: unknown) => void
    exportData.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveKey = resolve
        }),
    )
    const { wrapper, controller } = mountReport()
    await flushPromises()
    expect(exportData).not.toHaveBeenCalled()
    const pending = controller.revealKey(controller.providers.value[0] as ProfitProvider)
    expect(exportData).toHaveBeenCalledWith({ ids: [7], includeProxies: false })
    wrapper.unmount()
    resolveKey({ providers: [{ credentials: { api_key: 'test-secret' } }] })
    await pending
    expect(controller.secret.value).toBeNull()
  })
})
