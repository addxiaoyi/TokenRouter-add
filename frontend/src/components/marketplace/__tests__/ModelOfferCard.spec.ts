import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelOfferCard from '../ModelOfferCard.vue'
import { buildModelIndex } from '@/utils/marketplaceCatalog'
import { catalogQuote } from '@/utils/catalogQuote'
import type { MarketplaceGroup, MarketplaceModelPricing } from '@/api/marketplace.types'
import type { MonitoringGroup } from '@/api/monitoring'

// ModelOfferCard.spec.ts verifies original quote and observation boundaries in cards.
// Catalog indexing and price formatting remain real, while shell icons and balance
// settings are bounded to keep these checks independent of authentication and networking.

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({ balanceUnitName: { value: 'USD' } }),
}))

// mountOffer builds one original group quote using the real catalog selection rules.
function mountOffer(pricing: MarketplaceModelPricing, stats?: MonitoringGroup) {
  const group: MarketplaceGroup = {
    id: 1,
    name: 'Observed group',
    description: '',
    display_brand: 'OpenAI',
    rate_multiplier: 1,
    model_count: 1,
    models: [{ id: 'shared', display_name: 'Shared', pricing }],
  }
  const entry = buildModelIndex([group])[0]!
  return mount(ModelOfferCard, {
    props: {
      entry,
      quote: catalogQuote(entry, [group]),
      tokenUnit: 'M',
      stats,
      windowEnd: '2026-11-23T12:00:00Z',
    },
    global: { stubs: { Icon: true, ModelIcon: true } },
  })
}

describe('model offer observations', () => {
  // Base input and output prices have no corresponding base cache quote.
  // A cache price from another context interval must stay out of that row.
  it('keeps a context cache price separate from a base quote', () => {
    const view = mountOffer({
      pricing_mode: 'token',
      price_status: 'priced',
      input_price_per_token: 0.000001,
      output_price_per_token: 0.000002,
      context_intervals: [
        { min_context: 0, max_context: 100000, cache_read_price_per_token: 0.000003 },
      ],
    })
    expect(view.get('.price-inline').text()).not.toContain('marketplace.referenceCacheRead')
    view.unmount()
  })

  // A context-only quote explicitly supplies a free cache-read rate.
  // Cards must display that zero beside its matching interval input and output prices.
  it('preserves an explicit zero cache price from the chosen context', () => {
    const view = mountOffer({
      pricing_mode: 'token',
      price_status: 'priced',
      context_intervals: [
        {
          min_context: 0,
          max_context: 100000,
          input_price_per_token: 0.000001,
          output_price_per_token: 0.000002,
          cache_read_price_per_token: 0,
        },
      ],
    })
    expect(view.get('.price-inline').text()).toContain(
      'marketplace.referenceCacheRead 0.0000 USD/1M',
    )
    view.unmount()
  })

  // A snapshot ending exactly on an hour excludes that new hour's observations.
  // The final status bar represents the preceding bucket and cache zero remains measured.
  it('anchors hourly status to the exclusive end and retains zero cache hits', () => {
    const stats: MonitoringGroup = {
      id: 1,
      name: 'Observed group',
      success: 1,
      errors: 0,
      success_rate: 100,
      avg_duration_ms: 2000,
      avg_ttft_ms: 200,
      cache_hit_rate: 0,
      input_tokens: 1,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      trend: [{ time: '2026-11-23T11:00:00Z', success: 1, errors: 0 }],
      models: [],
    }
    const view = mountOffer({ pricing_mode: 'token', price_status: 'priced' }, stats)
    expect(view.findAll('.offer-mini-bars span').at(-1)!.attributes('data-state')).toBe('healthy')
    expect(view.findAll('[data-state="empty"]')).toHaveLength(11)
    expect(view.get('.offer-performance').text()).toContain('0.0%')
    expect(view.get('.offer-performance').text()).toContain('2.00s')
    view.unmount()
  })
})
