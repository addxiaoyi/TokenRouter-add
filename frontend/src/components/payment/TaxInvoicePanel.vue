<template>
  <p v-if="loading && !state" role="status" class="text-sm text-gray-500 dark:text-gray-400">
    {{ t('loading') }}
  </p>
  <section v-else-if="visible" data-testid="tax-invoice" class="space-y-4">
    <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('title') }}</h3>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>

    <div v-if="invoice || attemptedBuyer" class="space-y-4">
      <p role="status" class="text-sm font-medium text-gray-900 dark:text-white">{{
        statusText
      }}</p>
      <dl class="grid gap-2 text-sm sm:grid-cols-2">
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('buyerTitle') }}</dt>
          <dd class="break-words text-gray-900 dark:text-white">{{ savedBuyer?.title }}</dd>
        </div>
        <div v-if="savedBuyer?.tax_id">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('taxId') }}</dt>
          <dd class="break-all text-gray-900 dark:text-white">{{ savedBuyer.tax_id }}</dd>
        </div>
        <div>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('email') }}</dt>
          <dd class="break-all text-gray-900 dark:text-white">{{ savedBuyer?.email }}</dd>
        </div>
        <div v-if="invoice">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('amount') }}</dt>
          <dd class="text-gray-900 dark:text-white">{{ invoiceAmount }}</dd>
        </div>
        <div v-if="hasDocument" class="sm:col-span-2">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('invoiceNumber') }}</dt>
          <dd class="break-all font-mono text-gray-900 dark:text-white">{{
            invoice?.invoice_no
          }}</dd>
        </div>
      </dl>
      <p class="text-sm text-gray-500 dark:text-gray-400">{{
        invoice ? t('saved') : t('retryHint')
      }}</p>
      <p v-if="pending" class="text-sm text-gray-500 dark:text-gray-400">{{ t('pending') }}</p>
      <div class="flex flex-wrap gap-2">
        <button class="btn btn-secondary" :disabled="loading || busy" @click="refreshInvoice">
          {{ t('refresh') }}
        </button>
        <button
          v-if="canRetry"
          data-testid="invoice-retry"
          class="btn btn-primary"
          :disabled="loading || busy"
          @click="submitInvoice"
        >
          {{ busy ? t('submitting') : t('retrySubmission') }}
        </button>
        <button
          v-if="canDownload"
          data-testid="invoice-file"
          class="btn btn-primary"
          :disabled="loading || busy"
          @click="requestFile"
        >
          {{ t('requestFile') }}
        </button>
      </div>
      <div v-if="downloadURL" class="space-y-2">
        <a
          :href="downloadURL"
          target="_blank"
          rel="noopener noreferrer"
          class="text-link text-sm"
          >{{ t('download') }}</a
        >
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('downloadHint') }}</p>
      </div>
    </div>

    <form
      v-else-if="state?.enabled && state.eligible"
      class="space-y-4"
      @submit.prevent="submitInvoice"
    >
      <fieldset :disabled="busy || loading" class="space-y-4">
        <legend class="sr-only">{{ t('title') }}</legend>
        <fieldset>
          <legend class="input-label">{{ t('buyerType') }}</legend>
          <div class="mt-2 flex flex-wrap gap-4 text-sm text-gray-900 dark:text-white">
            <label class="flex items-center gap-2">
              <input
                v-model="draft.buyer_type"
                type="radio"
                name="invoice-buyer-type"
                value="INDIVIDUAL"
              />
              {{ t('individual') }}
            </label>
            <label class="flex items-center gap-2">
              <input
                v-model="draft.buyer_type"
                type="radio"
                name="invoice-buyer-type"
                value="ORGANIZATION"
              />
              {{ t('organization') }}
            </label>
          </div>
        </fieldset>
        <div>
          <label :for="`invoice-title-${orderId}`" class="input-label">{{ t('buyerTitle') }}</label>
          <input
            :id="`invoice-title-${orderId}`"
            v-model="draft.title"
            name="invoice-title"
            class="input mt-1 w-full"
            required
            maxlength="160"
            autocomplete="organization"
          />
        </div>
        <div v-if="draft.buyer_type === 'ORGANIZATION'">
          <label :for="`invoice-tax-id-${orderId}`" class="input-label">{{ t('taxId') }}</label>
          <input
            :id="`invoice-tax-id-${orderId}`"
            v-model="draft.tax_id"
            name="invoice-tax-id"
            class="input mt-1 w-full"
            required
            minlength="15"
            maxlength="20"
            pattern="[A-Z0-9]{15,20}"
            autocomplete="off"
          />
        </div>
        <div>
          <label :for="`invoice-email-${orderId}`" class="input-label">{{ t('email') }}</label>
          <input
            :id="`invoice-email-${orderId}`"
            v-model="draft.email"
            name="invoice-email"
            type="email"
            class="input mt-1 w-full"
            required
            maxlength="214"
            autocomplete="email"
          />
        </div>
      </fieldset>
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('buyerHint') }}</p>
      <button type="submit" class="btn btn-primary" :disabled="busy || loading">
        {{ busy ? t('submitting') : t('submit') }}
      </button>
    </form>

    <div v-else class="space-y-2">
      <p v-if="!error" class="text-sm text-gray-500 dark:text-gray-400">{{ t('unavailable') }}</p>
      <button class="btn btn-secondary" :disabled="loading" @click="refreshInvoice">{{
        t('refresh')
      }}</button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { taxInvoicesAPI } from '@/api/taxInvoices'
import type { TaxInvoiceBuyer, TaxInvoiceState } from '@/api/taxInvoices.types'
import en from '@/i18n/locales/en/taxInvoice'
import zh from '@/i18n/locales/zh/taxInvoice'

// TaxInvoicePanel.vue collects buyer details and follows one order's original tax
// invoice application. The backend owns eligibility, invoice amounts, and seller
// settings. A stored application always renders as read-only, including when new
// creation is disabled. Document retrieval requires an enabled service and an
// issued record with a number, so an uncertain request cannot appear as finished.
//
// The parent mounts this panel only while its order detail is open and keys it by
// order identity. One lifetime abort controller protects its requests. Submission
// freezes a buyer snapshot before dispatch; a failed response triggers a read of
// that same application. An explicit retry retains that snapshot, while a known
// rejection unlocks editing only after a successful read confirms no application.
// Bounded polling performs reads only, pauses while the document is hidden, and
// stops when the panel unmounts.

// The order identity is fixed for this mounted panel; its parent replaces the
// component instance when selecting another order to prevent buyer-state reuse.
const props = defineProps<{ orderId: number /* Selected owned payment order. */ }>()
const { t } = useI18n({ useScope: 'local', messages: { en, 'zh-Hans': zh } })

// Polling lasts at most ten minutes per mounted detail panel; manual refresh
// remains available after this limit and no timer ever submits an application.
const POLL_INTERVAL_MS = 30_000
const MAX_POLLS = 20
// Buyer limits mirror the invoice API boundary, including its delivery email
// byte limit; client checks prevent invalid drafts from becoming locked attempts.
const MAX_TITLE_LENGTH = 160
const MAX_EMAIL_BYTES = 214

// Panel-owned request state survives transient failures, while draft fields are
// replaced by attemptedBuyer as soon as the customer dispatches an application.
const state = ref<TaxInvoiceState | null>(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const downloadURL = ref('')
const attemptedBuyer = ref<TaxInvoiceBuyer | null>(null)
const draft = reactive<TaxInvoiceBuyer>({
  buyer_type: 'INDIVIDUAL',
  title: '',
  tax_id: '',
  email: '',
})

// The lifetime controller and poll timer belong to this mounted order detail.
// disposed rejects late results even when a transport does not honor cancellation.
const controller = new AbortController()
let disposed = false
let polls = 0
let pollTimer: ReturnType<typeof setTimeout> | undefined
// rejectedSubmission records a known validation rejection until an authoritative
// empty read confirms that unlocking buyer editing cannot replace a stored snapshot.
let rejectedSubmission = false

// Derived display state keeps server history visible independently of whether new
// creation is enabled and requires both issuance status and a document number.
const invoice = computed(() => state.value?.invoice)
const savedBuyer = computed(() => invoice.value || attemptedBuyer.value)
const visible = computed(() => !!(state.value?.enabled || savedBuyer.value || error.value))
const hasDocument = computed(() => invoice.value?.status === 'ISSUED' && !!invoice.value.invoice_no)
const canDownload = computed(() => state.value?.enabled && hasDocument.value)
const canRetry = computed(
  () => !invoice.value && !!attemptedBuyer.value && state.value?.enabled && state.value.eligible,
)
const pending = computed(() =>
  invoice.value
    ? ['UNKNOWN', 'PROCESSING', 'REVERSING'].includes(invoice.value.status)
    : !!attemptedBuyer.value,
)
const statusText = computed(() => {
  if (!invoice.value || (invoice.value.status === 'ISSUED' && !hasDocument.value))
    return t('unconfirmed')
  return t(`status.${invoice.value.status}`)
})
const invoiceAmount = computed(() => {
  // The backend returns actual collected CNY cents, independently of credited balance units.
  return ((invoice.value?.amount_cents ?? 0) / 100).toFixed(2)
})

// clearPoll releases the scheduled read while retaining the accumulated poll budget.
function clearPoll() {
  if (pollTimer !== undefined) clearTimeout(pollTimer)
  pollTimer = undefined
}

// schedulePoll schedules one bounded status read for an unresolved application.
// Hidden documents and active operations defer polling; visibility restoration or
// operation completion can schedule another read without creating an invoice.
function schedulePoll() {
  clearPoll()
  if (disposed || document.hidden || busy.value || !pending.value || polls >= MAX_POLLS) return
  pollTimer = setTimeout(() => {
    polls += 1
    void refreshInvoice()
  }, POLL_INTERVAL_MS)
}

// refreshInvoice retrieves the original application and preserves its buyer snapshot.
// Concurrent reads are skipped because one mounted panel owns the current request.
// A failed read retains the previous record and displays a safe error.
//
// A known submission rejection can unlock editing only when this read succeeds
// without an application. A transport failure alone cannot establish that result,
// so its snapshot stays locked for an explicit retry with the same buyer details.
async function refreshInvoice() {
  if (disposed || loading.value) return
  clearPoll()
  loading.value = true
  error.value = ''
  downloadURL.value = ''
  try {
    const reply = await taxInvoicesAPI.read(props.orderId, controller.signal)
    if (!disposed) {
      state.value = reply.data
      if (rejectedSubmission && !reply.data.invoice) {
        // Only this successful empty read corroborates the rejection, preventing
        // a failed status lookup from permitting changes to an accepted buyer snapshot.
        attemptedBuyer.value = null
        rejectedSubmission = false
      }
    }
  } catch {
    // A failed read cannot erase an accepted application or unlock buyer editing,
    // because issuance may still be progressing at the provider.
    if (!disposed) error.value = t('loadError')
  } finally {
    if (!disposed) {
      loading.value = false
      schedulePoll()
    }
  }
}

// submitInvoice submits or explicitly retries one immutable buyer snapshot.
// Existing applications remain read-only because the backend worker recovers
// unknown results using their original request identities. A network error may
// arrive after issuance began, so the customer can only retry the unchanged
// snapshot until a stored application is found. A validation rejection permits
// editing after a successful empty read. Backend idempotency remains authoritative
// across reloads; the browser never generates a replacement request identity.
async function submitInvoice() {
  if (disposed || busy.value || loading.value || !state.value?.enabled || !state.value.eligible)
    return
  if (invoice.value) return
  const snapshot: TaxInvoiceBuyer = attemptedBuyer.value || {
    buyer_type: draft.buyer_type,
    title: draft.title.trim(),
    tax_id: draft.buyer_type === 'ORGANIZATION' ? draft.tax_id.trim() : '',
    email: draft.email.trim(),
  }
  const validTitle =
    snapshot.title.length > 0 &&
    snapshot.title.length <= MAX_TITLE_LENGTH &&
    !/\p{Cc}/u.test(snapshot.title)
  const validEmail =
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(snapshot.email) &&
    new TextEncoder().encode(snapshot.email).length <= MAX_EMAIL_BYTES
  const validTaxID =
    snapshot.buyer_type === 'INDIVIDUAL' || /^[A-Z0-9]{15,20}$/.test(snapshot.tax_id)
  if (!validTitle || !validEmail || !validTaxID) {
    error.value = t('invalidBuyer')
    return
  }
  // Freeze the buyer before dispatch because a lost HTTP response cannot prove
  // the server rejected the application or safely permit different buyer fields.
  attemptedBuyer.value = snapshot
  rejectedSubmission = false
  busy.value = true
  error.value = ''
  clearPoll()
  try {
    const reply = await taxInvoicesAPI.submit(props.orderId, snapshot, controller.signal)
    if (!disposed) state.value = reply.data
  } catch (failure) {
    if (!disposed) {
      const wasRejected =
        typeof failure === 'object' &&
        failure !== null &&
        'status' in failure &&
        failure.status === 400
      rejectedSubmission = wasRejected
      await refreshInvoice()
      if (!disposed && !invoice.value)
        error.value = t(wasRejected && !attemptedBuyer.value ? 'submitRejected' : 'submitError')
    }
  } finally {
    if (!disposed) {
      busy.value = false
      schedulePoll()
    }
  }
}

// requestFile retrieves a fresh document link for a confirmed issued record when
// invoicing remains enabled. The API validates navigation safety. Failed retrieval
// preserves history and reports a safe message without exposing provider details.
async function requestFile() {
  if (disposed || busy.value || loading.value || !canDownload.value) return
  busy.value = true
  error.value = ''
  downloadURL.value = ''
  try {
    const reply = await taxInvoicesAPI.file(props.orderId, controller.signal)
    if (!disposed) downloadURL.value = reply.data.url
  } catch {
    if (!disposed) error.value = t('fileError')
  } finally {
    if (!disposed) busy.value = false
  }
}

// visibilityChanged pauses reads in hidden browser documents and resumes bounded
// polling when visible. Returning to the page does not resubmit buyer details
// or reset the mounted panel's polling budget.
function visibilityChanged() {
  schedulePoll()
}

onMounted(() => {
  document.addEventListener('visibilitychange', visibilityChanged)
  void refreshInvoice()
})

onBeforeUnmount(() => {
  // Mark disposal before cancellation so synchronous abort callbacks and later
  // promise completions cannot publish state or schedule work for a closed panel.
  disposed = true
  controller.abort()
  clearPoll()
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>
