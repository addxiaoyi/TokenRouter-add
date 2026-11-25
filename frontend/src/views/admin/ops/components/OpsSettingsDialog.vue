<script setup lang="ts">
import Skeleton from '@/components/common/Skeleton.vue'

import Disclosure from '@/components/common/Disclosure.vue'

import { ref, computed, watch, onScopeDispose } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI } from '@/api/admin/ops'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import type {
  OpsAlertRuntimeSettings,
  EmailNotificationConfig,
  AlertSeverity,
  OpsAdvancedSettings,
  OpsMetricThresholds,
} from '../types'

// OpsSettingsDialog.vue edits alert evaluation, email delivery, retention,
// dashboard display, and metric threshold settings. Each settings family has its
// own API response and update endpoint, while the dialog presents one validation
// result and one save action. Missing status-code configuration receives the
// standard ignored-code defaults when loaded.
//
// Saving validates recipient intent and normalizes ignored codes before issuing
// independent updates in parallel. Each completed request receives its own
// visible result because sibling writes cannot be rolled back together. The
// dialog waits for all requests before allowing another save, refreshes the
// dashboard after any success, and remains open when any family fails. Opening
// starts a fresh load generation; hidden or disposed generations cannot publish
// drafts over a newer editing session.

// Translation and application notifications supply dialog labels, validation
// feedback, and settings persistence results for this editor.
const { t } = useI18n()
const appStore = useAppStore()

// props controls dialog visibility, while close and saved events tell the parent
// when to hide or refresh it.
const props = defineProps<{
  show: boolean // owner-controlled dialog visibility
}>()

// emit publishes successful persistence and dismissal events after the
// corresponding dialog action completes for the parent.
const emit = defineEmits<{
  close: [] // dialog dismissal event payload
  saved: [] // at least one settings family persisted
}>()

// Loading and saving flags track request phases separately, while the
// ignored-code input retains editable text until validation.
const loading = ref(false)
const saving = ref(false)
const ignoredStatusCodesInput = ref('401, 403')

// SaveResult identifies one settings family's outcome without implying that
// the independent server writes form a single atomic administrative update.
interface SaveResult {
  label: string // translated settings family label
  status: 'pending' | 'saved' | 'failed' // observed settings request outcome
  error?: string // server or translated error detail
}

// Save results remain visible after partial failure; each new load or explicit
// save replaces them so old outcomes cannot describe a newer submission.
const saveResults = ref<SaveResult[]>([])
const loadFailed = ref(false)
// loadGeneration invalidates earlier requests when visibility changes or the
// component is disposed, preventing stale loads from replacing current drafts.
let loadGeneration = 0

// runtimeSettings holds the loaded alert evaluator settings; null means that
// settings family has not been populated.
const runtimeSettings = ref<OpsAlertRuntimeSettings | null>(null)
// emailConfig owns editable alert and report recipient lists and enablement
// flags before the combined save action.
const emailConfig = ref<EmailNotificationConfig | null>(null)
// advancedSettings holds retention and dashboard options, including the
// normalized ignored status codes sent on save.
const advancedSettings = ref<OpsAdvancedSettings | null>(null)
// metricThresholds begins with dashboard defaults and accepts individually
// supplied server values when threshold settings load.
const metricThresholds = ref<OpsMetricThresholds>({
  sla_percent_min: 99.5,
  ttft_p99_ms_max: 500,
  request_error_rate_percent_max: 5,
  upstream_error_rate_percent_max: 5,
})

// parseIgnoredStatusCodes validates comma- or whitespace-separated decimal HTTP
// statuses. Empty text returns an empty accepted list; invalid tokens reject the
// whole input, while valid codes are deduplicated and sorted numerically.
function parseIgnoredStatusCodes(raw: string): {
  // valid distinguishes an accepted empty list from rejected input; callers consult this flag
  // before using codes.
  valid: boolean // input acceptance flag
  // codes contains sorted unique statuses when valid is true; rejected input returns
  // an empty list alongside a validation error.
  codes: number[] // normalized HTTP status list
  error?: string // optional translated validation explanation
} {
  const text = raw.trim()
  if (!text) return { valid: true, codes: [] }
  const parts = text
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
  const seen = new Set<number>()
  const codes: number[] = []
  for (const part of parts) {
    if (!/^\d+$/.test(part)) {
      return {
        valid: false,
        codes: [],
        error: t('admin.ops.settings.validation.ignoredStatusCodesRange'),
      }
    }
    const code = Number.parseInt(part, 10)
    // Reject statuses outside the HTTP response range before saving, so one
    // malformed entry cannot enter the ignored-status configuration.
    if (!Number.isFinite(code) || code < 100 || code > 599) {
      return {
        valid: false,
        codes: [],
        error: t('admin.ops.settings.validation.ignoredStatusCodesRange'),
      }
    }
    if (!seen.has(code)) {
      seen.add(code)
      codes.push(code)
    }
  }
  codes.sort((a, b) => a - b)
  return { valid: true, codes }
}

// formatIgnoredStatusCodes formats validated status codes as comma-separated
// text for editing. Missing arrays use 401 and 403, and an invalid supplied list
// also falls back to that displayed default.
function formatIgnoredStatusCodes(codes: number[] | null | undefined): string {
  const fallback = Array.isArray(codes) ? codes : [401, 403]
  const parsed = parseIgnoredStatusCodes(fallback.join(', '))
  return parsed.valid ? parsed.codes.join(', ') : '401, 403'
}

// loadAllSettings retrieves all settings families before exposing a saveable
// draft. Opening clears earlier values, and a failed load keeps saving disabled
// until the administrator explicitly retries or reopens the dialog.
//
// Each invocation captures the current generation before starting requests.
// Closing, reopening, or disposing invalidates that generation, so a late
// response cannot overwrite newer edits or clear a newer loading indicator.
// Missing ignored-code arrays and threshold fields use the editor defaults only
// after the complete current generation has loaded successfully.
async function loadAllSettings() {
  const generation = ++loadGeneration
  loading.value = true
  loadFailed.value = false
  saveResults.value = []
  runtimeSettings.value = null
  emailConfig.value = null
  advancedSettings.value = null
  try {
    const [runtime, email, advanced, thresholds] = await Promise.all([
      opsAPI.getAlertRuntimeSettings(),
      opsAPI.getEmailNotificationConfig(),
      opsAPI.getAdvancedSettings(),
      opsAPI.getMetricThresholds(),
    ])
    if (generation !== loadGeneration) return
    runtimeSettings.value = runtime
    emailConfig.value = email
    advancedSettings.value = advanced
    // Older or incomplete settings responses can omit the ignored-code array;
    // seed the editable list so saving has a concrete status selection.
    if (!Array.isArray(advancedSettings.value.ignored_status_codes)) {
      advancedSettings.value.ignored_status_codes = [401, 403]
    }
    ignoredStatusCodesInput.value = formatIgnoredStatusCodes(
      advancedSettings.value.ignored_status_codes,
    )
    // Apply supplied threshold fields individually so partially populated
    // settings retain usable defaults for the remaining metric badges.
    metricThresholds.value = {
      sla_percent_min: thresholds?.sla_percent_min ?? 99.5,
      ttft_p99_ms_max: thresholds?.ttft_p99_ms_max ?? 500,
      request_error_rate_percent_max: thresholds?.request_error_rate_percent_max ?? 5,
      upstream_error_rate_percent_max: thresholds?.upstream_error_rate_percent_max ?? 5,
    }
  } catch (err: any) {
    if (generation !== loadGeneration) return
    loadFailed.value = true
    console.error('[OpsSettingsDialog] Failed to load settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.settings.loadFailed'))
  } finally {
    if (generation === loadGeneration) loading.value = false
  }
}

// Opening the dialog reloads persisted settings into its drafts before the next
// edit, while hidden state remains mounted.
watch(
  () => props.show,
  (show) => {
    if (show) {
      loadAllSettings()
    } else {
      // Invalidate the closed session before a pending response can publish
      // settings into a later reopening of this same mounted dialog instance.
      loadGeneration++
    }
  },
  { immediate: true },
)

// Disposal invalidates pending loads because their results belong to this
// instance and must not update drafts after its owning page has gone away.
onScopeDispose(() => {
  loadGeneration++
})

// Recipient inputs remain separate from persisted lists until an add action
// validates and appends a normalized email address.
const alertRecipientInput = ref('')
const reportRecipientInput = ref('')

// severityOptions maps notification severity names to translated labels; its
// empty value leaves email severity unrestricted.
const severityOptions: Array<{
  value: AlertSeverity | '' // notification severity selector value
  label: string // translated display label text
}> = [
  { value: '', label: t('admin.ops.email.minSeverityAll') },
  { value: 'critical', label: t('common.critical') },
  { value: 'warning', label: t('common.warning') },
  { value: 'info', label: t('common.info') },
]

// isValidEmailAddress checks the editor email shape for whitespace and domain
// separators.
function isValidEmailAddress(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
}

// addRecipient validates the selected email input and appends a lowercase
// address to its recipient list. Existing exact normalized addresses are not
// duplicated, and accepted input is cleared after processing.
function addRecipient(target: 'alert' | 'report') {
  if (!emailConfig.value) return
  const raw = (target === 'alert' ? alertRecipientInput.value : reportRecipientInput.value).trim()
  if (!raw) return

  if (!isValidEmailAddress(raw)) {
    appStore.showError(t('common.invalidEmail'))
    return
  }

  // Normalize newly entered addresses before comparing against the recipient list so
  // repeated lowercase entries do not create duplicate destinations.
  const normalized = raw.toLowerCase()
  const list =
    target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  if (!list.includes(normalized)) {
    list.push(normalized)
  }
  if (target === 'alert') alertRecipientInput.value = ''
  else reportRecipientInput.value = ''
}

// removeRecipient removes the first exact address match from the chosen
// recipient list. An unloaded email configuration or an absent address leaves
// the current lists unchanged for the caller.
function removeRecipient(target: 'alert' | 'report', email: string) {
  if (!emailConfig.value) return
  const list =
    target === 'alert' ? emailConfig.value.alert.recipients : emailConfig.value.report.recipients
  const idx = list.indexOf(email)
  if (idx >= 0) list.splice(idx, 1)
}

// validation collects evaluator, recipient, retention, ignored-code, and metric
// errors before saving. Empty enabled recipient lists require an explicit
// correction instead of silently changing the administrator's delivery choice.
const validation = computed(() => {
  const errors: string[] = []

  // Evaluation intervals range from one second to one day.
  if (runtimeSettings.value) {
    const evalSeconds = runtimeSettings.value.evaluation_interval_seconds
    if (!Number.isInteger(evalSeconds) || evalSeconds < 1 || evalSeconds > 86400) {
      errors.push(t('admin.ops.runtime.validation.evalIntervalRange'))
    }
  }

  if (emailConfig.value?.alert.enabled && emailConfig.value.alert.recipients.length === 0) {
    errors.push(t('admin.ops.email.validation.alertRecipientsRequired'))
  }
  if (emailConfig.value?.report.enabled && emailConfig.value.report.recipients.length === 0) {
    errors.push(t('admin.ops.email.validation.reportRecipientsRequired'))
  }

  // Retention and cleanup inputs are checked against the supported day,
  // batch-count, and pause-duration limits before requests are sent.
  if (advancedSettings.value) {
    const {
      error_log_retention_days,
      minute_metrics_retention_days,
      hourly_metrics_retention_days,
    } = advancedSettings.value.data_retention
    if (
      !Number.isInteger(error_log_retention_days) ||
      error_log_retention_days < 0 ||
      error_log_retention_days > 365
    ) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (
      !Number.isInteger(minute_metrics_retention_days) ||
      minute_metrics_retention_days < 0 ||
      minute_metrics_retention_days > 365
    ) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (
      !Number.isInteger(hourly_metrics_retention_days) ||
      hourly_metrics_retention_days < 0 ||
      hourly_metrics_retention_days > 365
    ) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    const { cleanup_batch_size, cleanup_pause_ms } = advancedSettings.value.data_retention
    if (
      !Number.isInteger(cleanup_batch_size) ||
      cleanup_batch_size < 100 ||
      cleanup_batch_size > 5000
    ) {
      errors.push(t('admin.ops.settings.validation.cleanupBatchSizeRange'))
    }
    if (!Number.isInteger(cleanup_pause_ms) || cleanup_pause_ms < 1 || cleanup_pause_ms > 2000) {
      errors.push(t('admin.ops.settings.validation.cleanupPauseRange'))
    }
  }
  const ignoredStatusCodes = parseIgnoredStatusCodes(ignoredStatusCodesInput.value)
  if (!ignoredStatusCodes.valid) {
    errors.push(
      ignoredStatusCodes.error || t('admin.ops.settings.validation.ignoredStatusCodesRange'),
    )
  }

  // Percentage thresholds use zero to one hundred, while first-token duration
  // accepts nonnegative milliseconds when supplied by the editor.
  if (
    metricThresholds.value.sla_percent_min != null &&
    (!Number.isFinite(metricThresholds.value.sla_percent_min) ||
      metricThresholds.value.sla_percent_min < 0 ||
      metricThresholds.value.sla_percent_min > 100)
  ) {
    errors.push(t('admin.ops.settings.validation.slaMinPercentRange'))
  }
  if (
    metricThresholds.value.ttft_p99_ms_max != null &&
    (!Number.isFinite(metricThresholds.value.ttft_p99_ms_max) ||
      metricThresholds.value.ttft_p99_ms_max < 0)
  ) {
    errors.push(t('admin.ops.settings.validation.ttftP99MaxRange'))
  }
  if (
    metricThresholds.value.request_error_rate_percent_max != null &&
    (!Number.isFinite(metricThresholds.value.request_error_rate_percent_max) ||
      metricThresholds.value.request_error_rate_percent_max < 0 ||
      metricThresholds.value.request_error_rate_percent_max > 100)
  ) {
    errors.push(t('admin.ops.settings.validation.requestErrorRateMaxRange'))
  }
  if (
    metricThresholds.value.upstream_error_rate_percent_max != null &&
    (!Number.isFinite(metricThresholds.value.upstream_error_rate_percent_max) ||
      metricThresholds.value.upstream_error_rate_percent_max < 0 ||
      metricThresholds.value.upstream_error_rate_percent_max > 100)
  ) {
    errors.push(t('admin.ops.settings.validation.upstreamErrorRateMaxRange'))
  }

  return { valid: errors.length === 0, errors }
})

// saveAllSettings validates a fully loaded draft and persists its four settings
// families. Concurrent clicks and calls during loading are ignored, and ignored
// HTTP codes are normalized before any request begins.
//
// Requests settle independently and publish individual outcomes before the
// aggregate result is reported. Waiting for every sibling prevents a retry from
// overlapping unfinished writes after an early failure. Any success asks the
// parent to refresh persisted settings; partial failure retains the draft and
// outcome list, while complete success also closes the dialog. Failed requests
// report an unconfirmed save because a network error cannot prove no write occurred.
async function saveAllSettings() {
  if (
    loading.value ||
    saving.value ||
    !runtimeSettings.value ||
    !emailConfig.value ||
    !advancedSettings.value
  )
    return
  if (!validation.value.valid) {
    appStore.showError(validation.value.errors[0])
    return
  }

  saving.value = true
  const generation = loadGeneration
  try {
    if (advancedSettings.value) {
      const ignoredStatusCodes = parseIgnoredStatusCodes(ignoredStatusCodesInput.value)
      advancedSettings.value.ignored_status_codes = ignoredStatusCodes.codes
      ignoredStatusCodesInput.value = ignoredStatusCodes.codes.join(', ')
    }
    const operations = [
      {
        label: t('admin.ops.settings.dataCollection'),
        save: () => opsAPI.updateAlertRuntimeSettings(runtimeSettings.value!),
      },
      {
        label: t('admin.ops.email.title'),
        save: () => opsAPI.updateEmailNotificationConfig(emailConfig.value!),
      },
      {
        label: t('admin.ops.settings.advancedSettings'),
        save: () => opsAPI.updateAdvancedSettings(advancedSettings.value!),
      },
      {
        label: t('admin.ops.settings.metricThresholds'),
        save: () => opsAPI.updateMetricThresholds(metricThresholds.value),
      },
    ]
    saveResults.value = operations.map(({ label }) => ({ label, status: 'pending' }))
    const results = saveResults.value
    // Every request records its own acknowledgement before the dialog permits
    // another submission; a rejected sibling cannot hide later completed writes.
    await Promise.allSettled(
      operations.map(async (operation, index) => {
        try {
          await operation.save()
          results[index].status = 'saved'
        } catch (err: any) {
          results[index].status = 'failed'
          results[index].error =
            err?.response?.data?.message ||
            err?.response?.data?.detail ||
            t('admin.ops.settings.saveFailed')
        }
      }),
    )
    // Forced dismissal or disposal detaches this batch from the current editor.
    // Its retained outcomes cannot close or report success for a newer opening.
    if (generation !== loadGeneration) return
    if (results.some((result) => result.status === 'saved')) emit('saved')
    const failed = results.filter((result) => result.status === 'failed')
    if (failed.length > 0) {
      appStore.showError(
        t('admin.ops.settings.saveIncomplete', {
          sections: failed.map((result) => result.label).join(', '),
        }),
      )
      return
    }
    appStore.showSuccess(t('admin.ops.settings.saveSuccess'))
    emit('close')
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to save settings', err)
    appStore.showError(
      err?.response?.data?.message ||
        err?.response?.data?.detail ||
        t('admin.ops.settings.saveFailed'),
    )
  } finally {
    saving.value = false
  }
}

// requestClose retains the active save's result view until all requests settle.
// Administrators can dismiss the dialog after the pending writes have completed.
function requestClose() {
  if (!saving.value) emit('close')
}
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('admin.ops.settings.title')"
    width="extra-wide"
    @close="requestClose"
  >
    <div
      v-if="loading"
      class="space-y-6"
      role="status"
      :aria-label="t('common.loading')"
      aria-busy="true"
      data-loading-skeleton
    >
      <div class="space-y-3 rounded-surface bg-gray-50 p-4 dark:bg-dark-950" aria-hidden="true">
        <Skeleton :width="142" :height="18" />
        <div class="space-y-2">
          <Skeleton :width="124" :height="14" />
          <Skeleton :height="36" />
          <Skeleton :width="260" :height="12" class="max-w-full" />
        </div>
      </div>
      <div
        v-for="section in 2"
        :key="section"
        class="space-y-3 rounded-surface bg-gray-50 p-4 dark:bg-dark-950"
        aria-hidden="true"
      >
        <Skeleton :width="176" :height="18" />
        <div class="flex items-center justify-between gap-4">
          <Skeleton :width="124" :height="16" />
          <Skeleton :width="44" :height="24" class="shrink-0 rounded-full" />
        </div>
      </div>
    </div>

    <SettingsNotice v-else-if="loadFailed" tone="error">
      <p>{{ t('admin.ops.settings.loadFailed') }}</p>
      <button type="button" class="btn btn-secondary" @click="loadAllSettings">{{
        t('common.retry')
      }}</button>
    </SettingsNotice>

    <fieldset
      v-else-if="runtimeSettings && emailConfig && advancedSettings"
      :disabled="saving"
      class="min-w-0 space-y-6"
    >
      <SettingsNotice
        v-if="saveResults.length > 0"
        tone="warning"
        aria-live="polite"
        data-save-results
      >
        <p>{{ t('admin.ops.settings.saveResultsHint') }}</p>
        <ul class="space-y-1">
          <li v-for="result in saveResults" :key="result.label" :data-save-status="result.status">
            {{ result.label }}: {{ t(`admin.ops.settings.saveStatus.${result.status}`) }}
            <span v-if="result.error" class="break-words"> · {{ result.error }}</span>
          </li>
        </ul>
      </SettingsNotice>
      <!-- Configuration validation errors -->
      <div
        v-if="!validation.valid"
        class="rounded-control border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200"
      >
        <div class="font-bold">{{ t('admin.ops.settings.validation.title') }}</div>
        <ul class="mt-1 list-disc space-y-1 pl-4">
          <li v-for="msg in validation.errors" :key="msg">{{ msg }}</li>
        </ul>
      </div>

      <!-- Collection and evaluation frequency -->
      <div class="rounded-surface bg-gray-50 p-4 dark:bg-dark-950">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{
          t('admin.ops.settings.dataCollection')
        }}</h4>
        <div>
          <label class="input-label">{{ t('admin.ops.settings.evaluationInterval') }}</label>
          <input
            v-model.number="runtimeSettings.evaluation_interval_seconds"
            type="number"
            min="1"
            max="86400"
            class="input"
          />
          <p class="mt-1 text-xs text-gray-500">{{
            t('admin.ops.settings.evaluationIntervalHint')
          }}</p>
        </div>
      </div>

      <!-- Alert email configuration -->
      <div class="rounded-surface bg-gray-50 p-4 dark:bg-dark-950">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{
          t('admin.ops.settings.alertConfig')
        }}</h4>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{
                t('admin.ops.settings.enableAlert')
              }}</label>
            </div>
            <Toggle v-model="emailConfig.alert.enabled" />
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="input-label">{{ t('admin.ops.settings.alertRecipients') }}</label>
            <div class="flex gap-2">
              <input
                v-model="alertRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @keydown.enter.prevent="addRecipient('alert')"
              />
              <button
                class="btn btn-secondary whitespace-nowrap"
                type="button"
                @click="addRecipient('alert')"
              >
                {{ t('common.add') }}
              </button>
            </div>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in emailConfig.alert.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-primary-100 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-500/8 dark:text-primary-500"
              >
                {{ email }}
                <button
                  type="button"
                  class="text-primary-700/80 hover:text-primary-900"
                  @click="removeRecipient('alert', email)"
                  >×</button
                >
              </span>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.alert.enabled">
            <label class="input-label">{{ t('admin.ops.settings.minSeverity') }}</label>
            <Select v-model="emailConfig.alert.min_severity" :options="severityOptions" />
          </div>
        </div>
      </div>

      <!-- Scheduled report configuration -->
      <div class="rounded-surface bg-gray-50 p-4 dark:bg-dark-950">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{
          t('admin.ops.settings.reportConfig')
        }}</h4>

        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">{{
                t('admin.ops.settings.enableReport')
              }}</label>
            </div>
            <Toggle v-model="emailConfig.report.enabled" />
          </div>

          <div v-if="emailConfig.report.enabled">
            <label class="input-label">{{ t('admin.ops.settings.reportRecipients') }}</label>
            <div class="flex gap-2">
              <input
                v-model="reportRecipientInput"
                type="email"
                class="input"
                :placeholder="t('admin.ops.settings.emailPlaceholder')"
                @keydown.enter.prevent="addRecipient('report')"
              />
              <button
                class="btn btn-secondary whitespace-nowrap"
                type="button"
                @click="addRecipient('report')"
              >
                {{ t('common.add') }}
              </button>
            </div>
            <div class="mt-2 flex flex-wrap gap-2">
              <span
                v-for="email in emailConfig.report.recipients"
                :key="email"
                class="inline-flex items-center gap-2 rounded-full bg-primary-100 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-500/8 dark:text-primary-500"
              >
                {{ email }}
                <button
                  type="button"
                  class="text-primary-700/80 hover:text-primary-900"
                  @click="removeRecipient('report', email)"
                  >×</button
                >
              </span>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.ops.settings.recipientsHint') }}
            </p>
          </div>

          <div v-if="emailConfig.report.enabled" class="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                t('admin.ops.settings.dailySummary')
              }}</label>
              <Toggle v-model="emailConfig.report.daily_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.daily_summary_enabled">
              <input
                v-model="emailConfig.report.daily_summary_schedule"
                type="text"
                class="input"
                placeholder="0 9 * * *"
              />
            </div>
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                t('admin.ops.settings.weeklySummary')
              }}</label>
              <Toggle v-model="emailConfig.report.weekly_summary_enabled" />
            </div>
            <div v-if="emailConfig.report.weekly_summary_enabled">
              <input
                v-model="emailConfig.report.weekly_summary_schedule"
                type="text"
                class="input"
                placeholder="0 9 * * 1"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Metric threshold settings -->
      <div class="rounded-surface bg-gray-50 p-4 dark:bg-dark-950">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{
          t('admin.ops.settings.metricThresholds')
        }}</h4>
        <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">{{
          t('admin.ops.settings.metricThresholdsHint')
        }}</p>

        <div class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.ops.settings.slaMinPercent') }}</label>
            <input
              v-model.number="metricThresholds.sla_percent_min"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{
              t('admin.ops.settings.slaMinPercentHint')
            }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.settings.ttftP99MaxMs') }}</label>
            <input
              v-model.number="metricThresholds.ttft_p99_ms_max"
              type="number"
              min="0"
              step="50"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.ttftP99MaxMsHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{
              t('admin.ops.settings.requestErrorRateMaxPercent')
            }}</label>
            <input
              v-model.number="metricThresholds.request_error_rate_percent_max"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{
              t('admin.ops.settings.requestErrorRateMaxPercentHint')
            }}</p>
          </div>

          <div>
            <label class="input-label">{{
              t('admin.ops.settings.upstreamErrorRateMaxPercent')
            }}</label>
            <input
              v-model.number="metricThresholds.upstream_error_rate_percent_max"
              type="number"
              min="0"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{
              t('admin.ops.settings.upstreamErrorRateMaxPercentHint')
            }}</p>
          </div>
        </div>
      </div>

      <!-- Advanced dashboard settings -->
      <Disclosure
        summary-class="cursor-pointer p-4 text-sm font-semibold text-gray-900 dark:text-white"
        class="rounded-surface bg-gray-50 dark:bg-dark-950"
      >
        <template #summary>
          {{ t('admin.ops.settings.advancedSettings') }}
        </template>
        <div class="space-y-4 px-4 pb-4">
          <!-- Observation retention settings -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{
              t('admin.ops.settings.dataRetention')
            }}</h5>

            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                t('admin.ops.settings.enableCleanup')
              }}</label>
              <Toggle v-model="advancedSettings.data_retention.cleanup_enabled" />
            </div>

            <div v-if="advancedSettings.data_retention.cleanup_enabled">
              <label class="input-label">{{ t('admin.ops.settings.cleanupSchedule') }}</label>
              <input
                v-model="advancedSettings.data_retention.cleanup_schedule"
                type="text"
                class="input"
                placeholder="0 3 * * *"
              />
              <p class="mt-1 text-xs text-gray-500">{{
                t('admin.ops.settings.cleanupScheduleHint')
              }}</p>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.cleanupBatchSize') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.cleanup_batch_size"
                  type="number"
                  min="100"
                  max="5000"
                  step="100"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.cleanupPauseMs') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.cleanup_pause_ms"
                  type="number"
                  min="1"
                  max="2000"
                  step="50"
                  class="input"
                />
              </div>
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.cleanupThrottleHint') }}</p>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div>
                <label class="input-label">{{
                  t('admin.ops.settings.errorLogRetentionDays')
                }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.error_log_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{
                  t('admin.ops.settings.minuteMetricsRetentionDays')
                }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.minute_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{
                  t('admin.ops.settings.hourlyMetricsRetentionDays')
                }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.hourly_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.retentionDaysHint') }}</p>
          </div>

          <!-- Error Filtering -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{
              t('admin.ops.settings.errorFiltering')
            }}</h5>

            <div>
              <label class="input-label">{{ t('admin.ops.settings.ignoredStatusCodes') }}</label>
              <input
                v-model="ignoredStatusCodesInput"
                type="text"
                class="input"
                :placeholder="t('admin.ops.settings.ignoredStatusCodesPlaceholder')"
              />
              <p class="mt-1 text-xs text-gray-500">
                {{ t('admin.ops.settings.ignoredStatusCodesHint') }}
              </p>
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.ignoreCountTokensErrors')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreCountTokensErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_count_tokens_errors" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.ignoreContextCanceled')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreContextCanceledHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_context_canceled" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.ignoreNoAvailableProviders')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreNoAvailableProvidersHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_no_available_providers" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.ignoreInsufficientBalanceErrors')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreInsufficientBalanceErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_insufficient_balance_errors" />
            </div>
          </div>

          <!-- Auto Refresh -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{
              t('admin.ops.settings.autoRefresh')
            }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.enableAutoRefresh')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.enableAutoRefreshHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.auto_refresh_enabled" />
            </div>

            <div v-if="advancedSettings.auto_refresh_enabled">
              <label class="input-label">{{ t('admin.ops.settings.refreshInterval') }}</label>
              <Select
                v-model="advancedSettings.auto_refresh_interval_seconds"
                :options="[
                  { value: 15, label: t('admin.ops.settings.refreshInterval15s') },
                  { value: 30, label: t('admin.ops.settings.refreshInterval30s') },
                  { value: 60, label: t('admin.ops.settings.refreshInterval60s') },
                ]"
              />
            </div>
          </div>

          <!-- Dashboard Cards -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{
              t('admin.ops.settings.dashboardCards')
            }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{
                  t('admin.ops.settings.displayAlertEvents')
                }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.displayAlertEventsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_alert_events" />
            </div>
          </div>
        </div>
      </Disclosure>
    </fieldset>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" :disabled="saving" @click="requestClose">{{
          t('common.cancel')
        }}</button>
        <button
          class="btn btn-primary"
          :disabled="
            loading ||
            saving ||
            !runtimeSettings ||
            !emailConfig ||
            !advancedSettings ||
            !validation.valid
          "
          @click="saveAllSettings"
        >
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
