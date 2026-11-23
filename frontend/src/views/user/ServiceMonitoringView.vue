<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupMonitorCard from '@/components/common/GroupMonitorCard.vue'
import { useAuthStore } from '@/stores'
import { readMonitoring, type MonitoringSnapshot } from '@/api/monitoring'
import { monitoringStatus, trendSegments, type MonitoringStatus } from '@/utils/monitoring'

// ServiceMonitoringView.vue presents authorized group health from observed requests.
// Window changes replace measurements, while periodic refresh failures retain
// the last successful snapshot with an explicit warning. No-sample groups remain
// distinct from healthy traffic and do not display fabricated latency values.
//
// Request generations protect the page from stale responses after account or
// window changes. A single owned timer refreshes visible data without overlapping
// requests and is cleared on unmount. Detail charts preserve missing intervals as
// gaps and show the same bounded model observations as the group's summary.

// Filters and remote data belong to this mounted authenticated monitoring page.
const { t } = useI18n()
const auth = useAuthStore()
const hours = ref(24)
const search = ref('')
const status = ref('all')
const sort = ref('requests')
const cacheHelp = ref(false)
const snapshot = ref<MonitoringSnapshot | null>(null)
const loading = ref(false)
const failed = ref(false)
const selectedID = ref<number | null>(null)
let generation = 0
let timer: ReturnType<typeof setInterval> | undefined
const windows = computed(() =>
  [1, 24, 168].map((value) => ({ value, label: t(`monitoring.hours${value}`) })),
)
const states: MonitoringStatus[] = ['healthy', 'degraded', 'incident', 'idle']
const statusOptions = computed(() => [
  { value: 'all', label: t('monitoring.all') },
  ...states.map((value) => ({ value, label: t(`monitoring.${value}`) })),
])
const groups = computed(() =>
  (snapshot.value?.groups ?? [])
    .filter(
      (group) =>
        `${group.name} ${group.description ?? ''}`
          .toLocaleLowerCase()
          .includes(search.value.toLocaleLowerCase()) &&
        (status.value === 'all' || monitoringStatus(group) === status.value),
    )
    .sort((left, right) => {
      // metric chooses one comparable observation; unavailable values remain last
      // so a group without traffic never outranks a measured zero latency or rate.
      const metric = (group: typeof left) =>
        sort.value === 'availability'
          ? group.success_rate
          : sort.value === 'latency'
            ? group.avg_duration_ms
            : sort.value === 'cache'
              ? group.cache_hit_rate
              : group.success + group.errors
      const a = metric(left)
      const b = metric(right)
      if (a == null) return b == null ? left.id - right.id : 1
      if (b == null) return -1
      return (sort.value === 'latency' ? a - b : b - a) || left.id - right.id
    }),
)
const selected = computed(() =>
  snapshot.value?.groups.find((group) => group.id === selectedID.value),
)
const segments = computed(() =>
  selected.value && snapshot.value
    ? trendSegments(
        selected.value.trend,
        snapshot.value.start,
        snapshot.value.end,
        snapshot.value.bucket_minutes,
      )
    : [],
)
// Status counts describe the entire authorized snapshot independently of search.
// Sorting applies only to displayed groups and keeps unavailable metrics last.
const counts = computed(() =>
  Object.fromEntries(
    states.map((state) => [
      state,
      snapshot.value?.groups.filter((group) => monitoringStatus(group) === state).length ?? 0,
    ]),
  ),
)
const sortOptions = computed(() =>
  ['requests', 'availability', 'latency', 'cache'].map((value) => ({
    value,
    label: t('monitoring.sort.' + value),
  })),
)

// duration converts observed milliseconds to seconds while retaining absent timing explicitly.
function duration(value: number | null): string {
  return value == null ? '—' : `${(value / 1000).toFixed(2)}s`
}

// refresh updates the current window and suppresses responses from earlier scopes.
// The caller clears stale measurements before changing window or account. Same-
// scope failures retain the last snapshot and show its timestamp with a warning.
// The generation owner alone may publish a response or clear loading state;
// an account change or a new observation window increments that owner first.
// This keeps a delayed private response from replacing the currently authorized
// groups while allowing ordinary refresh failures to retain their last observation.
async function refresh() {
  if (loading.value) return
  const current = ++generation
  loading.value = true
  failed.value = false
  try {
    const loaded = await readMonitoring(hours.value)
    if (current === generation) snapshot.value = loaded
  } catch {
    if (current === generation) failed.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

// openGroup selects a visible group for detailed observed trends and model activity.
function openGroup(id: number) {
  selectedID.value = id
}

// closeGroup returns focus to the monitoring page after reading group details.
function closeGroup() {
  selectedID.value = null
}

// Scope replacement invalidates previous reads before starting another private request.
watch(
  () => [hours.value, auth.user?.id],
  () => {
    generation++
    loading.value = false
    snapshot.value = null
    selectedID.value = null
    void refresh()
  },
  { immediate: true, flush: 'sync' },
)
// Poll only visible pages to bound background requests; the mounted page owns
// this timer and releases it when navigation removes the monitoring view.
onMounted(() => {
  timer = setInterval(() => {
    if (!document.hidden) void refresh()
  }, 60000)
})
onBeforeUnmount(() => {
  // Reject late request completion before releasing the timer so another account
  // or navigation cannot receive this mounted page's delayed private snapshot.
  generation++
  if (timer) clearInterval(timer)
})
</script>

<template>
  <AppLayout hide-page-heading class="monitoring-layout">
    <section class="monitoring-page" :aria-busy="loading">
      <header class="monitoring-heading">
        <div
          ><h1>{{ t('monitoring.title') }}</h1
          ><p>{{ t('monitoring.description') }}</p></div
        >
        <div class="monitoring-header-actions">
          <div class="monitoring-window" :aria-label="t('monitoring.window')">
            <button
              v-for="window in windows"
              :key="window.value"
              type="button"
              :aria-pressed="hours === window.value"
              @click="hours = window.value"
              >{{ window.label }}</button
            >
          </div>
          <button type="button" class="monitoring-refresh" :disabled="loading" @click="refresh"
            ><Icon name="refresh" size="sm" />{{ t('monitoring.refresh') }}</button
          >
        </div>
      </header>
      <div class="monitoring-toolbar">
        <p>{{ t('monitoring.scope') }}</p>
        <time v-if="snapshot" :datetime="snapshot.end">{{
          t('monitoring.updated', { time: new Date(snapshot.end).toLocaleTimeString() })
        }}</time>
      </div>
      <div class="monitoring-controls">
        <label class="monitoring-search"
          ><Icon name="search" size="sm" /><span class="sr-only">{{ t('monitoring.search') }}</span
          ><input v-model="search" :placeholder="t('monitoring.search')"
        /></label>
        <Select
          v-model="status"
          :options="statusOptions"
          :aria-label="t('monitoring.all')"
          class="monitoring-select"
        />
        <Select
          v-model="sort"
          :options="sortOptions"
          :aria-label="t('monitoring.sortLabel')"
          class="monitoring-select"
        />
        <button type="button" class="monitoring-text-button" @click="cacheHelp = true">{{
          t('monitoring.cacheExplanation')
        }}</button>
      </div>
      <div v-if="snapshot?.enabled" class="monitoring-legend">
        <span v-for="state in states" :key="state" :data-status="state"
          ><i aria-hidden="true"></i>{{ counts[state] }} {{ t('monitoring.' + state) }}</span
        >
        <span>{{ t('monitoring.groupCount', { count: snapshot.groups.length }) }}</span>
      </div>
      <div v-if="failed" role="alert" class="monitoring-error"
        ><p>{{ t('monitoring.refreshFailed') }}</p
        ><button type="button" class="btn btn-secondary" @click="refresh">{{
          t('monitoring.retry')
        }}</button></div
      >
      <div
        v-if="loading && !snapshot"
        class="monitoring-groups"
        role="status"
        :aria-label="t('common.loading')"
        ><div v-for="index in 6" :key="index" class="monitoring-placeholder"
          ><div class="skeleton h-80" aria-hidden="true"></div></div
      ></div>
      <div v-else-if="snapshot && !snapshot.enabled" class="monitoring-empty"
        ><h2>{{ t('monitoring.disabled') }}</h2
        ><p>{{ t('monitoring.disabledHint') }}</p></div
      >
      <template v-else-if="snapshot">
        <p v-if="!groups.length" class="monitoring-empty">{{ t('monitoring.empty') }}</p>
        <div v-else class="monitoring-groups">
          <article v-for="group in groups" :key="group.id" :data-group-id="group.id">
            <GroupMonitorCard
              :name="group.name"
              :description="group.description"
              :multiplier="group.rate_multiplier"
              :stats="group"
              :window-start="snapshot.start"
              :window-end="snapshot.end"
              :bucket-minutes="snapshot.bucket_minutes"
            >
              <template #footer
                ><span>{{
                  t('monitoring.activeModelCount', {
                    count: group.active_models ?? group.models.length,
                  })
                }}</span
                ><button type="button" class="monitoring-text-button" @click="openGroup(group.id)"
                  >{{ t('monitoring.viewModels') }}<Icon name="chevronRight" size="sm" /></button
              ></template>
            </GroupMonitorCard>
          </article>
        </div>
        <p class="monitoring-sample-note">{{ t('monitoring.sampleNote') }}</p>
      </template>
    </section>
    <BaseDialog
      :show="cacheHelp"
      :title="t('monitoring.cacheExplanation')"
      trap-focus
      @close="cacheHelp = false"
      ><div class="space-y-4 text-sm leading-6"
        ><p>{{ t('monitoring.cacheHitRateHint') }}</p
        ><p>{{ t('monitoring.tpsHint') }}</p
        ><p>{{ t('monitoring.sampleNote') }}</p></div
      ></BaseDialog
    >
    <BaseDialog
      :show="!!selected"
      :title="selected?.name ?? ''"
      width="wide"
      trap-focus
      @close="closeGroup"
    >
      <div v-if="selected && snapshot" class="space-y-6">
        <section
          ><h3 class="text-sm font-semibold">{{ t('monitoring.trend') }}</h3
          ><p class="mt-1 text-xs text-gray-500">{{ t('monitoring.trendHint') }}</p>
          <svg
            v-if="segments.length"
            viewBox="0 0 600 128"
            class="mt-4 h-36 w-full text-primary-500"
            role="img"
            :aria-label="t('monitoring.trend')"
          >
            <line x1="8" y1="112" x2="592" y2="112" stroke="currentColor" stroke-opacity="0.15" />
            <polyline
              v-for="(segment, index) in segments"
              :key="index"
              :points="segment"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              vector-effect="non-scaling-stroke"
            />
            <template v-for="(segment, index) in segments" :key="`dots-${index}`">
              <circle
                v-for="point in segment.split(' ')"
                :key="point"
                :cx="point.split(',')[0]"
                :cy="point.split(',')[1]"
                r="2"
                fill="currentColor"
              />
            </template>
          </svg>
          <p
            v-else
            class="mt-4 rounded-surface bg-gray-50 p-6 text-center text-sm text-gray-500 dark:bg-dark-800"
            >{{ t('monitoring.noSamples') }}</p
          >
        </section>
        <section
          ><h3 class="text-sm font-semibold">{{ t('monitoring.models') }}</h3>
          <div v-if="selected.models.length" class="mt-3 overflow-x-auto"
            ><table class="w-full text-left text-xs"
              ><thead
                ><tr class="border-b border-gray-100 text-gray-500 dark:border-dark-700"
                  ><th class="p-3">{{ t('monitoring.model') }}</th
                  ><th class="p-3">{{ t('monitoring.modelRequests') }}</th
                  ><th class="p-3">{{ t('monitoring.latency') }}</th
                  ><th class="p-3">{{ t('monitoring.ttft') }}</th
                  ><th class="p-3">TPS</th></tr
                ></thead
              ><tbody
                ><tr
                  v-for="model in selected.models"
                  :key="model.model"
                  class="border-b border-gray-100 dark:border-dark-700"
                  ><td class="max-w-64 break-words p-3 font-mono">{{ model.model }}</td
                  ><td class="p-3 tabular-nums">{{ model.requests.toLocaleString() }}</td
                  ><td class="whitespace-nowrap p-3 tabular-nums">{{
                    duration(model.avg_duration_ms)
                  }}</td
                  ><td class="whitespace-nowrap p-3 tabular-nums">{{
                    duration(model.avg_ttft_ms)
                  }}</td
                  ><td class="whitespace-nowrap p-3 tabular-nums">{{
                    model.avg_tps == null ? '—' : `${model.avg_tps.toFixed(1)} t/s`
                  }}</td></tr
                ></tbody
              ></table
            ></div
          >
          <p v-else class="mt-3 text-sm text-gray-500">{{ t('monitoring.noModels') }}</p>
        </section>
        <div class="flex justify-end"
          ><RouterLink
            class="btn btn-primary"
            :to="{ path: '/models', query: { q: selected.name } }"
            >{{ t('monitoring.pricing') }}</RouterLink
          ></div
        >
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<style scoped>
/* Transparent page surfaces reveal the application's shared backdrop; controls
   keep their own focus and selection states for reading the monitoring filters. */
.monitoring-layout :deep(.app-main) {
  @apply bg-transparent;
}
.monitoring-page {
  @apply w-full min-w-0;
}
.monitoring-heading {
  @apply flex flex-wrap items-start justify-between gap-4;
}
.monitoring-heading h1 {
  @apply text-2xl font-semibold;
}
.monitoring-heading p {
  @apply mt-3 text-sm leading-6 text-gray-500 dark:text-dark-300;
}
.monitoring-header-actions {
  @apply flex flex-wrap items-center gap-2;
}
.monitoring-window {
  @apply inline-flex border border-gray-200 dark:border-dark-600;
}
.monitoring-window button {
  @apply min-h-10 px-3 text-sm;
}
.monitoring-window button[aria-pressed='true'] {
  @apply bg-gray-100 font-semibold dark:bg-dark-700;
  color: var(--tr-color-brand);
}
.monitoring-refresh {
  @apply inline-flex min-h-10 items-center gap-2 border border-gray-200 px-3 text-sm disabled:opacity-50 dark:border-dark-600;
}
.monitoring-toolbar {
  @apply mt-4 flex flex-wrap items-center justify-between gap-2 text-xs leading-5 text-gray-500 dark:text-dark-300;
}
.monitoring-controls {
  @apply mt-5 flex flex-wrap items-center justify-end gap-2;
}
.monitoring-search {
  @apply mr-auto flex min-w-0 basis-60 items-center gap-2 border border-gray-200 px-3 py-2 text-gray-500 dark:border-dark-600;
}
.monitoring-search input {
  @apply min-w-0 w-full bg-transparent text-base outline-none;
}
.monitoring-select {
  @apply w-40;
}
.monitoring-select :deep(.input) {
  @apply bg-transparent;
  border-radius: 0;
}
.monitoring-text-button {
  @apply inline-flex min-h-10 items-center justify-center gap-1 px-2 text-sm;
}
.monitoring-legend {
  @apply mb-5 mt-6 flex flex-wrap gap-x-5 gap-y-2 text-sm text-gray-500 dark:text-dark-300;
}
.monitoring-legend span {
  @apply inline-flex items-center gap-2;
}
.monitoring-legend i {
  @apply h-2 w-2 rounded-full bg-current;
}
[data-status='healthy'] {
  @apply text-emerald-700 dark:text-emerald-400;
}
[data-status='degraded'] {
  @apply text-amber-700 dark:text-amber-400;
}
[data-status='incident'] {
  @apply text-red-600 dark:text-red-400;
}
.monitoring-groups {
  @apply grid grid-cols-1 gap-5 md:grid-cols-2 2xl:grid-cols-3;
}
.monitoring-groups article {
  @apply min-w-0;
}
.monitoring-placeholder {
  @apply border border-gray-200 p-5 dark:border-dark-600;
}
.monitoring-empty {
  @apply my-6 border border-gray-200 px-4 py-16 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-300;
}
.monitoring-empty h2 {
  @apply mb-3 font-semibold;
}
.monitoring-error {
  @apply my-4 flex flex-wrap items-center justify-between gap-2 border border-red-200 p-4 text-sm text-red-600 dark:border-red-900 dark:text-red-400;
}
.monitoring-sample-note {
  @apply mt-5 text-xs leading-6 text-gray-500 dark:text-dark-300;
}
.monitoring-text-button:hover,
.monitoring-refresh:hover {
  @apply bg-gray-50 dark:bg-dark-800;
}
.monitoring-page button:focus-visible,
.monitoring-search:focus-within {
  outline: 2px solid var(--tr-color-brand);
  outline-offset: 2px;
}
/* Match the shared sm breakpoint and retain usable controls on narrow phones. */
@media (max-width: 639px) {
  .monitoring-search {
    @apply basis-full;
  }
  .monitoring-select {
    @apply min-w-0 flex-1;
  }
  .monitoring-controls .monitoring-text-button {
    @apply basis-full justify-end;
  }
  .monitoring-header-actions button,
  .monitoring-text-button {
    @apply min-h-11;
  }
}
</style>
