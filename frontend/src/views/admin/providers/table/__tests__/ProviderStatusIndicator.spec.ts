import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderStatusIndicator from '../ProviderStatusIndicator.vue'
import type { Provider } from '@/api/admin/providers.types'

// ProviderStatusIndicator.spec.ts checks provider status badges for model
// limits, Grok recovery, Antigravity credits, and keyboard-accessible model
// details. Fixed formatter output separates presentation assertions from clock
// behavior, while fixtures distinguish exhausted credits from overage use.

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

vi.mock('@/utils/format', async () => {
  const actual = await vi.importActual<typeof import('@/utils/format')>('@/utils/format')
  return {
    ...actual,
    formatCountdown: () => '2d 20h',
    formatDateTimeToMinute: () => '2099/03/15 00:00',
  }
})

// makeProvider builds an active Antigravity OAuth fixture with no scheduling
// limits. Overrides replace complete fields, letting each status case supply
// its own model-limit map and overage policy.
function makeProvider(overrides: Partial<Provider>): Provider {
  return {
    id: 1,
    name: 'provider',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

describe('ProviderStatusIndicator', () => {
  // A Sonnet 5 model has an active rate limit. The status badge uses the
  // compact CSon5 alias instead of displaying the full model identifier.
  it('Claude Sonnet 5 模型限流时显示短别名', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          extra: {
            model_rate_limits: {
              'claude-sonnet-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2099-07-28T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('CSon5')
    expect(wrapper.text()).not.toContain('claude-sonnet-5')
  })

  // An Opus 5 model has an active rate limit. The status badge uses the
  // compact COpus5 alias instead of displaying the full model identifier.
  it('Claude Opus 5 模型限流时显示短别名', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          extra: {
            model_rate_limits: {
              'claude-opus-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2099-07-28T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })
    expect(wrapper.text()).toContain('COpus5')
    expect(wrapper.text()).not.toContain('claude-opus-5')
  })

  // A Grok provider has quota and legacy temporary-unschedulable timestamps.
  // Its warning shows rate limiting and automatic recovery rather than the
  // temporary scheduling label.
  it('Grok 提供商额度限流时显示自动恢复时间而非临时不可调度', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 5,
          name: 'grok-free-1',
          platform: 'grok',
          rate_limited_at: '2026-07-11T12:00:00Z',
          rate_limit_reset_at: '2099-07-11T13:00:00Z',
          temp_unschedulable_until: '2099-07-11T12:30:00Z',
          temp_unschedulable_reason: 'legacy grok rate limited',
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.find('.badge-warning').text()).toBe('admin.providers.status.rateLimited')
    expect(wrapper.text()).toContain('admin.providers.status.rateLimitedAutoResume')
    expect(wrapper.text()).not.toContain('admin.providers.status.tempUnschedulable')
  })

  // An Antigravity model is limited while overages are enabled and credits are
  // not exhausted. The badge includes the model alias and a bolt icon for
  // credit-backed availability.
  it('模型限流 + overages 启用 + 无 AICredits key → 显示闪电图标 (credits_active)', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 1,
          name: 'ag-1',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.find('icon-stub[name="bolt"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('CSon45')
  })

  // An Antigravity model is limited without overages enabled. The model alias
  // remains visible, and the badge omits the bolt icon associated with
  // available credits.
  it('模型限流 + overages 未启用 → 普通限流样式（无闪电图标）', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 2,
          name: 'ag-2',
          extra: {
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('CSon45')
    expect(wrapper.find('icon-stub[name="bolt"]').exists()).toBe(false)
  })

  // A model limit has a distant recovery timestamp. The badge displays a
  // day-based countdown and a tooltip containing the formatted complete
  // recovery date.
  it('模型限流徽标显示按天倒计时且提示包含完整恢复日期', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 6,
          name: 'ag-long-limit',
          extra: {
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('2d 20h')
    expect(wrapper.text()).toContain(
      'admin.providers.status.modelRateLimitedUntil:{"model":"CSon45","time":"2099/03/15 00:00"}',
    )
    expect(wrapper.find('.whitespace-nowrap').exists()).toBe(true)
  })

  // Provider and model limits share a near-term deadline while the component
  // remains mounted. Both badges must expire without a prop change or manual refresh.
  it('提供商和模型限流到期后会自动恢复为可用状态', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-28T00:00:00Z'))
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          rate_limit_reset_at: '2026-07-28T00:00:01Z',
          extra: {
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2026-07-28T00:00:01Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    try {
      expect(wrapper.text()).toContain('admin.providers.status.rateLimited')
      expect(wrapper.text()).toContain('CSon45')

      await vi.advanceTimersByTimeAsync(1000)
      await wrapper.vm.$nextTick()

      expect(wrapper.text()).not.toContain('admin.providers.status.rateLimited')
      expect(wrapper.text()).not.toContain('CSon45')
      expect(wrapper.text()).toContain('admin.providers.status.active')
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  // Overload and temporary-suspension windows can expire without a new API
  // response. The status must recover and release its interval after the final deadline passes.
  it.each(['overload_until', 'temp_unschedulable_until'] as const)(
    '%s 到期后停止计时',
    async (field) => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date('2026-07-28T00:00:00Z'))
      const timerCount = vi.getTimerCount()
      const wrapper = mount(ProviderStatusIndicator, {
        props: { provider: makeProvider({ [field]: '2026-07-28T00:00:01Z' }) },
        global: { stubs: { Icon: true } },
      })
      try {
        expect(vi.getTimerCount()).toBe(timerCount + 1)
        await vi.advanceTimersByTimeAsync(1000)
        expect(wrapper.text()).toContain('admin.providers.status.active')
        expect(vi.getTimerCount()).toBe(timerCount)
      } finally {
        wrapper.unmount()
        vi.useRealTimers()
      }
    },
  )

  // An idle row has no active deadline and receives a restriction later.
  // Its interval must restart for that new observation and disappear when the row unmounts.
  it('空闲状态收到新限流后重新计时并在卸载时清理', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-28T00:00:00Z'))
    const timerCount = vi.getTimerCount()
    const wrapper = mount(ProviderStatusIndicator, {
      props: { provider: makeProvider({}) },
      global: { stubs: { Icon: true } },
    })
    try {
      expect(vi.getTimerCount()).toBe(timerCount)
      await vi.advanceTimersByTimeAsync(60_000)
      await wrapper.setProps({
        provider: makeProvider({ rate_limit_reset_at: '2026-07-28T00:01:05Z' }),
      })
      expect(wrapper.text()).toContain('admin.providers.status.rateLimited')
      expect(vi.getTimerCount()).toBe(timerCount + 1)
      wrapper.unmount()
      expect(vi.getTimerCount()).toBe(timerCount)
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  // Invalid stored model reset values establish no active backend cooldown.
  // The frontend must omit the model badge and avoid an interval that can never expire.
  it('忽略无效模型限流日期', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          extra: { model_rate_limits: { 'claude-sonnet-5': { rate_limit_reset_at: 'invalid' } } },
        }),
      },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.text()).not.toContain('CSon5')
    wrapper.unmount()
  })

  // Automatic model cooldowns carry persisted classification reasons. Known
  // reasons receive translated explanations while unknown reasons remain inspectable as literal text.
  it.each([
    ['upstream_404_model_not_found', 'admin.providers.status.modelNotFoundReason'],
    ['upstream_400_codex_plan_gated_model', 'admin.providers.status.modelPlanGatedReason'],
    ['future-classification', 'future-classification'],
    ['<img src=x onerror=alert(1)>', '<img src=x onerror=alert(1)>'],
  ])('展示模型临时停用原因 %s', (reason, expected) => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          extra: {
            model_rate_limits: {
              'claude-sonnet-5': { rate_limit_reset_at: '2099-07-28T00:00:00Z', reason },
            },
          },
        }),
      },
      global: { stubs: { Icon: true } },
    })
    expect(wrapper.get('[data-model-restriction-reason]').text()).toBe(expected)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.providers.status.modelCooldownScopeHint')
    wrapper.unmount()
  })

  // Model status rows receive keyboard focus and expose their full tooltip as
  // the group's accessible description through a unique element reference.
  it('associates model cooldown details with its keyboard focus target', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 27,
          extra: {
            model_rate_limits: {
              'claude-sonnet-5': {
                rate_limited_at: '2026-07-28T00:00:00Z',
                rate_limit_reset_at: '2099-07-28T00:00:00Z',
              },
            },
          },
        }),
      },
      global: { stubs: { Icon: true } },
    })

    const status = wrapper.get('[role="group"][tabindex="0"]')
    const labelId = status.attributes('aria-labelledby')
    const tooltipId = status.attributes('aria-describedby')

    expect(labelId).toBeTruthy()
    expect(wrapper.get(`#${labelId}`).exists()).toBe(true)
    expect(tooltipId).toBeTruthy()
    expect(wrapper.get(`#${tooltipId}`).attributes('role')).toBe('tooltip')
    expect(wrapper.get(`#${tooltipId}`).text()).toContain('modelRateLimitedUntil')
    wrapper.unmount()
  })

  // An Antigravity AICredits limit is active while overages are enabled. The
  // status indicator displays the credits-exhausted message for that special
  // limit entry.
  it('AICredits key 生效 → 显示积分已用尽 (credits_exhausted)', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 3,
          name: 'ag-3',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              AICredits: {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('admin.providers.status.creditsExhausted')
  })

  // Model limits and an AICredits limit coexist with overages enabled. The
  // indicator retains the model alias, shows credits exhaustion, and
  // suppresses the bolt icon.
  it('模型限流 + overages 启用 + AICredits key 生效 → 普通限流样式（积分耗尽，无闪电图标）', () => {
    const wrapper = mount(ProviderStatusIndicator, {
      props: {
        provider: makeProvider({
          id: 4,
          name: 'ag-4',
          extra: {
            allow_overages: true,
            model_rate_limits: {
              'claude-sonnet-4-5': {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
              AICredits: {
                rate_limited_at: '2026-03-15T00:00:00Z',
                rate_limit_reset_at: '2099-03-15T00:00:00Z',
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('CSon45')
    expect(wrapper.find('icon-stub[name="bolt"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.providers.status.creditsExhausted')
  })
})
