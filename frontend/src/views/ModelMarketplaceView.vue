<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppHeader from '@/components/layout/AppHeader.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Disclosure from '@/components/common/Disclosure.vue'
import GroupMonitorCard from '@/components/common/GroupMonitorCard.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelMarketplaceSkeleton from '@/components/marketplace/ModelMarketplaceSkeleton.vue'
import ModelCapabilityTags from '@/components/marketplace/ModelCapabilityTags.vue'
import ModelAttributeDetails from '@/components/marketplace/ModelAttributeDetails.vue'
import ModelPricingPanel from '@/components/marketplace/ModelPricingPanel.vue'
import ModelPriceSummary from '@/components/marketplace/ModelPriceSummary.vue'
import ModelOfferCard from '@/components/marketplace/ModelOfferCard.vue'
import { catalogQuote } from '@/utils/catalogQuote'
import GroupAvailabilityBar from '@/components/marketplace/GroupAvailabilityBar.vue'
import { useAuthStore } from '@/stores'
import { useClipboard } from '@/composables/useClipboard'
import { useMarketplaceCatalog, type PricingFilter } from '@/composables/useMarketplaceCatalog'
import { useGroupMonitoring } from '@/composables/useGroupMonitoring'
import { buildModelIndex, groupBrandSource } from '@/utils/marketplaceCatalog'
import { providerBrandDisplayName } from '@/utils/providerBrand'

// ModelMarketplaceView.vue presents model discovery, comparable quotes, and routing choices.
// Search and persistent sidebar filters precede a model-level catalog, while
// detailed pricing remains attached to each original group offer. Card and table
// views use the same indexed models and price summaries, so changing density does
// not change authorization, selected filters, or the meaning of quoted prices.
//
// Group selection opens measured monitoring cards beside the catalog metadata.
// Authenticated observations come from the permission-filtered monitoring reader;
// guests retain the public probe history when it exists. Detail dialogs keep
// model capabilities and complete price controls beside the selected offer,
// allowing users to compare routing choices without merging their distinct quotes.

// Browsing presentation belongs to this mounted page; catalog and measurement
// ownership remain encapsulated in composables that invalidate retired account scopes.
const route = useRoute()
const auth = useAuthStore()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const catalog = useMarketplaceCatalog()
const monitoringEnabled = computed(() => auth.isAuthenticated)
const monitoring = useGroupMonitoring(monitoringEnabled)
const viewMode = ref<'cards' | 'table'>('cards')
const tokenUnit = ref<'M' | 'K'>('M')
const mobileFiltersOpen = ref(false)
const filterToggle = ref<HTMLButtonElement | null>(null)
const groupsOpen = ref(false)
const groupSearch = ref('')
const pricingKinds: PricingFilter[] = ['all', 'token', 'image', 'video', 'per_request', 'unpriced']
const pricingLabels: Record<PricingFilter, string> = {
  all: 'allTypes',
  token: 'tokenPricing',
  image: 'imagePricing',
  video: 'videoPricing',
  per_request: 'requestPricing',
  unpriced: 'unpriced',
}
const brandCounts = computed(
  () =>
    new Map(
      catalog.brands.value.map((brand) => [
        brand,
        buildModelIndex(
          catalog.groups.value.filter(
            (group) => providerBrandDisplayName(groupBrandSource(group)) === brand,
          ),
        ).length,
      ]),
    ),
)
// Select options retain the catalog's typed values while sharing the application's
// accessible dropdown behavior and localized labels across both filter locations.
const brandOptions = computed(() => [
  { value: 'all', label: t('marketplace.allBrands') },
  ...catalog.brands.value.map((brand) => ({
    value: brand,
    label: `${brand} (${brandCounts.value.get(brand)})`,
  })),
])
const pricingOptions = computed(() =>
  pricingKinds.map((kind) => ({ value: kind, label: t(`marketplace.${pricingLabels[kind]}`) })),
)
const sortOptions = computed(() => [
  { value: 'default', label: t('marketplace.catalogDefaultOrder') },
  { value: 'name', label: t('marketplace.catalogNameOrder') },
])
const selectedGroup = computed(() =>
  catalog.groups.value.find((group) => group.id === catalog.groupID.value),
)
const groupChoices = computed(() =>
  catalog.orderedGroups.value.filter((group) => {
    const query = groupSearch.value.trim().toLowerCase()
    return (
      !query ||
      [group.name, group.description, ...(group.search_terms ?? [])].some((value) =>
        value?.toLowerCase().includes(query),
      )
    )
  }),
)
const filtersActive = computed(
  () =>
    catalog.brand.value !== 'all' ||
    catalog.pricing.value !== 'all' ||
    catalog.groupID.value !== 'all',
)
// selectedQuote retains one selected group's original billing unit and amount,
// independently of the cheaper cross-group summary shown in the catalog row.
const selectedQuote = computed(() => {
  const offer = catalog.selectedOffer.value
  return offer ? buildModelIndex([{ ...offer.group, models: [offer.model] }])[0]?.price : null
})

// Card quotes match the indexed price pair before borrowing cache prices or
// observations, so every compact card keeps one coherent original routing identity.
const cardQuotes = computed(
  () =>
    new Map(
      catalog.visible.value.map((entry) => [
        entry.id,
        catalogQuote(entry, catalog.filteredGroups.value),
      ]),
    ),
)
const countDigits = computed(() =>
  catalog.loading.value ? ['—'] : String(catalog.allModels.value.length).split(''),
)

// groupState labels observed group traffic without inventing a running state.
function groupState(id: number): string {
  const rate = monitoring.byID.value.get(id)?.success_rate
  return rate == null ? 'idle' : rate >= 99 ? 'healthy' : rate >= 95 ? 'degraded' : 'incident'
}

// chooseGroup applies an authorized catalog identity and returns to model browsing.
// Selection changes local filtering only; it never reassigns an API key or grants
// access to a group absent from the authenticated catalog response.
function chooseGroup(id: number | 'all'): void {
  catalog.groupID.value = id
  groupsOpen.value = false
  if (mobileFiltersOpen.value) void closeFilters()
}

// openDetails starts model details on the same offer shown in its catalog card.
// The catalog still owns authorized alternatives; this initial selection keeps
// a lower summary quote from opening a differently priced route by default.
function openDetails(id: string): void {
  catalog.openModel(id)
  const quote = cardQuotes.value.get(id)
  if (quote) catalog.offerGroupID.value = quote.group.id
}

// closeFilters returns narrow-screen browsing focus after hiding inline filters.
// The toggle remains mounted below the filter area. Waiting for the layout update
// keeps focus visible instead of leaving it inside the newly hidden sidebar.
async function closeFilters(): Promise<void> {
  mobileFiltersOpen.value = false
  await nextTick()
  filterToggle.value?.focus()
}

// refresh independently reloads model quotes and current group observations.
// Each reader owns its account scope and failure state, so unavailable metrics
// never prevent users from discovering models or inspecting a valid quote.
function refresh(): void {
  void catalog.refresh()
  void monitoring.refresh()
}

// Route search remains reactive for links from global search and monitoring.
// An absent query keeps the user's existing local filter during navigation.
watch(
  () => route.query.q,
  (query) => {
    if (typeof query === 'string') catalog.search.value = query
  },
  { immediate: true },
)
</script>

<template>
  <div class="reference-shell">
    <AppHeader public-page class="reference-header" />
    <main id="main-content" class="catalog-page catalog-public">
      <header class="reference-title-row">
        <h1>{{ t('marketplace.referenceEnabledModels') }}</h1>
        <output
          class="reference-model-count"
          :aria-label="`${catalog.allModels.value.length} ${t('marketplace.catalogModels')}`"
          ><span v-for="(digit, index) in countDigits" :key="index" aria-hidden="true">{{
            digit
          }}</span></output
        >
      </header>
      <Disclosure class="reference-billing" summary-class="reference-billing-summary">
        <template #summary
          ><span class="reference-billing-icon"><Icon name="order" size="lg" /></span
          ><span
            ><strong>{{ t('marketplace.referenceBillingTitle') }}</strong
            ><span class="reference-billing-copy">{{
              t('marketplace.referenceBillingHint')
            }}</span></span
          ><span class="reference-billing-toggle"
            >{{ t('marketplace.referenceBillingDetails')
            }}<Icon name="chevronDown" size="sm" /></span
        ></template>
        <p>{{ t('marketplace.referenceBillingExplanation') }}</p>
      </Disclosure>
      <div class="catalog-workspace">
        <aside
          class="catalog-sidebar"
          :class="{ 'catalog-sidebar-open': mobileFiltersOpen }"
          :aria-label="t('marketplace.catalogFilters')"
        >
          <div class="reference-filter-heading"
            ><div
              ><h2>{{ t('marketplace.catalogFilters') }}</h2
              ><p>{{ t('marketplace.referenceFilterHint') }}</p></div
            ><button
              type="button"
              class="reference-reset"
              :disabled="!filtersActive && !catalog.search.value"
              @click="catalog.reset"
              >{{ t('common.reset') }}</button
            ><button
              type="button"
              class="offer-toolbar-button catalog-sidebar-close"
              :aria-label="t('common.close')"
              @click="closeFilters"
              ><Icon name="x" size="sm" /></button
          ></div>
          <h3 class="catalog-filter-title">{{ t('marketplace.filterGroup') }}</h3>
          <button
            type="button"
            class="catalog-filter"
            :aria-pressed="catalog.groupID.value === 'all'"
            :class="{ 'catalog-filter-active': catalog.groupID.value === 'all' }"
            @click="chooseGroup('all')"
            ><span>{{ t('marketplace.referenceAllGroups') }}</span></button
          >
          <div class="reference-group-list"
            ><button
              v-for="group in catalog.orderedGroups.value"
              :key="group.id"
              type="button"
              class="catalog-filter"
              :aria-pressed="catalog.groupID.value === group.id"
              :class="{ 'catalog-filter-active': catalog.groupID.value === group.id }"
              :title="`${group.name}: ${group.description || t(`monitoring.${groupState(group.id)}`)}`"
              @click="chooseGroup(group.id)"
              ><span class="reference-group-name">{{ group.name }}</span
              ><span v-if="group.is_exclusive" class="reference-exclusive">{{
                t('marketplace.exclusive')
              }}</span
              ><span
                class="reference-status-dot"
                :data-status="groupState(group.id)"
                :title="t(`monitoring.${groupState(group.id)}`)"
              ></span
              ><span v-if="!group.hide_rate_multiplier" class="reference-group-ratio"
                >×{{ group.rate_multiplier }}</span
              ></button
            ></div
          >
          <Disclosure class="reference-more-filters" summary-class="reference-more-summary"
            ><template #summary>{{ t('marketplace.referenceMoreFilters') }}</template
            ><label for="reference-brand">{{ t('marketplace.filterBrand') }}</label
            ><Select
              id="reference-brand"
              v-model="catalog.brand.value"
              :options="brandOptions"
              :aria-label="t('marketplace.filterBrand')"
              class="reference-select"
            />
            <label for="reference-pricing">{{ t('marketplace.filterPricingMode') }}</label
            ><Select
              id="reference-pricing"
              v-model="catalog.pricing.value"
              :options="pricingOptions"
              :aria-label="t('marketplace.filterPricingMode')"
              class="reference-select"
            />
          </Disclosure>
          <button
            type="button"
            class="reference-monitor-link catalog-group-filter"
            @click="groupsOpen = true"
            >{{ t('marketplace.referenceCompareGroups') }}<Icon name="arrowRight" size="sm"
          /></button>
        </aside>
        <section class="reference-results" :aria-label="t('marketplace.title')">
          <div class="catalog-toolbar">
            <button
              ref="filterToggle"
              type="button"
              class="offer-toolbar-button catalog-mobile-filter"
              :aria-expanded="mobileFiltersOpen"
              @click="mobileFiltersOpen = !mobileFiltersOpen"
              ><Icon name="filter" size="sm" />{{ t('marketplace.catalogFilters') }}</button
            >
            <p class="reference-result-count"
              ><strong>{{ catalog.loading.value ? '—' : catalog.models.value.length }}</strong>
              {{ t('marketplace.catalogModels') }}</p
            >
            <div class="catalog-search"
              ><Icon name="search" size="sm" /><label class="sr-only" for="model-catalog-search">{{
                t('marketplace.searchPlaceholder')
              }}</label
              ><input
                id="model-catalog-search"
                v-model="catalog.search.value"
                :placeholder="t('marketplace.referenceSearch')"
                data-testid="marketplace-search"
            /></div>
            <div class="reference-toolbar-options"
              ><span class="reference-standard" :title="selectedGroup?.name">{{
                t('marketplace.referenceQuotedPrice')
              }}</span
              ><div class="reference-segment" :aria-label="t('marketplace.catalogTokenScale')"
                ><button
                  v-for="unit in ['M', 'K'] as const"
                  :key="unit"
                  type="button"
                  :class="{ 'reference-active': tokenUnit === unit }"
                  :aria-pressed="tokenUnit === unit"
                  @click="tokenUnit = unit"
                  >/1{{ unit }}</button
                ></div
              ><label class="sr-only" for="catalog-sort">{{ t('marketplace.catalogSort') }}</label
              ><Select
                id="catalog-sort"
                v-model="catalog.sort.value"
                :options="sortOptions"
                :aria-label="t('marketplace.catalogSort')"
                :searchable="false"
                class="reference-sort reference-select" />
              <div class="reference-segment" :aria-label="t('marketplace.catalogView')"
                ><button
                  type="button"
                  :class="{ 'reference-active': viewMode === 'cards' }"
                  :aria-pressed="viewMode === 'cards'"
                  @click="viewMode = 'cards'"
                  >{{ t('marketplace.catalogCards') }}</button
                ><button
                  type="button"
                  :class="{ 'reference-active': viewMode === 'table' }"
                  :aria-pressed="viewMode === 'table'"
                  @click="viewMode = 'table'"
                  >{{ t('marketplace.catalogTable') }}</button
                ></div
              ><button
                type="button"
                class="offer-toolbar-button"
                :disabled="catalog.loading.value"
                :aria-label="t('common.refresh')"
                @click="refresh"
                ><Icon name="refresh" size="sm" /></button
            ></div>
          </div>
          <ModelMarketplaceSkeleton v-if="catalog.loading.value" />
          <div v-else-if="catalog.error.value" class="reference-empty" role="alert"
            ><h2>{{ t('common.error') }}</h2
            ><p>{{ catalog.error.value }}</p
            ><button type="button" class="btn btn-primary" @click="refresh">{{
              t('common.retry')
            }}</button></div
          >
          <div v-else-if="!catalog.models.value.length" class="reference-empty"
            ><h2>{{ t('marketplace.emptyTitle') }}</h2
            ><p>{{ t('marketplace.emptyDescription') }}</p
            ><button type="button" class="btn btn-secondary" @click="catalog.reset">{{
              t('common.reset')
            }}</button></div
          >
          <template v-else>
            <div v-if="viewMode === 'cards'" class="catalog-model-grid"
              ><ModelOfferCard
                v-for="entry in catalog.visible.value"
                :key="entry.id"
                :entry="entry"
                :quote="cardQuotes.get(entry.id)"
                :stats="monitoring.byID.value.get(cardQuotes.get(entry.id)?.group.id ?? -1)"
                :window-end="monitoring.snapshot.value?.end"
                :token-unit="tokenUnit"
                @details="openDetails"
                @copy="copyToClipboard"
            /></div>
            <div v-else class="reference-table"
              ><table
                ><thead
                  ><tr
                    ><th>{{ t('marketplace.catalogModel') }}</th
                    ><th>{{ t('marketplace.catalogPrice') }}</th
                    ><th>{{ t('marketplace.filterGroup') }}</th
                    ><th>{{ t('marketplace.catalogDetails') }}</th></tr
                  ></thead
                ><tbody
                  ><tr
                    v-for="entry in catalog.visible.value"
                    :key="entry.id"
                    :data-model-id="entry.id"
                    ><td
                      ><button type="button" @click="openDetails(entry.id)">{{
                        entry.id
                      }}</button></td
                    ><td><ModelPriceSummary :price="entry.price" :token-unit="tokenUnit" /></td
                    ><td>{{ entry.groupCount }}</td
                    ><td
                      ><button
                        type="button"
                        class="offer-toolbar-button"
                        @click="openDetails(entry.id)"
                        >{{ t('marketplace.catalogDetails') }}</button
                      ></td
                    ></tr
                  ></tbody
                ></table
              ></div
            >
            <Pagination
              :total="catalog.models.value.length"
              :page="catalog.page.value"
              :page-size="catalog.pageSize"
              :show-page-size-selector="false"
              @update:page="catalog.page.value = $event"
            />
          </template>
        </section>
      </div>
    </main>
    <BaseDialog
      :show="groupsOpen"
      :title="t('marketplace.filterGroup')"
      width="wide"
      trap-focus
      @close="groupsOpen = false"
    >
      <label class="sr-only" for="catalog-group-search">{{ t('keys.searchGroup') }}</label
      ><input
        id="catalog-group-search"
        v-model="groupSearch"
        class="input mb-4 text-base"
        :placeholder="t('keys.searchGroup')"
      />
      <div
        v-if="monitoring.failed.value"
        class="mb-4 flex flex-wrap items-center justify-between gap-2 text-sm text-red-600"
        role="alert"
        ><span>{{ t('monitoring.refreshFailed') }}</span
        ><button type="button" class="btn btn-secondary" @click="monitoring.refresh">{{
          t('common.retry')
        }}</button></div
      >
      <button type="button" class="btn btn-secondary mb-4 w-full" @click="chooseGroup('all')">{{
        t('marketplace.allGroups')
      }}</button>
      <div class="catalog-group-grid"
        ><button
          v-for="group in groupChoices"
          :key="group.id"
          type="button"
          class="min-w-0 rounded-surface text-left"
          @click="chooseGroup(group.id)"
          ><GroupMonitorCard
            :name="group.name"
            :description="group.description"
            :multiplier="group.hide_rate_multiplier ? null : group.rate_multiplier"
            :reference-multiplier="group.hide_max_discount ? null : group.official_price_ratio"
            :model-count="group.model_count"
            :stats="monitoring.byID.value.get(group.id)"
            :window-end="monitoring.snapshot.value?.end"
            :selected="catalog.groupID.value === group.id" /><GroupAvailabilityBar
            v-if="!auth.isAuthenticated && group.availability"
            :availability="group.availability"
            class="mt-2" /></button
      ></div>
      <p
        v-if="!groupChoices.length"
        class="py-8 text-center text-sm text-gray-500 dark:text-dark-300"
        >{{ t('keys.noGroupFound') }}</p
      >
    </BaseDialog>
    <BaseDialog
      :show="!!catalog.selectedOffer.value"
      :title="catalog.selectedOffer.value?.model.display_name ?? ''"
      width="wide"
      trap-focus
      @close="catalog.selectedID.value = null"
    >
      <div v-if="catalog.selectedOffer.value" class="space-y-5">
        <div class="flex flex-wrap items-center gap-3"
          ><code class="min-w-0 break-all text-sm">{{ catalog.selectedOffer.value.model.id }}</code
          ><button
            type="button"
            class="btn btn-secondary"
            @click="copyToClipboard(catalog.selectedOffer.value.model.id)"
            >{{ t('common.copy') }}</button
          ><ModelAttributeDetails :model="catalog.selectedOffer.value.model"
        /></div>
        <ModelCapabilityTags :model="catalog.selectedOffer.value.model" />
        <label for="catalog-offer" class="input-label">{{ t('marketplace.filterGroup') }}</label
        ><Select
          id="catalog-offer"
          v-model="catalog.offerGroupID.value"
          :options="
            catalog.offers.value.map((offer) => ({
              value: offer.group.id,
              label: offer.group.name,
            }))
          "
          :aria-label="t('marketplace.filterGroup')"
        />
        <GroupMonitorCard
          :name="catalog.selectedOffer.value.group.name"
          :description="catalog.selectedOffer.value.group.description"
          :multiplier="
            catalog.selectedOffer.value.group.hide_rate_multiplier
              ? null
              : catalog.selectedOffer.value.group.rate_multiplier
          "
          :reference-multiplier="
            catalog.selectedOffer.value.group.hide_max_discount
              ? null
              : catalog.selectedOffer.value.group.official_price_ratio
          "
          :stats="monitoring.byID.value.get(catalog.selectedOffer.value.group.id)"
          :window-end="monitoring.snapshot.value?.end"
        />
        <ModelPriceSummary v-if="selectedQuote" :price="selectedQuote" :token-unit="tokenUnit" />
        <ModelPricingPanel
          :key="catalog.selectedOffer.value.group.id"
          :model="catalog.selectedOffer.value.model"
        />
      </div>
    </BaseDialog>
  </div>
</template>

<style scoped>
/* The reference layout uses a wide white comparison surface, rectangular cards,
   and aligned controls. Dark mode changes surrounding surfaces while selections
   retain the designated brand foreground rather than borrowing an accent shade. */
.reference-shell {
  @apply min-h-screen bg-white pt-[var(--header-h)] text-gray-950 dark:bg-dark-900 dark:text-white;
}
.reference-header :deep(a[href='/login']) {
  white-space: nowrap;
  flex-shrink: 0;
}
.catalog-public {
  @apply mx-auto w-full max-w-[1800px] px-4 pb-10 pt-7 sm:px-6;
}
.reference-title-row {
  @apply mb-6 flex items-center gap-3;
}
.reference-title-row h1 {
  @apply text-lg font-semibold;
}
.reference-model-count {
  @apply inline-flex gap-1;
}
.reference-model-count span {
  @apply inline-flex h-9 min-w-8 items-center justify-center border border-gray-100 bg-gray-50 font-mono text-lg font-semibold dark:border-dark-600 dark:bg-dark-800;
}
.reference-billing {
  @apply mb-6 border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800;
}
.reference-billing :deep(.reference-billing-summary) {
  @apply flex w-full cursor-pointer items-center gap-3 p-4 text-left;
}
.reference-billing :deep(.reference-billing-summary > svg) {
  display: none;
}
.reference-billing-icon {
  @apply flex h-10 w-10 shrink-0 items-center justify-center bg-gray-200 dark:bg-dark-700;
}
.reference-billing strong {
  @apply block text-sm font-semibold;
}
.reference-billing-copy {
  @apply mt-1 block text-sm leading-6 text-gray-500 dark:text-dark-300;
}
.reference-billing-toggle {
  @apply ml-auto inline-flex shrink-0 items-center gap-1 text-xs;
}
.reference-billing p {
  @apply border-t border-gray-200 px-4 py-3 text-sm leading-6 text-gray-500 dark:border-dark-600 dark:text-dark-300;
}
.catalog-workspace {
  @apply grid min-w-0 grid-cols-1 items-start gap-4;
}
.catalog-sidebar {
  @apply hidden min-w-0 border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-900;
}
.catalog-sidebar-open {
  @apply block;
}
.reference-filter-heading {
  @apply mb-5 flex items-start gap-2;
}
.reference-filter-heading h2 {
  @apply text-base font-semibold;
}
.reference-filter-heading p {
  @apply mt-1 text-xs leading-5 text-gray-500 dark:text-dark-300;
}
.reference-reset {
  @apply ml-auto min-h-9 shrink-0 text-xs text-gray-500 disabled:opacity-50 dark:text-dark-300;
}
.catalog-filter-title {
  @apply mb-3 text-sm font-semibold;
}
.reference-group-list {
  @apply min-w-0 overflow-y-auto;
  max-height: calc(100vh - var(--header-h) - 16rem);
}
.catalog-filter {
  @apply flex min-h-11 w-full items-center gap-2 px-2.5 py-2.5 text-left text-sm text-gray-600 hover:bg-gray-50 dark:text-dark-200 dark:hover:bg-dark-800;
}
.catalog-filter-active {
  @apply bg-gray-100 font-semibold dark:bg-dark-700;
  color: var(--tr-color-brand);
}
.catalog-filter-active:hover {
  @apply bg-gray-100 dark:bg-dark-700;
}
.reference-group-name {
  @apply min-w-0 flex-1 truncate;
}
.reference-status-dot {
  @apply h-1.5 w-1.5 shrink-0 rounded-full;
}
[data-status='healthy'] {
  @apply bg-emerald-500;
}
[data-status='degraded'] {
  @apply bg-amber-500;
}
[data-status='incident'] {
  @apply bg-red-500;
}
[data-status='idle'] {
  @apply bg-gray-300 dark:bg-dark-500;
}
.reference-group-ratio {
  @apply shrink-0 bg-gray-50 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-300;
}
.reference-exclusive {
  @apply shrink-0 text-xs text-amber-700 dark:text-amber-400;
}
.reference-more-filters {
  @apply mt-5 border-t border-gray-100 pt-3 text-xs dark:border-dark-600;
}
.reference-more-filters :deep(.reference-more-summary) {
  @apply flex w-full cursor-pointer items-center gap-2 py-2;
}
.reference-more-filters label {
  @apply mb-1 mt-3 block;
}
.reference-select :deep(.input) {
  @apply text-base;
  border-radius: 0;
}
.reference-monitor-link {
  @apply mt-4 flex min-h-10 w-full items-center justify-between border-t border-gray-100 pt-3 text-xs dark:border-dark-600;
}
.reference-results {
  @apply min-w-0 space-y-4;
}
.catalog-toolbar {
  @apply flex min-w-0 flex-wrap items-center gap-2 border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900;
}
.reference-result-count {
  @apply shrink-0 text-sm;
}
.catalog-search {
  @apply flex min-w-0 flex-1 basis-48 items-center gap-2 border border-gray-200 px-3 py-2 text-gray-400 dark:border-dark-600;
}
.catalog-search input {
  @apply min-w-0 flex-1 bg-transparent text-base text-gray-950 outline-none dark:text-white;
}
.reference-toolbar-options {
  @apply ml-auto flex flex-wrap items-center gap-2;
}
.reference-standard {
  @apply border border-gray-200 px-2 py-2 text-xs dark:border-dark-600;
}
.reference-segment {
  @apply inline-flex border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800;
}
.reference-segment button {
  @apply min-h-9 px-2 text-xs;
}
.reference-active {
  @apply bg-white shadow-sm dark:bg-dark-700;
  color: var(--tr-color-brand);
}
.offer-toolbar-button {
  @apply inline-flex min-h-9 items-center justify-center gap-1 border border-gray-200 bg-transparent px-2 text-xs dark:border-dark-600;
}
.reference-sort {
  @apply w-32;
}
.catalog-model-grid {
  @apply grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3;
}
.reference-table {
  @apply min-w-0 overflow-x-auto border border-gray-200 dark:border-dark-600;
}
.reference-table table {
  @apply w-full min-w-[540px] text-left text-sm;
}
.reference-table th,
.reference-table td {
  @apply border-b border-gray-100 p-4 dark:border-dark-600;
}
.reference-table th {
  @apply bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-300;
}
.reference-table td:first-child {
  @apply font-mono;
}
.reference-empty {
  @apply border border-gray-200 p-6 dark:border-dark-600;
}
.reference-empty h2 {
  @apply font-semibold;
}
.reference-empty p {
  @apply mb-4 mt-2 text-sm text-gray-500 dark:text-dark-300;
}
.catalog-group-grid {
  @apply grid grid-cols-1 gap-4 sm:grid-cols-2;
}
.catalog-filter:focus-visible,
.reference-segment button:focus-visible,
.reference-reset:focus-visible,
.reference-monitor-link:focus-visible,
.reference-billing :deep(button:focus-visible),
.catalog-search:focus-within,
.offer-toolbar-button:focus-visible {
  outline: 2px solid var(--tr-color-brand);
  outline-offset: 2px;
}
.catalog-sidebar-close {
  @apply inline-flex;
}
/* The persistent group rail follows the shared lg navigation breakpoint. */
@media (min-width: 1024px) {
  .catalog-workspace {
    grid-template-columns: minmax(210px, 21%) minmax(0, 1fr);
  }
  .catalog-sidebar {
    display: block;
    position: sticky;
    top: calc(var(--header-h) + 1rem);
  }
  .catalog-mobile-filter,
  .catalog-sidebar-close {
    display: none;
  }
}
/* Below the shared sm breakpoint, controls wrap into touch-sized rows. */
@media (max-width: 639px) {
  .reference-billing-toggle {
    @apply text-xs;
  }
  .reference-billing-copy {
    @apply text-xs;
  }
  .reference-standard {
    display: none;
  }
  .reference-segment button,
  .offer-toolbar-button {
    @apply min-h-11;
  }
  .reference-group-list {
    max-height: 18rem;
  }
}
</style>
