<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
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
// requests and is cleared on unmount. Detail charts preserve missing hours as
// gaps and show the same bounded model observations as the group's summary.

// Filters and remote data belong to this mounted authenticated monitoring page.
const { t } = useI18n()
const auth = useAuthStore()
const hours = ref(24)
const search = ref('')
const status = ref('all')
const snapshot = ref<MonitoringSnapshot | null>(null)
const loading = ref(false)
const failed = ref(false)
const selectedID = ref<number | null>(null)
let generation = 0
let timer: ReturnType<typeof setInterval> | undefined
const windows = computed(() =>
  [24, 72, 168].map((value) => ({ value, label: t(`monitoring.hours${value}`) })),
)
const states: MonitoringStatus[] = ['healthy', 'degraded', 'incident', 'idle']
const statusOptions = computed(() => [
  { value: 'all', label: t('monitoring.all') },
  ...states.map((value) => ({ value, label: t(`monitoring.${value}`) })),
])
const groups = computed(() =>
  (snapshot.value?.groups ?? []).filter(
    (group) =>
      group.name.toLocaleLowerCase().includes(search.value.toLocaleLowerCase()) &&
      (status.value === 'all' || monitoringStatus(group) === status.value),
  ),
)
const selected = computed(() =>
  snapshot.value?.groups.find((group) => group.id === selectedID.value),
)
const segments = computed(() =>
  selected.value && snapshot.value
    ? trendSegments(selected.value.trend, snapshot.value.start, snapshot.value.end)
    : [],
)
const totals = computed(() =>
  (snapshot.value?.groups ?? []).reduce(
    (sum, group) => ({ success: sum.success + group.success, errors: sum.errors + group.errors }),
    { success: 0, errors: 0 },
  ),
)
const successRate = computed(() =>
  totals.value.success + totals.value.errors
    ? (100 * totals.value.success) / (totals.value.success + totals.value.errors)
    : null,
)

// tone applies semantic status colors while retaining the translated status text.
function tone(value: MonitoringStatus): string {
  return {
    healthy: 'text-green-600 dark:text-green-400',
    degraded: 'text-amber-600 dark:text-amber-400',
    incident: 'text-red-600 dark:text-red-400',
    idle: 'text-gray-500 dark:text-gray-400',
  }[value]
}

// percentage preserves missing observations as a dash and measured zero as zero.
function percentage(value: number | null): string {
  return value == null ? '—' : `${value.toFixed(2)}%`
}

// duration formats only observed millisecond durations, retaining absent timing explicitly.
function duration(value: number | null): string {
  return value == null ? '—' : `${Math.round(value).toLocaleString()} ms`
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
  { immediate: true },
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
  <AppLayout>
    <section class="space-y-4" :aria-busy="loading">
      <div class="card p-4 sm:p-5">
        <div class="flex flex-wrap items-center gap-3">
          <Select
            v-model="hours"
            :options="windows"
            :aria-label="t('monitoring.window')"
            class="w-36"
          />
          <label class="min-w-0 flex-1 basis-48"
            ><span class="sr-only">{{ t('monitoring.search') }}</span
            ><input v-model="search" class="input w-full" :placeholder="t('monitoring.search')"
          /></label>
          <Select
            v-model="status"
            :options="statusOptions"
            :aria-label="t('monitoring.all')"
            class="w-36"
          />
          <button class="btn btn-secondary" :disabled="loading" @click="refresh"
            ><Icon name="refresh" size="sm" />{{ t('monitoring.refresh') }}</button
          >
          <RouterLink class="btn btn-primary" to="/support-tickets"
            ><Icon name="ticket" size="sm" />{{ t('monitoring.tickets') }}</RouterLink
          >
        </div>
        <div class="mt-4 flex flex-wrap justify-between gap-2 text-xs text-gray-500"
          ><p>{{ t('monitoring.scope') }}</p
          ><time v-if="snapshot" :datetime="snapshot.end">{{
            t('monitoring.updated', { time: new Date(snapshot.end).toLocaleTimeString() })
          }}</time></div
        >
      </div>
      <div v-if="failed" role="alert" class="card flex flex-wrap justify-between gap-3 p-4"
        ><p class="text-sm text-red-600 dark:text-red-400">{{ t('monitoring.refreshFailed') }}</p
        ><button class="btn btn-secondary" @click="refresh">{{
          t('monitoring.retry')
        }}</button></div
      >
      <div
        v-if="loading && !snapshot"
        class="grid grid-cols-1 gap-4 md:grid-cols-2"
        role="status"
        :aria-label="t('common.loading')"
        ><div v-for="index in 4" :key="index" class="card h-48 p-5"
          ><div class="skeleton h-full rounded-surface" aria-hidden="true"></div></div
      ></div>
      <div v-else-if="snapshot && !snapshot.enabled" class="card px-5 py-16 text-center"
        ><Icon name="monitor" size="xl" class="mx-auto text-gray-400" /><h2
          class="mt-4 font-semibold"
          >{{ t('monitoring.disabled') }}</h2
        ><p class="mt-2 text-sm text-gray-500">{{ t('monitoring.disabledHint') }}</p></div
      >
      <template v-else-if="snapshot">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div class="card p-5"
            ><p class="text-xs text-gray-500">{{ t('monitoring.successRate') }}</p
            ><p class="mt-2 text-2xl font-semibold tabular-nums">{{
              percentage(successRate)
            }}</p></div
          >
          <div class="card p-5"
            ><p class="text-xs text-gray-500">{{ t('monitoring.requests') }}</p
            ><p class="mt-2 text-2xl font-semibold tabular-nums">{{
              (totals.success + totals.errors).toLocaleString()
            }}</p></div
          >
          <div class="card p-5"
            ><p class="text-xs text-gray-500">{{ t('monitoring.failures') }}</p
            ><p class="mt-2 text-2xl font-semibold tabular-nums">{{
              totals.errors.toLocaleString()
            }}</p></div
          >
        </div>
        <p class="text-xs leading-6 text-gray-500">{{ t('monitoring.sampleNote') }}</p>
        <div v-if="!groups.length" class="card py-16 text-center text-sm text-gray-500">{{
          t('monitoring.empty')
        }}</div>
        <div v-else class="monitoring-groups">
          <article v-for="group in groups" :key="group.id" class="card flex min-w-0 flex-col p-5">
            <div class="flex flex-wrap items-start justify-between gap-3"
              ><h2 class="min-w-0 break-words font-semibold">{{ group.name }}</h2
              ><span class="shrink-0 text-xs font-medium" :class="tone(monitoringStatus(group))">{{
                t(`monitoring.${monitoringStatus(group)}`)
              }}</span></div
            >
            <p
              class="mt-5 text-3xl font-semibold tabular-nums"
              :class="tone(monitoringStatus(group))"
              >{{ percentage(group.success_rate) }}</p
            >
            <p class="mt-1 text-xs text-gray-500">{{
              group.success_rate == null ? t('monitoring.noSamples') : t('monitoring.successRate')
            }}</p>
            <dl class="mt-5 grid grid-cols-2 gap-x-4 gap-y-3 text-xs">
              <div class="col-span-2" :title="t('monitoring.cacheHitRateHint')">
                <dt class="text-gray-500">{{ t('monitoring.cacheHitRate') }}</dt>
                <dd class="mt-1 font-medium tabular-nums">{{
                  group.cache_hit_rate == null
                    ? t('monitoring.noCacheSamples')
                    : percentage(group.cache_hit_rate * 100)
                }}</dd>
              </div>
              <div
                ><dt class="text-gray-500">{{ t('monitoring.requests') }}</dt
                ><dd class="mt-1 font-medium tabular-nums">{{
                  (group.success + group.errors).toLocaleString()
                }}</dd></div
              >
              <div
                ><dt class="text-gray-500">{{ t('monitoring.failures') }}</dt
                ><dd class="mt-1 font-medium tabular-nums">{{
                  group.errors.toLocaleString()
                }}</dd></div
              >
              <div
                ><dt class="text-gray-500">{{ t('monitoring.latency') }}</dt
                ><dd class="mt-1 font-medium tabular-nums">{{
                  duration(group.avg_duration_ms)
                }}</dd></div
              >
              <div
                ><dt class="text-gray-500">{{ t('monitoring.ttft') }}</dt
                ><dd class="mt-1 font-medium tabular-nums">{{
                  duration(group.avg_ttft_ms)
                }}</dd></div
              >
            </dl>
            <button class="btn btn-secondary mt-5 w-full" @click="openGroup(group.id)"
              >{{ t('monitoring.details') }}<Icon name="arrowRight" size="sm"
            /></button>
          </article>
        </div>
      </template>
    </section>
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
                  ><th class="p-3">{{ t('monitoring.ttft') }}</th></tr
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
/* Group cards keep enough room for latency units inside a sidebar-constrained page. */
.monitoring-groups {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr));
  gap: 1rem;
}
</style>
