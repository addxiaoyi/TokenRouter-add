import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import ModelMarketplaceView from '../ModelMarketplaceView.vue'
import type { MarketplaceGroup } from '@/api/marketplace.types'

// ModelMarketplaceView.spec.ts exercises model-level browsing and original group offers.
// Public and private catalogs use separate API doubles, while real catalog and
// monitoring composables retain identity invalidation. Lightweight shell and dialog
// substitutes keep quote selection observable without network calls or animation.

// Catalog reads can be delayed independently so stale private responses remain
// distinguishable from the current public or replacement-account catalog.
const fixture = vi.hoisted(() => ({
  public: vi.fn(),
  available: vi.fn(),
  monitoring: vi.fn(),
  copy: vi.fn(),
}))
const auth = reactive({ isAuthenticated: true, user: { id: 7 }, checkAuth: vi.fn() })
const route = reactive({ path: '/models', query: {} as Record<string, string> })
vi.mock('@/api/marketplace', () => ({
  getMarketplaceModels: fixture.public,
  getAvailableMarketplaceModels: fixture.available,
}))
vi.mock('@/api/monitoring', () => ({ readMonitoring: fixture.monitoring }))
vi.mock('@/stores', () => ({
  useAuthStore: () => auth,
  useAppStore: () => ({ publicSettingsLoaded: true, fetchPublicSettings: vi.fn() }),
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: fixture.copy }),
}))
vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({ balanceUnitName: { value: 'USD' } }),
}))

// groups creates distinct routing quotes for one shared model and one unpriced model.
function groups(): MarketplaceGroup[] {
  return [1, 2].map((id) => ({
    id,
    name: `Group ${id}`,
    description: `route ${id}`,
    display_brand: id === 1 ? 'OpenAI' : 'Anthropic',
    sort_order: id,
    rate_multiplier: id,
    hide_max_discount: id === 2,
    hide_rate_multiplier: id === 2,
    official_price_ratio: 0.5,
    model_count: 2,
    models: [
      {
        id: 'shared',
        display_name: 'Shared model',
        pricing: {
          pricing_mode: 'token',
          price_status: 'priced',
          input_price_per_token: id * 0.000001,
          output_price_per_token: id * 0.000002,
        },
      },
      {
        id: 'unpriced',
        display_name: 'Unpriced model',
        pricing: { pricing_mode: 'token', price_status: 'unpriced' },
      },
    ],
  }))
}

// mountCatalog keeps the catalog state real while bounding unrelated shell rendering.
function mountCatalog() {
  return mount(ModelMarketplaceView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        AppHeader: true,
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template:
            '<select :value="modelValue" @change="$emit(\'update:modelValue\', options.find(option => String(option.value) === $event.target.value).value)"><option v-for="option in options" :key="option.value" :value="option.value">{{option.label}}</option></select>',
        },
        Icon: true,
        ProviderIcon: true,
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show" data-testid="dialog"><slot /></div>',
        },
        ModelPricingPanel: {
          props: ['model'],
          template: '<div data-testid="offer-price">{{model.pricing.input_price_per_token}}</div>',
        },
        ModelAttributeDetails: true,
        GroupAvailabilityBar: true,
        ModelCapabilityTags: true,
        Pagination: true,
      },
    },
  })
}

beforeEach(() => {
  vi.resetAllMocks()
  auth.isAuthenticated = true
  auth.user = { id: 7 }
  route.query = {}
  fixture.available.mockResolvedValue(groups())
  fixture.public.mockResolvedValue(groups().slice(0, 1))
  fixture.monitoring.mockResolvedValue({
    enabled: true,
    start: '2026-11-22T00:00:00Z',
    end: '2026-11-23T00:00:00Z',
    groups: [],
  })
})

describe('model catalog', () => {
  // The lowest priced offer can appear after the first group in catalog order.
  // Opening details must preserve that displayed offer before the user selects another route.
  it('opens the displayed starting quote when a later group is cheaper', async () => {
    const offers = groups()
    offers[1]!.models[0]!.pricing.input_price_per_token = 0.0000005
    fixture.available.mockResolvedValue(offers)
    const view = mountCatalog()
    await flushPromises()
    await view.get('[data-model-id="shared"] .catalog-detail-button').trigger('click')
    expect(view.get('[data-testid="offer-price"]').text()).toBe('5e-7')
    view.unmount()
  })

  // Persistent group choices filter original offers before model summaries are built.
  // The selected route must supply both displayed prices and the detail dialog's initial offer.
  it('selects a sidebar group and keeps its original quote in details', async () => {
    const view = mountCatalog()
    await flushPromises()
    const group = view
      .findAll('.reference-group-list button')
      .find((button) => button.text().includes('Group 2'))!
    await group.trigger('click')
    expect(group.attributes('aria-pressed')).toBe('true')
    expect(view.get('[data-model-id="shared"]').text()).toContain('2.00 USD')
    await view.get('[data-model-id="shared"] .catalog-detail-button').trigger('click')
    expect(view.get('[data-testid="offer-price"]').text()).toBe('0.000002')
    view.unmount()
  })

  // Repeated model identities share a browsing row while their original routing quotes remain distinct.
  // Switching the offer inside details must use that group's price without changing the catalog summary.
  it('merges models and preserves every group-specific quote', async () => {
    const view = mountCatalog()
    await flushPromises()
    expect(view.findAll('article')).toHaveLength(2)
    expect(view.get('[data-model-id="shared"]').text()).toContain('1.00 USD')
    await view.get('[data-model-id="shared"] .catalog-detail-button').trigger('click')
    expect(view.get('[data-testid="offer-price"]').text()).toBe('0.000001')
    await view.get('#catalog-offer').setValue(2)
    expect(view.get('[data-testid="offer-price"]').text()).toBe('0.000002')
    view.unmount()
  })

  // Global-search links supply a model query and can update it without replacing the page.
  // Each query is reflected in the actual field and narrows the model-level rows.
  it('uses reactive route search and keeps unpriced models discoverable', async () => {
    route.query = { q: 'shared' }
    const view = mountCatalog()
    await flushPromises()
    expect(view.findAll('article')).toHaveLength(1)
    route.query = { q: 'unpriced' }
    await flushPromises()
    expect(view.get('[data-model-id="unpriced"]').text()).toContain(
      'marketplace.pricingUnavailable',
    )
    view.unmount()
  })

  // A view change affects comparison density while preserving the selected search and quotes.
  // Token scale changes presentation amounts, and model identifiers remain copyable in cards.
  it('switches table density and token units without changing the model set', async () => {
    const view = mountCatalog()
    await flushPromises()
    await view.get('[data-testid="marketplace-search"]').setValue('shared')
    const buttons = view.findAll('button')
    await buttons.find((button) => button.text() === '/1K')!.trigger('click')
    expect(view.get('[data-model-id="shared"]').text()).toContain('0.0010 USD')
    await view.get('[aria-label="common.copy shared"]').trigger('click')
    expect(fixture.copy).toHaveBeenCalledWith('shared')
    await buttons.find((button) => button.text() === 'marketplace.catalogTable')!.trigger('click')
    expect(view.findAll('tbody tr')).toHaveLength(1)
    expect((view.get('[data-testid="marketplace-search"]').element as HTMLInputElement).value).toBe(
      'shared',
    )
    view.unmount()
  })

  // A group selection exposes only that catalog group's models and honors its hidden rate tags.
  // The selected choice remains local filtering rather than an API-key reassignment request.
  it('uses monitored group choices and respects hidden multiplier metadata', async () => {
    const view = mountCatalog()
    await flushPromises()
    await view.get('.catalog-group-filter').trigger('click')
    const choices = view.findAll('.catalog-group-grid button')
    expect(choices[0]!.text()).toContain('×1')
    expect(choices[1]!.text()).not.toContain('×2')
    await choices[1]!.trigger('click')
    await view.get('[data-model-id="shared"] .catalog-detail-button').trigger('click')
    expect(view.get('#catalog-offer').findAll('option')).toHaveLength(1)
    expect(view.get('[data-testid="offer-price"]').text()).toBe('0.000002')
    view.unmount()
  })

  // Loading and failed catalog reads must not look like an empty successful response.
  // Retrying recovers model rows, while genuinely empty data exposes a resettable empty state.
  it('distinguishes loading, failure, and empty responses', async () => {
    fixture.available.mockRejectedValueOnce(new Error('catalog offline'))
    const view = mountCatalog()
    expect(view.find('[data-testid="marketplace-skeleton"]').exists()).toBe(true)
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('catalog offline')
    await view.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(view.findAll('article')).toHaveLength(2)
    await view.get('[data-testid="marketplace-search"]').setValue('missing')
    expect(view.text()).toContain('marketplace.emptyTitle')
    view.unmount()
  })

  // Logout happens while a private catalog is still loading; the old completion arrives afterward.
  // Both late success and failure must leave the guest catalog and public endpoint choice intact.
  it.each(['success', 'failure'])('rejects a late private %s after logout', async (completion) => {
    let resolve!: (value: MarketplaceGroup[]) => void
    let reject!: (cause: Error) => void
    fixture.available.mockImplementationOnce(
      () =>
        new Promise((yes, no) => {
          resolve = yes
          reject = no
        }),
    )
    const view = mountCatalog()
    await flushPromises()
    auth.isAuthenticated = false
    await flushPromises()
    if (completion === 'success')
      resolve([
        {
          ...groups()[0]!,
          name: 'PRIVATE',
          models: [
            {
              id: 'private',
              display_name: 'PRIVATE',
              pricing: { pricing_mode: 'token', price_status: 'unpriced' },
            },
          ],
        },
      ])
    else reject(new Error('private failure'))
    await flushPromises()
    expect(fixture.public).toHaveBeenCalledOnce()
    expect(view.text()).not.toContain('PRIVATE')
    expect(view.text()).not.toContain('private failure')
    expect(view.findAll('article')).toHaveLength(2)
    view.unmount()
  })
})
