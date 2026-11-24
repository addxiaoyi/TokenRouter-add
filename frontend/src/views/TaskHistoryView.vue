<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  readTaskHistory,
  type TaskHistoryFilter,
  type TaskStatus,
  type TaskSummary,
} from '@/api/taskHistory'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAuthStore } from '@/stores'
import { formatDateTime } from '@/utils/format'

// TaskHistoryView.vue presents persisted batch jobs as newest-first rows with
// literal batch, status, and administrator submitter filters. The account route
// shows jobs across the authenticated user's keys. The administrator route can
// filter a known submitter ID, and the API determines which scope is allowed.
//
// Changing account, scope, filters, or page cancels the earlier read. The view
// clears records before starting a request and commits the new filters and
// cursor only together with that response. Retrying a failed read uses the
// attempted query instead of trying to infer it from controls.

const { t } = useI18n()
const route = useRoute()
const auth = useAuthStore()

// Status filters reflect persisted stages so reconciliation and payment work
// can be distinguished from provider execution in the history list.
const statuses: TaskStatus[] = [
  'created',
  'uploading',
  'submitted',
  'running',
  'indexing',
  'settling',
  'completed',
  'failed',
  'cancelled',
  'output_deleted',
]
const statusOptions = computed(() => [
  { value: '', label: t('taskHistory.allStatuses') },
  ...statuses.map((status) => ({ value: status, label: t(`taskHistory.statuses.${status}`) })),
])
const scope = computed(() => (route.meta.requiresAdmin ? 'admin' : 'account'))

// HistoryQuery keeps the applied filter and breadcrumb of previous cursors
// together so a newer page can be retried independently of unsaved filters.
interface HistoryQuery {
  filters: TaskHistoryFilter // Applied filters for every page.
  cursors: (string | undefined)[] // Current cursor follows earlier visited cursors.
}

// Draft filters are applied explicitly, while the rendered rows and cursor
// always correspond to a single attempted query in the current account.
const batchID = ref('')
const userID = ref('')
const status = ref<TaskStatus | ''>('')
const items = ref<TaskSummary[]>([])
const nextCursor = ref<string>()
const query = ref<HistoryQuery>({ filters: {}, cursors: [undefined] })
const loading = ref(true)
const failed = ref(false)
let request: AbortController | undefined

// applyFilters starts a fresh page with a snapshot of the current inputs.
function applyFilters() {
  void load({
    filters: {
      batch_id: batchID.value.trim() || undefined,
      status: status.value || undefined,
      user_id: scope.value === 'admin' ? userID.value.trim() || undefined : undefined,
    },
    cursors: [undefined],
  })
}

// load replaces the displayed query and begins its read. An abort may race
// with a response already being delivered, so only the current controller
// may publish returned rows or update loading and error state.
async function load(nextQuery: HistoryQuery) {
  request?.abort()
  const current = new AbortController()
  request = current
  query.value = nextQuery
  items.value = []
  nextCursor.value = undefined
  failed.value = false
  loading.value = true
  try {
    const history = await readTaskHistory(
      scope.value,
      { ...nextQuery.filters, cursor: nextQuery.cursors[nextQuery.cursors.length - 1] },
      current.signal,
    )
    if (request !== current || current.signal.aborted) return
    items.value = history.items
    nextCursor.value = history.next_cursor
  } catch {
    if (request === current && !current.signal.aborted) failed.value = true
  } finally {
    if (request === current && !current.signal.aborted) loading.value = false
  }
}

// refresh repeats the displayed query, including a failed page request.
function refresh() {
  void load(query.value)
}

// nextPage carries the server's cursor forward with the applied filters.
function nextPage() {
  if (!nextCursor.value) return
  void load({ ...query.value, cursors: [...query.value.cursors, nextCursor.value] })
}

// previousPage uses the cursor saved when the previous page was read.
function previousPage() {
  if (query.value.cursors.length <= 1) return
  void load({ ...query.value, cursors: query.value.cursors.slice(0, -1) })
}

// statusTone adds a color cue while the text remains the status identifier.
function statusTone(value: TaskStatus): string {
  switch (value) {
    case 'completed':
      return 'text-green-700 dark:text-green-400'
    case 'failed':
      return 'text-red-700 dark:text-red-400'
    case 'output_deleted':
    case 'cancelled':
      return 'text-gray-500 dark:text-gray-400'
    default:
      return 'text-amber-700 dark:text-amber-400'
  }
}

// Reset private filters and rows synchronously when the account or route scope
// changes. Abort the active request at unmount so it cannot publish late data.
watch(
  [scope, () => auth.user?.id],
  () => {
    batchID.value = ''
    userID.value = ''
    status.value = ''
    applyFilters()
  },
  { immediate: true, flush: 'sync' },
)
onBeforeUnmount(() => request?.abort())
</script>

<template>
  <AppLayout>
    <section class="space-y-4" :aria-busy="loading">
      <form class="card flex flex-wrap items-end gap-3 p-4 sm:p-5" @submit.prevent="applyFilters">
        <label class="min-w-0 flex-1 basis-48">
          <span class="mb-1.5 block text-sm font-medium">{{ t('taskHistory.batchID') }}</span>
          <input v-model="batchID" class="input w-full" type="search" maxlength="64" />
        </label>
        <label v-if="scope === 'admin'" class="min-w-0 flex-1 basis-32">
          <span class="mb-1.5 block text-sm font-medium">{{ t('taskHistory.userID') }}</span>
          <input
            v-model="userID"
            class="input w-full"
            type="text"
            inputmode="numeric"
            pattern="[1-9][0-9]*"
            maxlength="19"
          />
        </label>
        <div class="min-w-0 flex-1 basis-40">
          <span class="mb-1.5 block text-sm font-medium">{{ t('taskHistory.status') }}</span>
          <Select v-model="status" :options="statusOptions" :aria-label="t('taskHistory.status')" />
        </div>
        <button class="btn btn-primary" type="submit" :disabled="loading">
          <Icon name="search" size="sm" />
          {{ t('taskHistory.apply') }}
        </button>
        <button class="btn btn-secondary" type="button" :disabled="loading" @click="refresh">
          <Icon name="refresh" size="sm" />
          {{ t('taskHistory.refresh') }}
        </button>
      </form>

      <p v-if="loading" role="status" class="card p-5 text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </p>
      <div v-else-if="failed" role="alert" class="card flex flex-wrap items-center gap-3 p-5">
        <p>{{ t('taskHistory.failed') }}</p>
        <button class="btn btn-secondary" type="button" @click="refresh">{{
          t('taskHistory.retry')
        }}</button>
      </div>
      <div v-else-if="items.length === 0" class="card">
        <EmptyState
          :title="t('taskHistory.empty')"
          :description="
            t(
              Object.values(query.filters).some(Boolean)
                ? 'taskHistory.emptyFiltered'
                : 'taskHistory.emptyHint',
            )
          "
        />
      </div>
      <ul v-else class="card divide-y divide-gray-200 dark:divide-dark-700">
        <li v-for="item in items" :key="item.id" class="space-y-3 px-4 py-4 sm:px-5">
          <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
            <div class="min-w-0">
              <h2 class="break-words font-semibold">{{
                item.model || t('taskHistory.unknownModel')
              }}</h2>
              <p class="break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{
                item.id
              }}</p>
            </div>
            <span :class="['text-sm font-medium', statusTone(item.status)]">
              {{ t(`taskHistory.statuses.${item.status}`) }}
            </span>
          </div>
          <dl class="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{
                t('taskHistory.itemCount')
              }}</dt>
              <dd class="tabular-nums">{{ item.item_count }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{
                t('taskHistory.successCount')
              }}</dt>
              <dd class="tabular-nums">{{ item.success_count }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{
                t('taskHistory.failureCount')
              }}</dt>
              <dd class="tabular-nums">{{ item.failure_count }}</dd>
            </div>
            <div>
              <dt class="text-xs text-gray-500 dark:text-gray-400">{{
                t('taskHistory.cancelledCount')
              }}</dt>
              <dd class="tabular-nums">{{ item.cancelled_count }}</dd>
            </div>
          </dl>
          <dl class="flex flex-wrap gap-x-5 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
            <div class="flex flex-wrap gap-x-1">
              <dt>{{ t('taskHistory.submittedAt') }}</dt>
              <dd
                ><time :datetime="item.created_at">{{ formatDateTime(item.created_at) }}</time></dd
              >
            </div>
            <div v-if="item.finished_at" class="flex flex-wrap gap-x-1">
              <dt>{{ t('taskHistory.finishedAt') }}</dt>
              <dd
                ><time :datetime="item.finished_at">{{
                  formatDateTime(item.finished_at)
                }}</time></dd
              >
            </div>
            <div v-if="item.user_id != null" class="flex flex-wrap gap-x-1">
              <dt>{{ t('taskHistory.userID') }}</dt>
              <dd>{{ item.user_id }}</dd>
            </div>
          </dl>
        </li>
      </ul>
      <nav
        v-if="!loading && !failed && (query.cursors.length > 1 || nextCursor)"
        class="flex flex-wrap items-center justify-between gap-2"
        :aria-label="t('taskHistory.pagination')"
      >
        <button
          class="btn btn-secondary"
          type="button"
          :disabled="query.cursors.length <= 1"
          @click="previousPage"
          >{{ t('taskHistory.previous') }}</button
        >
        <span class="text-sm">{{ t('taskHistory.page', { number: query.cursors.length }) }}</span>
        <button class="btn btn-secondary" type="button" :disabled="!nextCursor" @click="nextPage">{{
          t('taskHistory.next')
        }}</button>
      </nav>
    </section>
  </AppLayout>
</template>
