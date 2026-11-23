<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelPriceSummary from './ModelPriceSummary.vue'
import type { ModelSearchEntry } from '@/utils/marketplaceCatalog'
import type { CatalogQuote } from '@/utils/catalogQuote'
import type { MonitoringGroup } from '@/api/monitoring'

// ModelOfferCard.vue presents an indexed model with a coherent original quote.
// Compact input, output, and cache prices share that offer's routing identity.
// Capability text is derived only from declared attributes, while group descriptions
// remain labeled as group information rather than invented model introductions.
//
// Footer latency and status describe the quoted group, not every occurrence of
// this model. Missing observations remain empty, and cache hits use the same
// input-token denominator as service monitoring. Keeping the group name
// beside these measurements prevents a merged model row from implying universal
// performance across its independently priced and authorized routing offers.

// Props supplies immutable catalog and observation values; actions remain owned
// by the page so this card cannot alter routing or fetch additional private groups.
const props = defineProps<{
  entry: ModelSearchEntry // Merged model display identity.
  quote?: CatalogQuote // Original offer behind displayed prices.
  stats?: MonitoringGroup // Authorized quoted-group observations.
  tokenUnit: 'M' | 'K' // Token display scale preference.
  windowEnd?: string // Exclusive observation window endpoint.
}>()
const emit = defineEmits<{
  details: [id: string] // Opens original routing offers.
  copy: [id: string] // Copies the requested model identifier.
}>()
const { t } = useI18n()

// Capability facts and price metadata retain their original catalog sources;
// omitted attributes and absent measurements do not become affirmative claims.
const facts = computed(() => {
  const attributes = props.quote?.model.attributes
  const labels: string[] = []
  if (attributes?.reasoning) labels.push(t('marketplace.referenceReasoning'))
  if (attributes?.tool_call) labels.push(t('marketplace.referenceTools'))
  if (attributes?.structured_output) labels.push(t('marketplace.referenceStructured'))
  if (attributes?.context)
    labels.push(t('marketplace.referenceContext', { count: attributes.context.toLocaleString() }))
  return labels
})
const cacheRead = computed(() => {
  const pricing = props.quote?.model.pricing
  if (!pricing || props.entry.price.kind !== 'token') return undefined
  // Match the index's primary price source before reading cache prices, keeping
  // base quotes separate from context-specific intervals even inside one group.
  const hasBaseRate = [pricing.input_price_per_token, pricing.output_price_per_token].some(
    (rate) => typeof rate === 'number' && Number.isFinite(rate) && rate >= 0,
  )
  const source = hasBaseRate ? pricing : (pricing.context_intervals?.[0] ?? pricing)
  return source.cache_read_price_per_token
})
// Cache fractions become percentages, while durations convert from milliseconds
// to seconds; absence stays distinct from a measured zero in both summaries.
const cacheHit = computed(() =>
  props.stats?.cache_hit_rate == null ? '—' : `${(props.stats.cache_hit_rate * 100).toFixed(1)}%`,
)
const latency = computed(() =>
  props.stats?.avg_duration_ms == null
    ? '—'
    : `${(props.stats.avg_duration_ms / 1000).toFixed(2)}s`,
)
const buckets = computed(() => {
  // Missing recent hours remain empty instead of moving an old request to the
  // present; the exclusive endpoint keeps an exact hour boundary in the preceding
  // bucket. Status bands follow monitoring's 99% healthy and 95% degraded cutoffs.
  const hourMS = 3600000
  const lastHour =
    Math.floor(
      ((props.windowEnd ? new Date(props.windowEnd).getTime() : Date.now()) - 1) / hourMS,
    ) * hourMS
  const points = new Map(
    props.stats?.trend.map((point) => [new Date(point.time).getTime(), point]) ?? [],
  )
  return Array.from({ length: 12 }, (_, index) => {
    const point = points.get(lastHour - (11 - index) * hourMS)
    const total = point ? point.success + point.errors : 0
    return total ? point!.success / total : null
  })
})

// openDetails delegates model selection without changing the quoted group identity.
function openDetails(): void {
  emit('details', props.entry.id)
}

// copyID delegates copying the requested identifier to the page's clipboard owner.
function copyID(): void {
  emit('copy', props.entry.id)
}
</script>

<template>
  <article class="offer-card" :data-model-id="entry.id">
    <div class="offer-heading">
      <span class="offer-icon" aria-hidden="true"><ModelIcon :model="entry.id" size="30px" /></span>
      <h2 class="offer-name" :title="entry.id">{{ entry.id }}</h2>
      <div class="offer-actions"
        ><button
          type="button"
          class="offer-action catalog-detail-button"
          :aria-label="`${t('marketplace.catalogDetails')} ${entry.id}`"
          @click="openDetails"
          ><span class="offer-detail-label">{{ t('marketplace.catalogDetails') }}</span
          ><Icon name="chevronRight" size="sm" /></button
        ><button
          type="button"
          class="offer-action offer-copy"
          :aria-label="`${t('common.copy')} ${entry.id}`"
          @click="copyID"
          ><Icon name="copy" size="sm" /></button
      ></div>
    </div>
    <ModelPriceSummary
      :price="entry.price"
      :token-unit="tokenUnit"
      :cache-read="cacheRead"
      inline
    />
    <div class="offer-description">
      <p v-if="facts.length">{{ facts.join(' · ') }}</p>
      <p v-else>{{ t('marketplace.referenceNoDescription') }}</p>
      <p v-if="quote?.group.description" class="offer-group-description"
        >{{ t('marketplace.referenceGroupNote') }}{{ quote.group.description }}</p
      >
    </div>
    <div class="offer-footer">
      <div class="offer-metadata"
        ><strong>{{
          t(
            entry.price.kind !== 'unpriced' && entry.price.varies
              ? 'marketplace.catalogStartingPrice'
              : 'marketplace.catalogPrice',
          )
        }}</strong
        ><span>{{ t(`marketplace.referenceBilling.${entry.price.kind}`) }}</span
        ><p>{{ entry.brands.join(' · ') }}</p
        ><p>{{ t('marketplace.catalogOffers', { count: entry.groupCount }) }}</p
        ><p v-if="quote" class="offer-route">{{ quote.group.name }}</p></div
      >
      <dl class="offer-performance"
        ><div
          ><dt>{{ t('marketplace.referenceGroupLatency') }}</dt
          ><dd>{{ latency }}</dd></div
        ><div
          ><dt>{{ t('monitoring.cacheHitRate') }}</dt
          ><dd>{{ cacheHit }}</dd></div
        ><div
          ><dt>{{ t('marketplace.referenceGroupStatus') }}</dt
          ><dd
            class="offer-mini-bars"
            role="img"
            :aria-label="`${quote?.group.name ?? ''}: ${stats?.success_rate == null ? t('monitoring.noSamples') : `${stats.success_rate.toFixed(2)}%`}`"
            ><span
              v-for="(rate, index) in buckets"
              :key="index"
              :data-state="
                rate == null
                  ? 'empty'
                  : rate >= 0.99
                    ? 'healthy'
                    : rate >= 0.95
                      ? 'degraded'
                      : 'incident'
              "
              :title="
                rate == null ? t('monitoring.noSamples') : `${(rate * 100).toFixed(2)}%`
              " /></dd></div
      ></dl>
    </div>
  </article>
</template>

<style scoped>
/* Flat borders, aligned prices, and compact metadata follow the requested
   comparison layout while status colors retain independent semantic meaning. */
.offer-card {
  @apply flex min-w-0 flex-col border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-900;
  container-type: inline-size;
}
.offer-heading {
  @apply mb-4 grid min-w-0 items-start gap-2;
  grid-template-columns: 2.5rem minmax(0, 1fr) auto;
}
.offer-icon {
  @apply flex h-10 w-10 shrink-0 items-center justify-center bg-gray-50 dark:bg-dark-800;
}
.offer-name {
  @apply min-w-0 break-words font-mono text-base font-semibold leading-6;
  overflow-wrap: anywhere;
}
.offer-actions {
  @apply flex shrink-0 gap-1;
}
.offer-action {
  @apply inline-flex min-h-9 items-center gap-1 border border-gray-200 px-2 text-xs text-gray-500 dark:border-dark-600 dark:text-dark-300;
}
.offer-copy {
  @apply min-w-9 justify-center px-1;
}
.offer-description {
  @apply my-4 line-clamp-3 text-sm leading-6 text-gray-500 dark:text-dark-300;
  min-height: 4.5rem;
}
.offer-group-description {
  @apply mt-2;
}
.offer-footer {
  @apply mt-auto flex flex-wrap items-end justify-between gap-3;
}
.offer-metadata {
  @apply min-w-0 flex-1 text-xs leading-5 text-gray-500 dark:text-dark-300;
}
.offer-metadata > strong {
  @apply mr-2 font-medium;
}
.offer-route {
  @apply mt-1 break-words;
}
.offer-performance {
  @apply flex flex-wrap gap-3 text-right text-xs text-gray-500 dark:text-dark-300;
}
.offer-performance dd {
  @apply mt-1 font-mono text-gray-500 dark:text-dark-200;
}
.offer-mini-bars {
  @apply flex h-5 items-end gap-0.5;
}
.offer-mini-bars span {
  width: 3px;
  height: 100%;
}
[data-state='healthy'] {
  @apply bg-emerald-500;
}
[data-state='degraded'] {
  @apply bg-amber-400;
}
[data-state='incident'] {
  @apply bg-red-500;
}
[data-state='empty'] {
  @apply bg-gray-200 dark:bg-dark-600;
}
/* Below twenty-two rem of card content, icon actions reserve model-name space.
   Their accessible labels still name the complete model and requested action. */
@container (max-width: 22rem) {
  .offer-detail-label {
    display: none;
  }
}
/* Narrow cards give route metadata its own row so names cannot become vertical.
   Performance values retain their side-by-side comparison below the route. */
@container (max-width: 20rem) {
  .offer-footer {
    @apply flex-col items-stretch;
  }
  .offer-performance {
    @apply justify-between text-left;
  }
}
.offer-action:hover {
  @apply bg-gray-50 text-gray-950 dark:bg-dark-700 dark:text-white;
}
.offer-action:focus-visible {
  outline: 2px solid var(--tr-color-brand);
  outline-offset: 2px;
}
/* Match the shared sm breakpoint while preserving touch-sized card actions. */
@media (max-width: 639px) {
  .offer-action {
    @apply min-h-11;
  }
}
</style>
