<script setup lang="ts">
import Skeleton from '@/components/common/Skeleton.vue'
import TableSkeletonBody from '@/components/common/TableSkeletonBody.vue'

import { computed, ref, watch } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { TABLE_DESKTOP_MEDIA_QUERY } from '@/constants/layout'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  opsAPI,
  type OpsCacheStatsGroupItem,
  type OpsCacheStatsResponse,
  type OpsTokenStatsTimeRange,
} from '@/api/admin/ops'
import { formatNumber } from '@/utils/format'

// OpsCacheStatsCard.vue queries cache-hit statistics with an independent time
// control under dashboard platform and group filters. The overall block sums
// every matching request, while the table repeats the same counters per
// recorded group. The group selector emits its choice to the parent, which
// supplies the applied group filter.
//
// A hit rate arrives as a fraction of the input side, and a missing rate means
// the slice had no input-side tokens; the card renders that absence as a dash
// instead of zero percent. Request failures clear the result and expose an
// inline error for retry.

// Props supplies execution-platform and group constraints plus selectable group
// identities.
interface Props {
  // platformFilter supplies the execution-platform constraint; an empty or
  // omitted value leaves platform selection unrestricted.
  platformFilter?: string
  // groupIdFilter restricts requests to a positive persisted group ID; null or
  // omission leaves groups unrestricted.
  groupIdFilter?: number | null
  // groups supplies selectable group identities and labels; the view reads these
  // records and emits selections to their owner.
  groups?: Array<{
    id: number // persisted group identifier
    name: string // group selector display name
  }>
}

// Emits returns the selected group to the dashboard filter owner.
interface Emits {
  // update:group publishes a positive group identifier or null to clear
  // filtering.
  (e: 'update:group', value: number | null): void
}

// Component bindings read parent-owned group constraints and emit group choices
// before queries observe the updated props.
const props = withDefaults(defineProps<Props>(), {
  platformFilter: '',
  groupIdFilter: null,
  groups: () => [],
})
const emit = defineEmits<Emits>()

// Translation supplies cache-statistics labels and inline failure messages for
// this component instance and its selectors.
const { t } = useI18n()

// isDesktopViewport selects the shared desktop table or narrow card presentation
// for cache statistics rows.
const isDesktopViewport = useMediaQuery(TABLE_DESKTOP_MEDIA_QUERY)

// Request state holds the current cache response and a separate inline error
// when retrieval fails.
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsCacheStatsResponse | null>(null)

// Query controls choose the statistics window shared by the overall block and
// the group table.
const timeRange = ref<OpsTokenStatsTimeRange>('30d')

// Derived values expose the overall totals and group rows, using an absent
// response as an empty result for the first render.
const overall = computed(() => response.value?.overall ?? null)
const cacheGroups = computed(() => response.value?.groups ?? [])

// Selector choices map translated time windows and group identities to request
// parameters.
const timeRangeOptions = computed(() => [
  { value: '30m', label: t('admin.ops.timeRange.30m') },
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '1d', label: t('admin.ops.timeRange.1d') },
  { value: '15d', label: t('admin.ops.timeRange.15d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') },
])

const groupOptions = computed(() => {
  const filtered = props.groups
  return [
    { value: null, label: t('common.all') },
    ...filtered.map((group) => ({ value: group.id, label: group.name })),
  ]
})

// formatInt rounds finite counts before delegating to formatNumber for
// locale-aware display. Missing or nonfinite observations return a dash, and
// large rounded values use the shared formatter's compact notation.
function formatInt(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  // Round measurements to integers before locale-aware numeric formatting.
  return formatNumber(Math.round(v))
}

// formatPercent renders a hit-rate fraction as a percentage with two decimal
// places. A missing or nonfinite rate returns a dash, because the slice had no
// input-side tokens and no rate was observed.
function formatPercent(v?: number | null): string {
  if (typeof v !== 'number' || !Number.isFinite(v)) return '-'
  return `${(v * 100).toFixed(2)}%`
}

// groupLabel returns the recorded group name, or the numeric identifier when
// the group no longer carries a display name.
function groupLabel(row: OpsCacheStatsGroupItem): string {
  return row.group_name || `#${row.group_id}`
}

// handleGroupChange emits a positive selected group or clears the parent filter.
// Null, empty, boolean, invalid, and nonpositive selections emit null; string
// IDs are parsed before publication.
function handleGroupChange(value: string | number | boolean | null) {
  if (value === null || value === '' || typeof value === 'boolean') {
    emit('update:group', null)
    return
  }
  const id = typeof value === 'number' ? value : Number.parseInt(value, 10)
  emit('update:group', Number.isFinite(id) && id > 0 ? id : null)
}

// buildParams builds the cache-statistics request for the current window.
// Positive group IDs and nonempty platforms constrain the query; other values
// are omitted so the server applies its unrestricted defaults.
function buildParams() {
  return {
    time_range: timeRange.value,
    platform: props.platformFilter || undefined,
    group_id:
      typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0
        ? props.groupIdFilter
        : undefined,
  }
}

// loadData retrieves statistics and clears the previous inline error before
// requesting. Failures clear the response and expose an inline message, and
// loading resets in finally. There is no response-generation guard, so
// overlapping invocations can publish in completion order.
async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getCacheStats(buildParams())
  } catch (err: any) {
    // Clear the response on failure so the inline error replaces statistics from
    // an earlier query and the next load can retry the selection.
    console.error('[OpsCacheStatsCard] Failed to load data', err)
    response.value = null
    errorMessage.value = err?.message || t('admin.ops.cacheStats.failedToLoad')
  } finally {
    loading.value = false
  }
}

// Watch query controls together so a filter change issues one request with the
// updated window, platform, and group parameters.
watch(
  () => ({
    timeRange: timeRange.value,
    platform: props.platformFilter,
    groupId: props.groupIdFilter,
  }),
  () => {
    void loadData()
  },
  { immediate: true },
)
</script>

<template>
  <section class="card p-4 md:p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">
        {{ t('admin.ops.cacheStats.title') }}
      </h3>
      <div class="flex flex-wrap items-center gap-2">
        <div class="w-44" data-test="cache-stats-group-filter">
          <Select
            :model-value="groupIdFilter"
            :options="groupOptions"
            :aria-label="t('admin.ops.cacheStats.groupFilter')"
            @update:model-value="handleGroupChange"
          />
        </div>
        <div class="w-36">
          <Select v-model="timeRange" :options="timeRangeOptions" />
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-icon"
          :aria-label="t('common.refresh')"
          :title="t('common.refresh')"
          :disabled="loading"
          data-test="cache-stats-refresh"
          @click="loadData"
        >
          <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <div
      v-if="errorMessage"
      class="mb-4 rounded-control bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400"
    >
      {{ errorMessage }}
    </div>

    <EmptyState
      v-if="!loading && (!overall || overall.request_count === 0)"
      :title="t('common.noData')"
      :description="t('admin.ops.cacheStats.empty')"
    />

    <div v-else class="space-y-3">
      <div v-if="overall" class="grid grid-cols-2 gap-3 md:grid-cols-5">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.cacheStats.overall.hitRate') }}
          </div>
          <div class="mt-0.5 text-base font-semibold text-gray-900 dark:text-white">
            {{ formatPercent(overall.hit_rate) }}
          </div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.cacheStats.overall.requestCount') }}
          </div>
          <div class="mt-0.5 text-base font-semibold text-gray-900 dark:text-white">
            {{ formatInt(overall.request_count) }}
          </div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.cacheStats.overall.totalInputTokens') }}
          </div>
          <div class="mt-0.5 text-base font-semibold text-gray-900 dark:text-white">
            {{ formatInt(overall.total_input_tokens) }}
          </div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.cacheStats.overall.cacheReadTokens') }}
          </div>
          <div class="mt-0.5 text-base font-semibold text-gray-900 dark:text-white">
            {{ formatInt(overall.cache_read_tokens) }}
          </div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.cacheStats.overall.cacheCreationTokens') }}
          </div>
          <div class="mt-0.5 text-base font-semibold text-gray-900 dark:text-white">
            {{ formatInt(overall.cache_creation_tokens) }}
          </div>
        </div>
      </div>

      <div
        v-if="cacheGroups.length > 0 || loading"
        class="overflow-hidden rounded-surface border border-gray-200 dark:border-dark-700"
      >
        <div class="max-h-panel overflow-auto">
          <div v-if="!isDesktopViewport" class="divide-y divide-gray-100 dark:divide-dark-800">
            <div
              v-if="loading && !cacheGroups.length"
              role="status"
              :aria-label="t('common.loading')"
              aria-busy="true"
              data-loading-skeleton
            >
              <div
                v-for="row in 4"
                :key="row"
                class="space-y-2 p-3 border-b border-gray-100 last:border-0 dark:border-dark-800"
                aria-hidden="true"
              >
                <Skeleton :width="146" :height="14" class="max-w-full" />
                <div class="grid grid-cols-2 gap-x-3 gap-y-1">
                  <div
                    v-for="metric in 5"
                    :key="metric"
                    class="flex min-h-4 min-w-0 items-center justify-between gap-2"
                  >
                    <Skeleton width="56%" :height="12" />
                    <Skeleton width="24%" :height="12" />
                  </div>
                </div>
              </div>
            </div>
            <div v-else v-for="row in cacheGroups" :key="row.group_id" class="space-y-2 p-3">
              <div class="break-all text-xs font-medium text-gray-900 dark:text-gray-100">
                {{ groupLabel(row) }}
              </div>
              <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{
                    t('admin.ops.cacheStats.table.requestCount')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-200">{{
                    formatInt(row.request_count)
                  }}</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{
                    t('admin.ops.cacheStats.table.inputTokens')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-200">{{
                    formatInt(row.input_tokens)
                  }}</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{
                    t('admin.ops.cacheStats.table.cacheReadTokens')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-200">{{
                    formatInt(row.cache_read_tokens)
                  }}</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{
                    t('admin.ops.cacheStats.table.cacheCreationTokens')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-200">{{
                    formatInt(row.cache_creation_tokens)
                  }}</span>
                </div>
                <div class="flex items-baseline justify-between gap-2">
                  <span class="text-gray-500 dark:text-gray-400">{{
                    t('admin.ops.cacheStats.table.hitRate')
                  }}</span>
                  <span class="text-gray-700 dark:text-gray-200">{{
                    formatPercent(row.hit_rate)
                  }}</span>
                </div>
              </div>
            </div>
          </div>
          <table v-else class="min-w-full text-left text-xs md:text-sm">
            <thead class="sticky top-0 z-10 bg-white dark:bg-dark-900">
              <tr
                class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400"
              >
                <th class="px-2 py-2 font-semibold">{{ t('admin.ops.cacheStats.table.group') }}</th>
                <th class="px-2 py-2 font-semibold">
                  {{ t('admin.ops.cacheStats.table.requestCount') }}
                </th>
                <th class="px-2 py-2 font-semibold">
                  {{ t('admin.ops.cacheStats.table.inputTokens') }}
                </th>
                <th class="px-2 py-2 font-semibold">
                  {{ t('admin.ops.cacheStats.table.cacheReadTokens') }}
                </th>
                <th class="px-2 py-2 font-semibold">
                  {{ t('admin.ops.cacheStats.table.cacheCreationTokens') }}
                </th>
                <th class="px-2 py-2 font-semibold">
                  {{ t('admin.ops.cacheStats.table.hitRate') }}
                </th>
              </tr>
            </thead>
            <TableSkeletonBody
              v-if="loading && !cacheGroups.length"
              :columns="6"
              cell-class="px-2 py-2"
            />
            <tbody v-else>
              <tr
                v-for="row in cacheGroups"
                :key="row.group_id"
                class="border-b border-gray-100 text-gray-700 last:border-b-0 dark:border-dark-800 dark:text-gray-200"
              >
                <td class="px-2 py-2 font-medium">{{ groupLabel(row) }}</td>
                <td class="px-2 py-2">{{ formatInt(row.request_count) }}</td>
                <td class="px-2 py-2">{{ formatInt(row.input_tokens) }}</td>
                <td class="px-2 py-2">{{ formatInt(row.cache_read_tokens) }}</td>
                <td class="px-2 py-2">{{ formatInt(row.cache_creation_tokens) }}</td>
                <td class="px-2 py-2">{{ formatPercent(row.hit_rate) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </section>
</template>
