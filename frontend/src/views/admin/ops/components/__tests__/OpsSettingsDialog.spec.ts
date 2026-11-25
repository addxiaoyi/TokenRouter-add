import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSettingsDialog from '../OpsSettingsDialog.vue'
import type {
  EmailNotificationConfig,
  OpsAdvancedSettings,
  OpsAlertRuntimeSettings,
} from '@/api/admin/ops'

// OpsSettingsDialog.spec.ts exercises settings loads and independent saves through
// the real editor with controlled API responses. Deferred requests reproduce a
// failed sibling finishing before another write, while rendered outcomes and
// emitted events establish which persisted state the administrator can observe.

// API substitutes expose each settings family independently so scenarios can
// control completion order without replacing the dialog's real save orchestration.
const { api, showError, showSuccess } = vi.hoisted(() => ({
  api: {
    getAlertRuntimeSettings: vi.fn(),
    getEmailNotificationConfig: vi.fn(),
    getAdvancedSettings: vi.fn(),
    getMetricThresholds: vi.fn(),
    updateAlertRuntimeSettings: vi.fn(),
    updateEmailNotificationConfig: vi.fn(),
    updateAdvancedSettings: vi.fn(),
    updateMetricThresholds: vi.fn(),
  },
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin/ops', () => ({ opsAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}))

// runtimeFixture creates valid evaluator settings with all untouched nested
// fields present. Each load receives its own object so form edits in one test
// cannot affect another test's initial state or request assertions.
function runtimeFixture(): OpsAlertRuntimeSettings {
  return {
    evaluation_interval_seconds: 30,
    distributed_lock: { enabled: false, key: 'test', ttl_seconds: 60 },
    silencing: { enabled: false, global_until_rfc3339: '', global_reason: '' },
    thresholds: {},
  }
}

// emailFixture supplies disabled alert and report delivery with complete report
// fields. Tests explicitly enable a section when checking recipient validation,
// keeping the fixture's original delivery intent observable in submitted requests.
function emailFixture(): EmailNotificationConfig {
  return {
    alert: {
      enabled: false,
      recipients: [],
      min_severity: '',
      rate_limit_per_hour: 10,
      batching_window_seconds: 0,
      include_resolved_alerts: false,
    },
    report: {
      enabled: false,
      recipients: [],
      daily_summary_enabled: false,
      daily_summary_schedule: '0 3 * * *',
      weekly_summary_enabled: false,
      weekly_summary_schedule: '0 3 * * 1',
      error_digest_enabled: false,
      error_digest_schedule: '0 * * * *',
      error_digest_min_count: 1,
      provider_health_enabled: false,
      provider_health_schedule: '0 3 * * *',
      provider_health_error_rate_threshold: 5,
    },
  }
}

// advancedFixture creates valid retention, exclusion, and dashboard settings.
// Retention inputs have explicit finite values so clearing one field tests the
// browser draft rather than relying on missing fixture data.
function advancedFixture(): OpsAdvancedSettings {
  return {
    data_retention: {
      cleanup_enabled: false,
      cleanup_schedule: '0 3 * * *',
      cleanup_batch_size: 1000,
      cleanup_pause_ms: 200,
      error_log_retention_days: 7,
      minute_metrics_retention_days: 7,
      hourly_metrics_retention_days: 30,
    },
    openai_provider_quota_auto_pause: { default_threshold_5h: 0, default_threshold_7d: 0 },
    ignored_status_codes: [401, 403],
    ignore_count_tokens_errors: false,
    ignore_context_canceled: false,
    ignore_no_available_providers: false,
    ignore_invalid_api_key_errors: false,
    ignore_insufficient_balance_errors: false,
    display_openai_token_stats: false,
    display_alert_events: false,
    auto_refresh_enabled: false,
    auto_refresh_interval_seconds: 30,
  }
}

// mountDialog keeps the real input and submission logic while replacing dialog
// chrome and unrelated controls. Callers unmount the returned wrapper after
// settling requests, allowing Vue to dispose the editor's request generation.
function mountDialog(show = true) {
  return mount(OpsSettingsDialog, {
    props: { show },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        Disclosure: { template: '<div><slot /></div>' },
        Select: true,
        Toggle: true,
        Skeleton: true,
        Icon: true,
      },
    },
  })
}

describe('Ops settings persistence', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    for (const method of Object.values(api)) method.mockReset()
    api.getAlertRuntimeSettings.mockResolvedValue(runtimeFixture())
    api.getEmailNotificationConfig.mockResolvedValue(emailFixture())
    api.getAdvancedSettings.mockResolvedValue(advancedFixture())
    api.getMetricThresholds.mockResolvedValue({})
    api.updateAlertRuntimeSettings.mockResolvedValue({})
    api.updateEmailNotificationConfig.mockResolvedValue({})
    api.updateAdvancedSettings.mockResolvedValue({})
    api.updateMetricThresholds.mockResolvedValue(undefined)
  })

  afterEach(() => vi.restoreAllMocks())

  // The editor first mounts already visible, as a lazy dialog can do. It must
  // load every settings family before the administrator can issue a save.
  it('loads on the first visible mount and blocks saving during loading', async () => {
    let resolve!: (value: OpsAdvancedSettings) => void
    api.getAdvancedSettings.mockReturnValue(
      new Promise((done) => {
        resolve = done
      }),
    )
    const wrapper = mountDialog()
    expect(api.getAlertRuntimeSettings).toHaveBeenCalledOnce()
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeDefined()
    await wrapper.get('.btn-primary').trigger('click')
    expect(api.updateMetricThresholds).not.toHaveBeenCalled()
    resolve(advancedFixture())
    await flushPromises()
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  // All four endpoints acknowledge the submitted settings. The editor should
  // emit one refresh and close only after the complete submission succeeds.
  it('closes after every settings family confirms success', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()
    expect(
      Object.values(api)
        .slice(4)
        .every((method) => method.mock.calls.length === 1),
    ).toBe(true)
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  // Email rejects while an advanced-settings write remains pending. The editor
  // must expose both outcomes and wait for the pending write before accepting retries.
  it('waits for pending siblings and shows each partially saved outcome', async () => {
    let finish!: (value: OpsAdvancedSettings) => void
    api.updateEmailNotificationConfig.mockRejectedValue({
      response: { data: { detail: 'mail rejected' } },
    })
    api.updateAdvancedSettings.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve
      }),
    )
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-save-status="saved"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-save-status="failed"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-save-status="pending"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('mail rejected')
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('saved')).toBeUndefined()
    await wrapper.get('.btn-primary').trigger('click')
    expect(api.updateAdvancedSettings).toHaveBeenCalledOnce()
    finish(advancedFixture())
    await flushPromises()
    expect(wrapper.findAll('[data-save-status="saved"]')).toHaveLength(3)
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('admin.ops.email.title'))
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  // Every request rejects with an unknown server outcome. The editor retains
  // its drafts and reports four unconfirmed saves without announcing success or closing.
  it('keeps the editor open when no settings family confirms success', async () => {
    for (const method of Object.values(api).slice(4)) method.mockRejectedValue(new Error('offline'))
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-save-status="failed"]')).toHaveLength(4)
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  // Delivery is explicitly enabled with no destination. Validation must retain
  // that choice and require a recipient instead of silently disabling delivery during persistence.
  it.each(['alert', 'report'] as const)(
    'requires recipients for enabled %s delivery',
    async (section) => {
      const email = emailFixture()
      email[section].enabled = true
      api.getEmailNotificationConfig.mockResolvedValue(email)
      const wrapper = mountDialog()
      await flushPromises()
      expect(wrapper.text()).toContain(`admin.ops.email.validation.${section}RecipientsRequired`)
      expect(wrapper.get('.btn-primary').attributes('disabled')).toBeDefined()
      expect(email[section].enabled).toBe(true)
      expect(api.updateEmailNotificationConfig).not.toHaveBeenCalled()
      wrapper.unmount()
    },
  )

  // Retention values originate in numeric inputs but can become empty or
  // fractional after editing. The form must require integer days before any settings request starts.
  it.each(['', '1.5'])('rejects an invalid retention field: %s', async (value) => {
    const wrapper = mountDialog()
    await flushPromises()
    const retention = wrapper
      .findAll('input[type="number"]')
      .find((input) => input.attributes('max') === '365')!
    await retention.setValue(value)
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('admin.ops.settings.validation.retentionDaysRange')
    wrapper.unmount()
  })

  // A closed editing session has a delayed response and a new session loads
  // first. Completing the old request must not overwrite the newly loaded evaluator value.
  it('ignores a previous opening that finishes after a newer load', async () => {
    let finish!: (value: OpsAlertRuntimeSettings) => void
    api.getAlertRuntimeSettings.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = resolve
      }),
    )
    const wrapper = mountDialog()
    await wrapper.setProps({ show: false })
    api.getAlertRuntimeSettings.mockResolvedValue({
      ...runtimeFixture(),
      evaluation_interval_seconds: 60,
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe('60')
    finish(runtimeFixture())
    await flushPromises()
    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe('60')
    wrapper.unmount()
  })

  // An initial request fails, leaving no complete draft to submit. An explicit
  // retry must reload all settings and enable saving only after the retry succeeds.
  it('keeps saving disabled after loading fails and supports retry', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    api.getAdvancedSettings.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeDefined()
    const retry = wrapper.findAll('button').find((button) => button.text() === 'common.retry')!
    await retry.trigger('click')
    await flushPromises()
    expect(wrapper.get('.btn-primary').attributes('disabled')).toBeUndefined()
    expect(api.getAdvancedSettings).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
