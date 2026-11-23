<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitoringGroup } from '@/api/monitoring'
import { formatCompactNumber } from '@/utils/format'
import { monitoringBuckets, monitoringStatus } from '@/utils/monitoring'

// GroupMonitorCard.vue presents routing metadata beside measured group performance.
// The caller supplies an authorized group and optional aggregate observations;
// this card cannot fetch or grant group access. Request success, input-token
// caching, and catalog model counts describe different observations and retain
// separate labels so a missing measurement never looks like healthy traffic.
//
// An aligned time strip uses the supplied observation range and bucket resolution.
// Missing intervals receive empty cells, while measured rates determine both bar
// height and status color. The headline uses weighted request counts rather than
// averaging interval rates, and cache percentages use the endpoint's input-token
// denominator. This keeps sparse groups comparable without filling their gaps.

// Props carries display metadata and nullable observations; the parent owns selection
// and persistence, while this presentation preserves explicit no-sample states.
const props = defineProps<{
  name: string // Authorized group display name.
  description?: string | null // Optional group purpose description.
  multiplier?: number | null // Displayed group billing multiplier.
  referenceMultiplier?: number | null // Effective official-price reference ratio.
  modelCount?: number // Catalog requestable model count.
  stats?: MonitoringGroup | null // Permission-scoped aggregate observation window.
  selected?: boolean // Parent-owned current group selection.
  // windowEnd anchors the strip to the observed endpoint; omission uses current
  // time so old samples cannot occupy the newest displayed hour by default.
  windowEnd?: string
  windowStart?: string // Inclusive observation window start.
  bucketMinutes?: number // Trend resolution; omitted historical responses use sixty.
}>()
const { t } = useI18n()

// Hour conversion labels observation windows without altering their declared
// bucket resolution or the timestamps returned by the monitoring endpoint.
const HOUR_MS = 3600000

// Summary and interval measurements retain their own denominators and availability;
// no-sample labels remain visible independently of the caller's selected state.
const rate = computed(() => props.stats?.success_rate)
const requestCount = computed(() => (props.stats?.success ?? 0) + (props.stats?.errors ?? 0))
const history = computed(() => {
  // Anchor missing recent intervals to the observation endpoint, rather than moving
  // the most recent request to now and hiding the following period without traffic.
  const end = props.windowEnd ? new Date(props.windowEnd).getTime() : Date.now()
  const step = (props.bucketMinutes ?? 60) * 60000
  const start =
    props.windowStart ?? new Date(Math.floor((end - 1) / step) * step - 23 * step).toISOString()
  return monitoringBuckets(
    props.stats?.trend ?? [],
    start,
    new Date(end).toISOString(),
    props.bucketMinutes,
  )
})
const windowHours = computed(() =>
  props.windowStart && props.windowEnd
    ? Math.round((Date.parse(props.windowEnd) - Date.parse(props.windowStart)) / HOUR_MS)
    : 24,
)
const status = computed(() => (props.stats ? monitoringStatus(props.stats) : 'idle'))

// percent formats observed fractional cache rates without inventing missing samples.
// Missing or invalid values return an explicit no-sample label. Measured zero
// remains zero percent so users can distinguish cache misses from absent input.
function percent(value?: number | null): string {
  return value == null || !Number.isFinite(value)
    ? t('monitoring.noCacheSamples')
    : `${(value * 100).toFixed(2)}%`
}

// seconds formats observed request timing in seconds for compact monitoring cards.
// Missing and invalid timing remain unavailable. Millisecond observations are
// divided by one thousand for display without rounding the underlying snapshot.
function seconds(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '—' : `${(value / 1000).toFixed(2)}s`
}

// bucketTitle names a local interval and its measured success or absent sample state.
function bucketTitle(time: number, rate: number | null): string {
  return `${new Date(time).toLocaleString()} · ${rate == null ? t('monitoring.noSamples') : `${(rate * 100).toFixed(2)}%`}`
}
</script>

<template>
  <div class="group-monitor-card" :class="{ 'group-monitor-selected': selected }">
    <div class="group-monitor-heading">
      <span class="group-monitor-name" :title="name">{{ name }}</span>
      <span v-if="multiplier != null" class="shrink-0 text-xs text-gray-500"
        >×{{ multiplier }}</span
      >
      <span class="group-monitor-status" :data-status="status"
        ><span class="group-monitor-dot" aria-hidden="true"></span
        >{{ t(`monitoring.${status}`) }}</span
      >
    </div>
    <p v-if="description" class="group-monitor-description" :title="description">{{
      description
    }}</p>
    <p class="sr-only">{{ t('monitoring.windowHours', { count: windowHours }) }}</p>
    <dl class="group-monitor-timings">
      <div
        ><dt>{{ t('monitoring.latency') }}</dt
        ><dd>{{ seconds(stats?.avg_duration_ms) }}</dd></div
      >
      <div
        ><dt>TTFT</dt><dd>{{ seconds(stats?.avg_ttft_ms) }}</dd></div
      >
      <div :title="t('monitoring.tpsHint')"
        ><dt>TPS</dt
        ><dd>{{ stats?.avg_tps == null ? '—' : `${stats.avg_tps.toFixed(1)} t/s` }}</dd></div
      >
    </dl>
    <div class="group-monitor-availability">
      <div class="flex flex-wrap items-end justify-between gap-2">
        <div class="text-left"
          ><p class="text-xs text-gray-500">{{ t('monitoring.availability') }}</p
          ><p class="mt-1 text-xs text-gray-500">{{
            requestCount
              ? t('monitoring.successfulRequests', {
                  success: (stats?.success ?? 0).toLocaleString(),
                  total: requestCount.toLocaleString(),
                })
              : t('monitoring.noSamples')
          }}</p></div
        >
        <strong class="group-monitor-rate" :data-status="status">{{
          rate == null ? '—' : `${rate.toFixed(2)}%`
        }}</strong>
      </div>
    </div>
    <div class="group-monitor-bars" role="img" :aria-label="`${name}: ${t('monitoring.trend')}`">
      <span
        v-for="(bucket, index) in history"
        :key="index"
        :data-state="
          bucket.rate == null
            ? 'empty'
            : bucket.rate >= 0.99
              ? 'healthy'
              : bucket.rate >= 0.95
                ? 'degraded'
                : 'incident'
        "
        :style="{ '--bar-height': `${bucket.rate == null ? 16 : 25 + bucket.rate * 75}%` }"
        :title="bucketTitle(bucket.time, bucket.rate)"
      />
    </div>
    <div class="mt-1 flex justify-between text-xs text-gray-400"
      ><span>{{ t('monitoring.past') }}</span
      ><span>{{ t('monitoring.now') }}</span></div
    >
    <dl class="group-monitor-footer">
      <div :title="t('monitoring.cacheHitRateHint')"
        ><dt>{{ t('monitoring.cacheHitRate') }}</dt
        ><dd>{{ percent(stats?.cache_hit_rate) }}</dd></div
      >
      <!-- Compact totals keep large cache volumes on one line; the title retains
           the exact observed count for inspecting a rounded display. -->
      <div
        ><dt>{{ t('monitoring.cacheReadTokens') }}</dt
        ><dd class="whitespace-nowrap" :title="stats?.cache_read_tokens?.toLocaleString()">{{
          stats?.cache_read_tokens == null ? '—' : formatCompactNumber(stats.cache_read_tokens)
        }}</dd></div
      >
      <div v-if="referenceMultiplier != null"
        ><dt>{{ t('monitoring.referenceMultiplier') }}</dt
        ><dd>{{ referenceMultiplier.toFixed(3) }}×</dd></div
      >
      <div v-else-if="modelCount != null"
        ><dt>{{ t('monitoring.availableModels') }}</dt
        ><dd>{{ modelCount }}</dd></div
      >
      <div v-else
        ><dt>{{ t('monitoring.failures') }}</dt
        ><dd>{{ stats?.errors ?? '—' }}</dd></div
      >
    </dl>
    <div v-if="$slots.footer" class="group-monitor-actions"><slot name="footer" /></div>
  </div>
</template>

<style scoped>
/* Group cards use shared surfaces and status colors; selection retains the exact
   primary token so routing choices match other account controls in both themes. */
.group-monitor-card {
  @apply flex h-full min-w-0 flex-col border border-gray-200 bg-white p-5 text-left dark:border-dark-600 dark:bg-dark-900;
  container-type: inline-size;
}
.group-monitor-selected {
  @apply ring-1;
  border-color: var(--tr-color-brand);
  --tw-ring-color: var(--tr-color-brand);
}
.group-monitor-heading {
  @apply flex min-w-0 flex-wrap items-center gap-2;
}
.group-monitor-name {
  @apply min-w-0 flex-1 truncate text-base font-semibold;
  color: var(--tr-color-brand);
}
.group-monitor-description {
  @apply mt-2 truncate text-left text-sm text-gray-500 dark:text-dark-300;
}
.group-monitor-status {
  @apply ml-auto inline-flex shrink-0 items-center gap-1.5 rounded-full border border-gray-200 px-2 py-1 text-xs dark:border-dark-600;
}
.group-monitor-dot {
  @apply h-1.5 w-1.5 rounded-full bg-current;
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
[data-status='idle'] {
  @apply text-gray-500 dark:text-gray-400;
}
.group-monitor-timings {
  @apply mt-4 grid grid-cols-3 gap-2;
}
.group-monitor-timings > div {
  @apply min-w-0 bg-gray-50 p-3 dark:bg-dark-800;
}
dt {
  @apply text-xs text-gray-500 dark:text-dark-300;
}
dd {
  @apply mt-1 break-words font-mono text-base font-semibold tabular-nums text-gray-900 dark:text-white;
}
.group-monitor-availability {
  @apply mt-4 bg-gray-50 p-4 dark:bg-dark-800;
}
.group-monitor-rate {
  @apply font-mono text-3xl font-semibold tabular-nums;
}
.group-monitor-bars {
  @apply mt-4 flex h-12 items-end gap-1;
}
.group-monitor-bars > span {
  @apply min-w-0 flex-1;
  --radius-bar: 3px;
  border-radius: var(--radius-bar);
  height: var(--bar-height);
}
[data-state='healthy'] {
  @apply bg-emerald-500;
}
[data-state='degraded'] {
  @apply bg-amber-400;
}
[data-state='incident'] {
  @apply bg-red-400;
}
[data-state='empty'] {
  @apply bg-gray-200 dark:bg-dark-500;
}
.group-monitor-footer {
  @apply mt-5 grid grid-cols-3 gap-3 border-t border-gray-200 pt-4 dark:border-dark-600;
}
.group-monitor-actions {
  @apply mt-auto flex min-w-0 flex-wrap items-center justify-between gap-2 pt-5 text-sm text-gray-500 dark:text-dark-300;
}
/* Short labels stay above their values when cards enter compact selection menus. */
@container (max-width: 20rem) {
  .group-monitor-bars {
    @apply gap-0.5;
  }
  .group-monitor-card dd {
    @apply text-sm;
  }
  .group-monitor-timings > div {
    @apply p-2;
  }
  .group-monitor-footer {
    @apply grid-cols-2;
  }
  .group-monitor-status {
    @apply text-xs;
  }
}
</style>
