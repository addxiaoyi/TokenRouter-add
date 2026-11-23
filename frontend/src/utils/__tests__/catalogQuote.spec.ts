import { describe, expect, it } from 'vitest'
import { catalogQuote } from '../catalogQuote'
import { buildModelIndex } from '../marketplaceCatalog'
import type { MarketplaceGroup } from '@/api/marketplace.types'

// catalogQuote.spec.ts keeps compact cache prices attached to their original offer.
// Two groups share one model identity but quote different input and cache rates,
// so a merged row must retain the group that supplied its selected price pair.

describe('original catalog quote identity', () => {
  // The cheaper displayed model price comes from the second authorized group.
  // Its cache price must come from that same group rather than the first model occurrence.
  it('matches the displayed pair before selecting cache pricing', () => {
    const groups = [1, 2].map((id) => ({
      id,
      name: `Group ${id}`,
      sort_order: id,
      display_brand: 'OpenAI',
      rate_multiplier: 1,
      description: '',
      model_count: 1,
      models: [
        {
          id: 'shared',
          display_name: 'Shared',
          pricing: {
            pricing_mode: 'token',
            price_status: 'priced',
            input_price_per_token: id === 1 ? 2 : 1,
            output_price_per_token: id === 1 ? 4 : 2,
            cache_read_price_per_token: id === 1 ? 0.7 : 0.2,
          },
        },
      ],
    })) as MarketplaceGroup[]
    const entry = buildModelIndex(groups)[0]!
    const quote = catalogQuote(entry, groups)
    expect(quote?.group.id).toBe(2)
    expect(quote?.model.pricing.cache_read_price_per_token).toBe(0.2)
    const stale = {
      ...entry,
      price: { kind: 'token' as const, input: 99, output: 99, varies: false },
    }
    expect(catalogQuote(stale, groups)).toBeUndefined()
  })

  // An indexed identity no longer exists in the current authorized group response.
  // Resolving its offer stays unavailable instead of borrowing another model's quote.
  it('returns no offer for a model outside the current catalog', () => {
    expect(
      catalogQuote(
        {
          id: 'missing',
          displayName: 'Missing',
          brands: [],
          searchTerms: [],
          groupCount: 0,
          price: { kind: 'unpriced' },
        },
        [],
      ),
    ).toBeUndefined()
  })
})
