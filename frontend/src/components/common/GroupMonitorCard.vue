<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitoringGroup } from '@/api/monitoring'

// GroupMonitorCard.vue presents routing metadata beside measured group performance.
// The caller supplies an authorized group and optional aggregate observations;
// this card cannot fetch or grant group access. Request success, input-token
// caching, and catalog model counts describe different observations and retain
// separate labels so a missing measurement never looks like healthy traffic.
//
// A fixed twenty-four-hour strip aligns every group's hourly request buckets.
// Missing hours receive empty cells, while measured rates determine both bar
// height and status color. The headline uses weighted request counts rather than
// averaging hourly rates, and cache percentages use the endpoint's input-token
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
}>()
const { t } = useI18n()

// Hourly windows and health thresholds govern presentation of observed traffic;
// these display bands do not declare provider availability or service guarantees.
const OBSERVATION_HOURS = 24
const HOUR_MS = 3600000
const HEALTHY_RATE = 99
const DEGRADED_RATE = 95

// Summary and hourly measurements retain their own denominators and availability;
// no-sample labels remain visible independently of the caller's selected state.
const rate = computed(() => props.stats?.success_rate)
const requestCount = computed(() => (props.stats?.success ?? 0) + (props.stats?.errors ?? 0))
const history = computed(() => {
  // Anchor missing recent hours to the observation endpoint, rather than moving
  // the most recent request to now and hiding the following period without traffic.
  const end = props.windowEnd ? new Date(props.windowEnd).getTime() : Date.now()
  const finalHour = Math.floor(end / HOUR_MS) * HOUR_MS
  const points = new Map(
    props.stats?.trend.map((point) => [new Date(point.time).getTime(), point]) ?? [],
  )
  return Array.from({ length: OBSERVATION_HOURS }, (_, index) => {
    const point = points.get(finalHour - (OBSERVATION_HOURS - 1 - index) * HOUR_MS)
    const total = point ? point.success + point.errors : 0
    return total ? point!.success / total : null
  })
})
const status = computed(() =>
  rate.value == null
    ? 'idle'
    : rate.value >= HEALTHY_RATE
      ? 'healthy'
      : rate.value >= DEGRADED_RATE
        ? 'degraded'
        : 'incident',
)

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
</script>

<template>
  <div class="group-monitor-card" :class="{ 'group-monitor-selected': selected }">
    <div class="group-monitor-heading">
      <span class="min-w-0 break-words font-semibold text-gray-900 dark:text-white">{{
        name
      }}</span>
      <span v-if="multiplier != null" class="shrink-0 text-xs text-gray-500"
        >×{{ multiplier }}</span
      >
      <span class="group-monitor-status" :data-status="status">{{
        t(`monitoring.${status}`)
      }}</span>
    </div>
    <p
      v-if="description"
      class="mt-2 line-clamp-2 text-left text-sm text-gray-500 dark:text-dark-300"
      >{{ description }}</p
    >
    <p class="mt-3 text-xs text-gray-500 dark:text-dark-300">{{ t('monitoring.hours24') }}</p>
    <dl class="group-monitor-timings">
      <div
        ><dt>{{ t('monitoring.latency') }}</dt
        ><dd>{{ seconds(stats?.avg_duration_ms) }}</dd></div
      >
      <div
        ><dt>TTFT</dt><dd>{{ seconds(stats?.avg_ttft_ms) }}</dd></div
      >
    </dl>
    <div class="group-monitor-availability">
      <div class="flex flex-wrap items-end justify-between gap-2">
        <div class="text-left"
          ><p class="text-xs text-gray-500">{{ t('monitoring.successRate') }}</p
          ><p class="mt-1 text-xs text-gray-500"
            >{{ stats?.success ?? 0 }}/{{ requestCount }} {{ t('monitoring.requests') }}</p
          ></div
        >
        <strong class="text-xl tabular-nums" :data-status="status">{{
          rate == null ? '—' : `${rate.toFixed(2)}%`
        }}</strong>
      </div>
      <div class="group-monitor-bars" role="img" :aria-label="t('monitoring.trend')">
        <span
          v-for="(bucket, index) in history"
          :key="index"
          :data-state="
            bucket == null
              ? 'empty'
              : bucket >= 0.99
                ? 'healthy'
                : bucket >= 0.95
                  ? 'degraded'
                  : 'incident'
          "
          :style="{ '--bar-height': `${bucket == null ? 16 : 25 + bucket * 75}%` }"
          :title="bucket == null ? t('monitoring.noSamples') : `${(bucket * 100).toFixed(2)}%`"
        />
      </div>
      <div class="mt-1 flex justify-between text-xs text-gray-400"
        ><span>{{ t('monitoring.past') }}</span
        ><span>{{ t('monitoring.now') }}</span></div
      >
    </div>
    <dl class="group-monitor-footer">
      <div
        ><dt>{{ t('monitoring.cacheHitRate') }}</dt
        ><dd>{{ percent(stats?.cache_hit_rate) }}</dd></div
      >
      <div v-if="referenceMultiplier != null"
        ><dt>{{ t('monitoring.referenceMultiplier') }}</dt
        ><dd>{{ referenceMultiplier.toFixed(3) }}×</dd></div
      >
      <div v-if="modelCount != null"
        ><dt>{{ t('monitoring.availableModels') }}</dt
        ><dd>{{ modelCount }}</dd></div
      >
    </dl>
  </div>
</template>

<style scoped>
/* Group cards use shared surfaces and status colors; selection retains the exact
   primary token so routing choices match other account controls in both themes. */
.group-monitor-card {
  @apply rounded-surface border border-gray-200 bg-white p-4 text-left dark:border-dark-600 dark:bg-dark-800;
}
.group-monitor-selected {
  @apply ring-1;
  border-color: var(--tr-color-brand);
  --tw-ring-color: var(--tr-color-brand);
}
.group-monitor-heading {
  @apply flex items-start gap-2;
}
.group-monitor-status {
  @apply ml-auto shrink-0 rounded-full border border-current px-2 py-0.5 text-xs;
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
  @apply mt-4 grid grid-cols-2 gap-2;
}
.group-monitor-timings > div {
  @apply rounded-compact bg-gray-50 p-2.5 dark:bg-dark-700;
}
dt {
  @apply text-xs text-gray-500 dark:text-dark-300;
}
dd {
  @apply mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white;
}
.group-monitor-availability {
  @apply mt-3 rounded-compact bg-gray-50 p-3 dark:bg-dark-700;
}
.group-monitor-bars {
  @apply mt-3 flex h-8 items-end gap-1;
}
.group-monitor-bars > span {
  @apply min-w-0 flex-1 rounded-compact;
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
  @apply mt-4 grid grid-cols-2 gap-3 border-t border-gray-100 pt-3 dark:border-dark-600;
}
</style>
