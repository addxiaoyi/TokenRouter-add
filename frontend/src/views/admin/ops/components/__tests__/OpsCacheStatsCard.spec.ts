import { describe, it, expect, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsCacheStatsCard from '../OpsCacheStatsCard.vue'

// OpsCacheStatsCard.spec.ts checks cache-statistics filters, overall and
// per-group presentation, and empty or failed retrieval with mocked API
// results. Selector doubles emit the public model events while the real
// component builds requests and renders rates, totals, and group rows.

// mockGetCacheStats captures filter parameters while each scenario controls
// response rows or retrieval failures.
const mockGetCacheStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getCacheStats: (...args: any[]) => mockGetCacheStats(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

// SelectStub exposes model events and options so tests drive the real card query
// controls without dropdown interactions.
const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    }, // selected control fixture value
    options: {
      type: Array,
      default: () => [],
    }, // selectable control fixture entries
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

// EmptyStateStub renders its supplied labels so tests can observe the card
// decision to display no matching statistics.
const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' }, // displayed stub heading text
    description: { type: String, default: '' }, // displayed stub explanatory text
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

// sampleResponse supplies overall totals with a measured rate and two group
// rows. The second row has no group name and no observed rate, so the card
// must fall back to its identifier and a dash.
const sampleResponse = {
  time_range: '30d' as const,
  start_time: '2026-01-01T00:00:00Z',
  end_time: '2026-01-31T00:00:00Z',
  platform: 'anthropic',
  group_id: 7,
  overall: {
    request_count: 40,
    input_tokens: 1000,
    cache_creation_tokens: 200,
    cache_read_tokens: 300,
    total_input_tokens: 1500,
    hit_rate: 0.2,
  },
  groups: [
    {
      group_id: 7,
      group_name: 'Claude 主分组',
      request_count: 30,
      input_tokens: 600,
      cache_creation_tokens: 150,
      cache_read_tokens: 250,
      total_input_tokens: 1000,
      hit_rate: 0.25,
    },
    {
      group_id: 12,
      group_name: '',
      request_count: 10,
      input_tokens: 400,
      cache_creation_tokens: 50,
      cache_read_tokens: 50,
      total_input_tokens: 500,
      hit_rate: null,
    },
  ],
}

// emptyResponse supplies a window without requests, so the overall block has
// no observed rate and the group breakdown stays empty.
const emptyResponse = {
  time_range: '30d' as const,
  start_time: '2026-01-01T00:00:00Z',
  end_time: '2026-01-31T00:00:00Z',
  overall: {
    request_count: 0,
    input_tokens: 0,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    total_input_tokens: 0,
    hit_rate: null,
  },
  groups: [],
}

describe('OpsCacheStatsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The card loads with the parent platform and group constraints applied.
  // The rendered output must show the overall rate as a percentage, the named
  // group row with its own rate, and the identifier fallback plus dash for
  // the unnamed group without an observed rate.
  it('默认加载并透传 platform/group 过滤，渲染整体与分组命中率', async () => {
    mockGetCacheStats.mockResolvedValue(sampleResponse)

    const wrapper = mount(OpsCacheStatsCard, {
      props: {
        platformFilter: 'anthropic',
        groupIdFilter: 7,
        groups: [{ id: 7, name: 'Claude 主分组' }],
      },
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()
    expect(mockGetCacheStats).toHaveBeenCalledWith(
      expect.objectContaining({
        time_range: '30d',
        platform: 'anthropic',
        group_id: 7,
      }),
    )
    expect(wrapper.text()).toContain('20.00%')
    expect(wrapper.text()).toContain('Claude 主分组')
    expect(wrapper.text()).toContain('25.00%')
    expect(wrapper.text()).toContain('#12')
  })

  // A window without requests produces no group rows. The card must present
  // the empty state instead of an overall block of zeros.
  it('无请求时显示空状态', async () => {
    mockGetCacheStats.mockResolvedValue(emptyResponse)

    const wrapper = mount(OpsCacheStatsCard, {
      props: {},
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.find('.empty-state').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.ops.cacheStats.empty')
  })

  // A failed request clears the statistics. The card must surface the
  // translated failure message so the administrator can retry.
  it('加载失败时显示错误信息', async () => {
    mockGetCacheStats.mockRejectedValue(new Error('network down'))

    const wrapper = mount(OpsCacheStatsCard, {
      props: {},
      global: {
        stubs: {
          Select: SelectStub,
          EmptyState: EmptyStateStub,
        },
      },
    })

    await flushPromises()
    expect(wrapper.text()).toContain('network down')
  })
})
