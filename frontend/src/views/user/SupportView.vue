<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import axios from 'axios'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores'
import {
  supportAPI,
  type SupportPage,
  type SupportSettings,
  type SupportThread,
  type TicketStatus,
} from '@/api/support'

// SupportView.vue provides owned conversations and administrator support workflows.
// Lists and messages use bounded server pages; individual threads preserve plain
// text and author roles. Route and account changes invalidate outstanding reads
// so a delayed private response cannot populate a different conversation.
//
// Submission UUIDs survive failed requests and reset only after success. A draft
// stays editable after an error, while busy controls prevent accidental duplicate
// actions. Ticket status updates include the displayed revision and expose server
// conflicts instead of overwriting an operator's newer state.

// Route, account, and translation dependencies establish this page's private scope.
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()
const staff = computed(() => route.meta.requiresAdmin === true)
const base = computed(() => (staff.value ? '/admin/support-tickets' : '/support-tickets'))
const ticketID = computed(() => Number(route.params.id) || 0)
const pageNumber = computed(() => Math.max(1, Number(route.query.page) || 1))
const api = computed(() => supportAPI(staff.value))

// Remote records belong to the current generation and are cleared on scope changes.
const tickets = ref<SupportPage | null>(null)
const thread = ref<SupportThread | null>(null)
const loading = ref(true)
const error = ref('')
const busy = ref(false)
let generation = 0
const search = ref('')
const status = ref('')
const statusValues: TicketStatus[] = ['open', 'in_progress', 'resolved', 'closed']
const statusOptions = computed(() => [
  { value: '', label: t('support.all') },
  ...statusValues.map((value) => ({ value, label: t(`support.status.${value}`) })),
])

// Drafts keep their retry IDs until persistence succeeds; email settings are staff-only.
const creating = ref(false)
const subject = ref('')
const description = ref('')
const createID = ref(crypto.randomUUID())
const reply = ref('')
const replyID = ref(crypto.randomUUID())
const draftError = ref('')
const settingsOpen = ref(false)
const settings = ref<SupportSettings | null>(null)
const recipients = ref('')
const settingsError = ref('')
const notice = ref('')

// errorText exposes server validation messages without displaying stack traces.
function errorText(cause: unknown, fallback: string): string {
  if (axios.isAxiosError(cause) && typeof cause.response?.data?.message === 'string')
    return cause.response.data.message
  return t(fallback)
}

// formatTime formats a server timestamp in the viewer's local date and time.
function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

// load reads the current conversation or filtered page and rejects stale completion.
// Navigation and account changes advance the generation before starting a new
// read. Failed requests expose recovery controls, and private records are cleared
// before a different route or owner can render them.
async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  tickets.value = null
  thread.value = null
  try {
    if (ticketID.value) {
      const loaded = await api.value.thread(ticketID.value)
      if (current === generation) thread.value = loaded
    } else {
      const loaded = await api.value.list({
        page: pageNumber.value,
        status: String(route.query.status ?? ''),
        search: String(route.query.search ?? ''),
      })
      if (current === generation) tickets.value = loaded
    }
  } catch (cause) {
    if (current === generation) error.value = errorText(cause, 'support.loadFailed')
  } finally {
    if (current === generation) loading.value = false
  }
}

// applyFilters starts at the first page whenever search or workflow filters change.
function applyFilters() {
  void router.push({
    path: base.value,
    query: { search: search.value.trim() || undefined, status: status.value || undefined },
  })
}

// openTicket navigates to the conversation selected from the authorized list.
function openTicket(id: number) {
  void router.push(`${base.value}/${id}`)
}

// backToList returns to the current role's private ticket listing.
function backToList() {
  void router.push(base.value)
}

// turnPage preserves filters while changing the bounded list's current page.
function turnPage(offset: number) {
  void router.push({ path: base.value, query: { ...route.query, page: pageNumber.value + offset } })
}

// openCreate opens the owned-ticket form without discarding a failed submission draft.
function openCreate() {
  creating.value = true
  draftError.value = ''
}

// closeCreate dismisses the form only when no submission is in flight.
function closeCreate() {
  if (!busy.value) creating.value = false
}

// createTicket persists a draft and opens its committed conversation on success.
// Failed submissions retain text and UUID, making an interrupted response safe to
// retry. A route change during submission prevents stale form state publication.
async function createTicket() {
  if (busy.value) return
  if (subject.value.trim().length < 2 || !description.value.trim()) {
    draftError.value = t('support.required')
    return
  }
  busy.value = true
  draftError.value = ''
  const current = generation
  try {
    const saved = await api.value.create(subject.value, description.value, createID.value)
    if (current !== generation) return
    creating.value = false
    subject.value = ''
    description.value = ''
    createID.value = crypto.randomUUID()
    await router.push(`/support-tickets/${saved.ticket.id}`)
  } catch (cause) {
    if (current === generation) draftError.value = errorText(cause, 'support.saveFailed')
  } finally {
    busy.value = false
  }
}

// sendReply appends the current draft while preserving it after failed requests.
// Success resets only that reply's text and retry identity, then reloads the
// committed conversation. Closed threads require an explicit reopen action.
async function sendReply() {
  if (busy.value || !reply.value.trim() || !thread.value) return
  busy.value = true
  draftError.value = ''
  const current = generation
  try {
    await api.value.reply(thread.value.ticket.id, reply.value, replyID.value)
    if (current !== generation) return
    reply.value = ''
    replyID.value = crypto.randomUUID()
    await load()
  } catch (cause) {
    if (current === generation) draftError.value = errorText(cause, 'support.saveFailed')
  } finally {
    busy.value = false
  }
}

// changeStatus sends the displayed revision and refreshes committed workflow state.
// Conflicts remain visible with the reply draft intact; a subsequent refresh
// lets the operator make another decision against current state.
async function changeStatus(next: TicketStatus) {
  if (busy.value || !thread.value) return
  const current = generation
  busy.value = true
  draftError.value = ''
  try {
    await api.value.status(thread.value.ticket, next)
    if (current !== generation) return
    await load()
  } catch (cause) {
    if (current === generation) draftError.value = errorText(cause, 'support.saveFailed')
  } finally {
    busy.value = false
  }
}

// loadMore appends one chronological message page to the currently open thread.
// Generation checks discard responses for a different conversation, and IDs
// prevent duplicate rows if the server page overlaps a recently loaded response.
async function loadMore() {
  if (busy.value || !thread.value?.has_more) return
  const current = generation
  busy.value = true
  try {
    const next = await api.value.thread(thread.value.ticket.id, thread.value.next_id)
    if (current !== generation || !thread.value) return
    const known = new Set(thread.value.messages.map((message) => message.id))
    thread.value = {
      ...next,
      messages: [
        ...thread.value.messages,
        ...next.messages.filter((message) => !known.has(message.id)),
      ],
    }
  } catch (cause) {
    if (current === generation) draftError.value = errorText(cause, 'support.loadFailed')
  } finally {
    busy.value = false
  }
}

// openSettings loads staff recipients before making the notification form editable.
// Failure leaves the form disabled with a visible retry path, preventing an
// unread configuration from being accidentally replaced with empty values.
async function openSettings() {
  const current = generation
  settingsOpen.value = true
  settings.value = null
  settingsError.value = ''
  notice.value = ''
  try {
    const loaded = await api.value.settings()
    if (current !== generation) return
    settings.value = loaded
    recipients.value = loaded.recipients.join('\n')
  } catch (cause) {
    if (current === generation) settingsError.value = errorText(cause, 'support.loadFailed')
  }
}

// closeSettings dismisses notification configuration when no write is in flight.
function closeSettings() {
  if (!busy.value) settingsOpen.value = false
}

// saveSettings persists the staff list without changing already queued recipients.
async function saveSettings() {
  if (busy.value || !settings.value) return
  const current = generation
  busy.value = true
  settingsError.value = ''
  try {
    await api.value.saveSettings(
      recipients.value
        .split(/[\n,;]+/)
        .map((value) => value.trim())
        .filter(Boolean),
    )
    if (current !== generation) return
    notice.value = t('support.savedSettings')
  } catch (cause) {
    if (current === generation) settingsError.value = errorText(cause, 'support.saveFailed')
  } finally {
    busy.value = false
  }
}

// retryDelivery reschedules failed notices after the operator repairs mail configuration.
async function retryDelivery() {
  if (busy.value) return
  const current = generation
  const client = api.value
  busy.value = true
  settingsError.value = ''
  try {
    await client.retryDelivery()
    if (current !== generation) return
    const loaded = await client.settings()
    if (current !== generation) return
    settings.value = loaded
    notice.value = t('support.retryQueued')
  } catch (cause) {
    if (current === generation) settingsError.value = errorText(cause, 'support.saveFailed')
  } finally {
    busy.value = false
  }
}

// Scope changes clear conversation drafts before loading the next account or route.
watch(
  () => [route.fullPath, auth.user?.id],
  (scope, previous) => {
    search.value = String(route.query.search ?? '')
    status.value = String(route.query.status ?? '')
    reply.value = ''
    replyID.value = crypto.randomUUID()
    draftError.value = ''
    settingsOpen.value = false
    settings.value = null
    settingsError.value = ''
    notice.value = ''
    if (scope[1] !== previous?.[1]) {
      creating.value = false
      subject.value = ''
      description.value = ''
      createID.value = crypto.randomUUID()
      recipients.value = ''
    }
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
})
</script>

<template>
  <AppLayout>
    <section class="space-y-4" :aria-busy="loading">
      <div
        v-if="error"
        role="alert"
        class="card flex flex-wrap items-center justify-between gap-3 p-4"
      >
        <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        <button class="btn btn-secondary" @click="load">{{ t('support.retry') }}</button>
      </div>
      <div v-if="!ticketID" class="card p-4 sm:p-5">
        <form class="flex flex-wrap items-center gap-3" @submit.prevent="applyFilters">
          <label class="min-w-0 flex-1 basis-56">
            <span class="sr-only">{{ t('support.search') }}</span>
            <input
              v-model="search"
              class="input w-full"
              :placeholder="t('support.search')"
              maxlength="100"
            />
          </label>
          <label>
            <span class="sr-only">{{ t('support.all') }}</span>
            <Select
              v-model="status"
              :options="statusOptions"
              class="w-36"
              @update:model-value="applyFilters"
            />
          </label>
          <button class="btn btn-secondary" type="submit"
            ><Icon name="search" size="sm" />{{ t('support.filter') }}</button
          >
          <button v-if="staff" class="btn btn-secondary" type="button" @click="openSettings"
            ><Icon name="cog" size="sm" />{{ t('support.settings') }}</button
          >
          <button v-else class="btn btn-primary" type="button" @click="openCreate"
            ><Icon name="plus" size="sm" />{{ t('support.newTicket') }}</button
          >
        </form>
      </div>
      <div
        v-if="loading"
        class="card space-y-4 p-5"
        role="status"
        :aria-label="t('common.loading')"
      >
        <div
          v-for="index in 4"
          :key="index"
          class="h-16 rounded-surface skeleton"
          aria-hidden="true"
        ></div>
      </div>
      <template v-else-if="tickets">
        <div v-if="!tickets.items.length" class="card px-6 py-16 text-center">
          <Icon name="ticket" size="xl" class="mx-auto text-gray-400" />
          <h2 class="mt-4 font-semibold">{{
            search || status ? t('support.noMatches') : t('support.empty')
          }}</h2>
          <p class="mt-2 text-sm text-gray-500">{{ t('support.emptyHint') }}</p>
        </div>
        <ul v-else class="card divide-y divide-gray-100 dark:divide-dark-700">
          <li v-for="ticket in tickets.items" :key="ticket.id">
            <button
              type="button"
              class="w-full p-4 text-left transition-colors hover:bg-gray-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:bg-dark-800 sm:p-5"
              @click="openTicket(ticket.id)"
            >
              <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500">
                <span class="font-mono">#{{ ticket.id }}</span>
                <span
                  class="rounded-compact bg-gray-100 px-2 py-1 font-medium text-gray-700 dark:bg-dark-800 dark:text-gray-300"
                  >{{ t(`support.status.${ticket.status}`) }}</span
                >
                <span v-if="staff">{{ ticket.username || `#${ticket.user_id}` }}</span>
                <span class="sm:ml-auto">{{
                  t('support.updated', { time: formatTime(ticket.updated_at) })
                }}</span>
              </div>
              <h2 class="mt-2 break-words font-semibold text-gray-900 dark:text-white">{{
                ticket.subject
              }}</h2>
              <p class="mt-2 text-xs text-gray-500">{{
                t('support.replies', { count: ticket.message_count })
              }}</p>
            </button>
          </li>
        </ul>
        <nav
          v-if="tickets.total > tickets.page_size"
          class="flex items-center justify-between gap-3"
          :aria-label="t('support.page', { page: pageNumber })"
        >
          <button class="btn btn-secondary" :disabled="pageNumber <= 1" @click="turnPage(-1)">{{
            t('support.previous')
          }}</button>
          <span class="text-sm text-gray-500">{{ t('support.page', { page: pageNumber }) }}</span>
          <button
            class="btn btn-secondary"
            :disabled="pageNumber * tickets.page_size >= tickets.total"
            @click="turnPage(1)"
            >{{ t('support.next') }}</button
          >
        </nav>
      </template>
      <template v-else-if="thread">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <button class="btn btn-secondary" @click="backToList"
            ><Icon name="arrowLeft" size="sm" />{{ t('support.back') }}</button
          >
          <button class="btn btn-secondary" :disabled="busy" @click="load"
            ><Icon name="refresh" size="sm" />{{ t('support.refresh') }}</button
          >
        </div>
        <header class="card space-y-4 p-5 sm:p-6">
          <div class="flex flex-wrap items-center gap-2 text-sm text-gray-500">
            <span>#{{ thread.ticket.id }}</span
            ><span>{{ t(`support.status.${thread.ticket.status}`) }}</span>
            <span v-if="thread.ticket.legacy_id">{{
              t('support.legacy', { id: thread.ticket.legacy_id })
            }}</span>
          </div>
          <h2 class="break-words text-xl font-semibold">{{ thread.ticket.subject }}</h2>
          <p class="text-xs text-gray-500"
            >{{
              staff
                ? thread.ticket.username || `#${thread.ticket.user_id}`
                : t('support.privateHint')
            }}
            · {{ t('support.created', { time: formatTime(thread.ticket.created_at) }) }}</p
          >
          <div class="flex flex-wrap gap-2">
            <button
              v-if="staff && thread.ticket.status === 'open'"
              class="btn btn-secondary"
              :disabled="busy"
              @click="changeStatus('in_progress')"
              >{{ t('support.progress') }}</button
            >
            <button
              v-if="staff && !['resolved', 'closed'].includes(thread.ticket.status)"
              class="btn btn-primary"
              :disabled="busy"
              @click="changeStatus('resolved')"
              >{{ t('support.resolve') }}</button
            >
            <button
              v-if="['resolved', 'closed'].includes(thread.ticket.status)"
              class="btn btn-secondary"
              :disabled="busy"
              @click="changeStatus('open')"
              >{{ t('support.reopen') }}</button
            >
            <button
              v-if="thread.ticket.status !== 'closed'"
              class="btn btn-secondary"
              :disabled="busy"
              @click="changeStatus('closed')"
              >{{ t('support.close') }}</button
            >
          </div>
        </header>
        <ol
          class="space-y-3"
          :aria-label="t('support.replies', { count: thread.ticket.message_count })"
        >
          <li v-for="message in thread.messages" :key="message.id" class="card p-5 sm:p-6">
            <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-gray-500">
              <span class="font-semibold" :class="message.is_staff ? 'text-primary-500' : ''">{{
                message.is_staff ? t('support.staff') : message.author || t('support.owner')
              }}</span>
              <time :datetime="message.created_at">{{ formatTime(message.created_at) }}</time>
            </div>
            <p
              class="mt-3 whitespace-pre-wrap break-words text-sm leading-7 text-gray-700 dark:text-gray-200"
              >{{ message.body }}</p
            >
          </li>
        </ol>
        <button
          v-if="thread.has_more"
          class="btn btn-secondary"
          :disabled="busy"
          @click="loadMore"
          >{{ t('support.loadMore') }}</button
        >
        <form class="card space-y-3 p-5 sm:p-6" @submit.prevent="sendReply">
          <p v-if="draftError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{
            draftError
          }}</p>
          <p v-if="thread.ticket.status === 'closed'" class="text-sm text-gray-500">{{
            t('support.closedHint')
          }}</p>
          <template v-else>
            <label for="support-reply" class="block text-sm font-semibold">{{
              t('support.reply')
            }}</label>
            <textarea
              id="support-reply"
              v-model="reply"
              class="input min-h-36 w-full resize-y"
              maxlength="10000"
              required
              :placeholder="t('support.replyPlaceholder')"
            ></textarea>
            <div class="flex justify-end"
              ><button type="submit" class="btn btn-primary" :disabled="busy">{{
                busy ? t('support.sending') : t('support.sendReply')
              }}</button></div
            >
          </template>
        </form>
      </template>
    </section>

    <BaseDialog
      :show="creating"
      :title="t('support.newTicket')"
      width="wide"
      trap-focus
      @close="closeCreate"
    >
      <form class="space-y-4" @submit.prevent="createTicket">
        <p v-if="draftError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{
          draftError
        }}</p>
        <div
          ><label for="support-subject" class="mb-2 block text-sm font-medium">{{
            t('support.subject')
          }}</label
          ><input
            id="support-subject"
            v-model="subject"
            class="input w-full"
            minlength="2"
            maxlength="160"
            required
            :placeholder="t('support.subjectPlaceholder')"
        /></div>
        <div
          ><label for="support-description" class="mb-2 block text-sm font-medium">{{
            t('support.message')
          }}</label
          ><textarea
            id="support-description"
            v-model="description"
            class="input min-h-48 w-full resize-y"
            maxlength="10000"
            required
            :placeholder="t('support.messagePlaceholder')"
          ></textarea>
        </div>
        <div class="flex justify-end gap-3"
          ><button class="btn btn-secondary" type="button" :disabled="busy" @click="closeCreate">{{
            t('support.cancel')
          }}</button
          ><button class="btn btn-primary" type="submit" :disabled="busy">{{
            busy ? t('support.submitting') : t('support.submit')
          }}</button></div
        >
      </form>
    </BaseDialog>

    <BaseDialog
      :show="settingsOpen"
      :title="t('support.settings')"
      trap-focus
      @close="closeSettings"
    >
      <div class="space-y-4">
        <p v-if="settingsError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{
          settingsError
        }}</p>
        <button v-if="settingsError && !settings" class="btn btn-secondary" @click="openSettings">{{
          t('support.retry')
        }}</button>
        <p v-if="notice" role="status" class="text-sm">{{ notice }}</p>
        <form v-if="settings" class="space-y-4" @submit.prevent="saveSettings">
          <label for="support-recipients" class="block text-sm font-medium">{{
            t('support.recipients')
          }}</label>
          <textarea
            id="support-recipients"
            v-model="recipients"
            class="input min-h-28 w-full"
            aria-describedby="support-recipients-help"
          ></textarea>
          <p id="support-recipients-help" class="text-xs leading-5 text-gray-500">{{
            t('support.recipientsHint')
          }}</p>
          <div class="flex flex-wrap gap-4 text-sm"
            ><span>{{ t('support.pending') }} {{ settings.delivery.pending }}</span
            ><span>{{ t('support.failed') }} {{ settings.delivery.failed }}</span
            ><span>{{ t('support.delivered') }} {{ settings.delivery.delivered }}</span></div
          >
          <p class="text-xs leading-5 text-gray-500">{{ t('support.emailHint') }}</p>
          <div class="flex flex-wrap justify-end gap-2"
            ><button
              v-if="settings.delivery.failed"
              class="btn btn-secondary"
              type="button"
              :disabled="busy"
              @click="retryDelivery"
              >{{ t('support.retryDelivery') }}</button
            ><button class="btn btn-primary" type="submit" :disabled="busy">{{
              busy ? t('support.processing') : t('support.save')
            }}</button></div
          >
        </form>
        <div
          v-else-if="!settingsError"
          class="h-40 skeleton rounded-surface"
          role="status"
          :aria-label="t('common.loading')"
        ></div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>
