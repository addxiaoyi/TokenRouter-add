import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPriceSummary from '../ModelPriceSummary.vue'

// ModelPriceSummary.spec.ts checks billing-unit identity and explicit zero quotes.
// The component formats indexed observations only; changing K/M scale must not
// hide a free side, turn missing data into zero, or relabel request prices as seconds.
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({ balanceUnitName: { value: 'USD' } }),
}))

describe('catalog price summaries', () => {
  // A token model has a free input quote and an unavailable output quote.
  // Both K/M scales retain those distinct meanings while formatting a valid starting price.
  it('preserves zero and missing token sides', async () => {
    const view = mount(ModelPriceSummary, {
      props: { price: { kind: 'token', input: 0, output: null, varies: true } },
    })
    expect(view.text()).toContain('0.0000 USD')
    expect(view.text()).toContain('—')
    expect(view.text()).toContain('marketplace.catalogStartingPrice')
    await view.setProps({ tokenUnit: 'K' })
    expect(view.text()).toContain('/1K tokens')
    expect(view.text()).toContain('0.0000 USD')
  })

  // Free image, video, and request offers use three different billing units.
  // Their zero prices remain observable without replacing their unit with a token scale.
  it.each(['image', 'video', 'per_request'] as const)('preserves the %s billing unit', (kind) => {
    const view = mount(ModelPriceSummary, { props: { price: { kind, amount: 0, varies: false } } })
    expect(view.text()).toContain(`marketplace.catalogUnit.${kind}`)
    expect(view.text()).toContain('0.0000 USD')
    expect(view.text()).not.toContain('/1M tokens')
  })
})
