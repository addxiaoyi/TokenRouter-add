import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  confirmCostRate,
  readProfit,
  type ProfitProvider,
  type ProfitReport,
} from '@/api/admin/profit'
import { exportData } from '@/api/admin/providers'
import { useStepUp, isStepUpCancelled, isStepUpBlocked } from '@/composables/useStepUp'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'

// useProfitReport.ts owns profit queries, rate confirmation, and explicit key access.
// Range refreshes cancel earlier reads and reject late responses. Cost changes
// refresh current reminders without rewriting historical snapshots. Only one
// credential action is admitted at a time because step-up owns one prompt.
//
// Revealed keys exist solely in the mounted page state and are cleared on hiding
// or unmounting. The existing protected export API supplies only the selected
// provider; its raw document is never downloaded or logged. Cancellation closes
// verification without an error, while blocked or failed verification remains
// visible and cannot fall back to an unprotected credential read.

// localDate formats browser calendar boundaries without converting them to UTC.
function localDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

// useProfitReport binds the administrator page to cancellable profit reporting.
// Call during component setup and mount its step-up dialog. Query generations
// protect state across rapid filters, and disposal cancels both reads and pending
// credential prompts. Saving rates is serialized independently from range reads.
//
// Credentials are revealed only after explicit selection and site-configured secondary
// verification. Late export completions cannot repopulate a disposed page. Error
// messages remain user-facing and never include the returned credential payload.
export function useProfitReport() {
  const { t, locale } = useI18n()
  const app = useAppStore()
  const { copyToClipboard } = useClipboard()
  const stepUp = useStepUp()
  // Report state belongs to this page; a generation marks the latest accepted range.
  const start = ref(localDate(new Date()))
  const end = ref(start.value)
  const report = ref<ProfitReport | null>(null)
  const loading = ref(false)
  const error = ref('')
  const search = ref('')
  const missingOnly = ref(false)
  let generation = 0
  let abort: AbortController | undefined
  let disposed = false
  // Drafts and sensitive text stay local and never enter browser persistence.
  const drafts = ref<Record<number, string>>({})
  const saving = ref<number | null>(null)
  const revealing = ref<number | null>(null)
  const secret = ref<{ id: number; text: string } | null>(null)
  const providers = computed(() =>
    (report.value?.providers ?? []).filter(
      (row) =>
        (!missingOnly.value || (!row.configured && !row.deleted)) &&
        `${row.name} ${row.id} ${row.site} ${row.key_hint}`
          .toLowerCase()
          .includes(search.value.trim().toLowerCase()),
    ),
  )

  // refresh replaces the displayed report only for the latest successful range.
  // Failure clears totals so a prior period cannot look current. Cancelled older
  // requests cannot clear a newer report's loading or error state.
  async function refresh(): Promise<void> {
    abort?.abort()
    abort = new AbortController()
    const current = ++generation
    loading.value = true
    error.value = ''
    report.value = null
    hideKey()
    try {
      const next = await readProfit(
        start.value,
        end.value,
        Intl.DateTimeFormat().resolvedOptions().timeZone,
        abort.signal,
      )
      if (current !== generation || disposed) return
      report.value = next
      drafts.value = Object.fromEntries(
        next.providers.map((row) => [row.id, row.configured ? String(row.rate) : '']),
      )
    } catch {
      if (current !== generation || disposed) return
      report.value = null
      error.value = t('admin.profit.loadError')
    } finally {
      if (current === generation && !disposed) loading.value = false
    }
  }

  // saveRate validates an explicit draft before confirming future upstream costs.
  // Empty values are not converted to zero; successful persistence reloads the
  // report, and failed persistence leaves a visible retryable notification.
  async function saveRate(row: ProfitProvider): Promise<void> {
    if (saving.value !== null) return
    const raw = drafts.value[row.id]?.trim() ?? ''
    const rate = Number(raw)
    if (!/^\d+(\.\d{1,4})?$/.test(raw) || !Number.isFinite(rate) || rate > 999999.9999) {
      app.showError(t('admin.profit.invalidRate'))
      return
    }
    saving.value = row.id
    try {
      await confirmCostRate(row.id, rate)
      if (disposed) return
      app.showSuccess(t('admin.profit.saved'))
      await refresh()
    } catch {
      if (!disposed) app.showError(t('admin.profit.saveError'))
    } finally {
      saving.value = null
    }
  }

  // revealKey reads exactly one provider through existing export step-up checks.
  // Call only after an explicit click. Unavailable credentials and blocked
  // verification show errors, while user cancellation leaves secrets hidden.
  async function revealKey(row: ProfitProvider): Promise<void> {
    if (revealing.value !== null) return
    secret.value = null
    revealing.value = row.id
    const current = generation
    try {
      const archive = await stepUp.run(() => exportData({ ids: [row.id], includeProxies: false }))
      if (disposed || current !== generation) return
      const key =
        archive.providers.length === 1 ? archive.providers[0]?.credentials.api_key : undefined
      if (typeof key !== 'string' || !key) throw new Error('API key unavailable')
      secret.value = { id: row.id, text: key }
    } catch (cause) {
      if (disposed || isStepUpCancelled(cause)) return
      app.showError(t(isStepUpBlocked(cause) ? 'admin.profit.keyBlocked' : 'admin.profit.keyError'))
    } finally {
      revealing.value = null
    }
  }

  // hideKey removes the revealed credential from the current page state.
  function hideKey(): void {
    secret.value = null
  }

  // copyKey copies only the credential already revealed by administrator verification.
  async function copyKey(): Promise<void> {
    if (secret.value) await copyToClipboard(secret.value.text)
  }

  // money formats signed USD amounts while preserving small request-level margins.
  function money(value: number): string {
    return new Intl.NumberFormat(locale.value, {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 6,
    }).format(value)
  }

  onMounted(refresh)
  onUnmounted(() => {
    // Revoke asynchronous publication before cancelling the active read and
    // verification prompt, so their continuations cannot restore a revealed key.
    disposed = true
    generation++
    abort?.abort()
    stepUp.onCancel()
    hideKey()
  })
  return {
    start,
    end,
    report,
    loading,
    error,
    search,
    missingOnly,
    providers,
    drafts,
    saving,
    revealing,
    secret,
    stepUp,
    refresh,
    saveRate,
    revealKey,
    hideKey,
    copyKey,
    money,
  }
}
