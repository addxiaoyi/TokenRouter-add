<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import AppHeader from '@/components/layout/AppHeader.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import GroupMonitorCard from '@/components/common/GroupMonitorCard.vue'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import ModelMarketplaceSkeleton from '@/components/marketplace/ModelMarketplaceSkeleton.vue'
import ModelCapabilityTags from '@/components/marketplace/ModelCapabilityTags.vue'
import ModelAttributeDetails from '@/components/marketplace/ModelAttributeDetails.vue'
import ModelPricingPanel from '@/components/marketplace/ModelPricingPanel.vue'
import ModelPriceSummary from '@/components/marketplace/ModelPriceSummary.vue'
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

// chooseGroup applies an authorized catalog identity and returns to model browsing.
// Selection changes local filtering only; it never reassigns an API key or grants
// access to a group absent from the authenticated catalog response.
function chooseGroup(id: number | 'all'): void {
  catalog.groupID.value = id
  groupsOpen.value = false
  mobileFiltersOpen.value = false
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
  <component
    :is="auth.isAuthenticated ? AppLayout : 'div'"
    :hide-page-heading="true"
    :class="
      auth.isAuthenticated ? '' : 'min-h-screen bg-gray-50 pt-[var(--header-h)] dark:bg-dark-900'
    "
  >
    <AppHeader v-if="!auth.isAuthenticated" public-page />
    <section class="catalog-page" :class="{ 'catalog-public': !auth.isAuthenticated }">
      <header class="catalog-heading">
        <div
          ><h1 class="text-2xl font-semibold tracking-tight">{{ t('marketplace.title') }}</h1
          ><p class="mt-2 text-sm text-gray-500 dark:text-dark-300">{{
            t('marketplace.catalogDescription', {
              count: catalog.allModels.value.length,
              groups: catalog.groups.value.length,
            })
          }}</p></div
        >
        <div class="catalog-search"
          ><label for="model-catalog-search" class="sr-only">{{
            t('marketplace.searchPlaceholder')
          }}</label
          ><Icon name="search" size="sm" class="text-gray-400" /><input
            id="model-catalog-search"
            v-model="catalog.search.value"
            class="min-w-0 flex-1 bg-transparent text-base outline-none"
            :placeholder="t('marketplace.searchPlaceholder')"
            data-testid="marketplace-search"
        /></div>
      </header>
      <div class="catalog-workspace">
        <aside
          class="catalog-sidebar"
          :class="{ 'catalog-sidebar-open': mobileFiltersOpen }"
          :aria-label="t('marketplace.catalogFilters')"
        >
          <div class="flex items-center justify-between gap-2"
            ><h2 class="text-sm font-semibold">{{ t('marketplace.catalogFilters') }}</h2
            ><button
              type="button"
              class="btn btn-secondary btn-icon catalog-sidebar-close"
              :aria-label="t('common.close')"
              @click="closeFilters"
              ><Icon name="x" size="sm" /></button
            ><button
              v-if="filtersActive"
              type="button"
              class="text-xs text-primary-500"
              @click="catalog.reset"
              >{{ t('common.reset') }}</button
            ></div
          >
          <section class="mt-5"
            ><h3 class="catalog-filter-title">{{ t('marketplace.filterGroup') }}</h3
            ><button
              type="button"
              class="catalog-filter catalog-group-filter"
              @click="groupsOpen = true"
              ><span class="min-w-0 break-words text-left">{{
                selectedGroup?.name ?? t('marketplace.allGroups')
              }}</span
              ><Icon name="chevronDown" size="sm" class="ml-auto shrink-0" /></button
            ><p class="mt-2 text-xs leading-5 text-gray-500 dark:text-dark-300">{{
              t('marketplace.catalogGroupHint')
            }}</p></section
          >
          <section class="mt-5"
            ><h3 class="catalog-filter-title">{{ t('marketplace.filterPricingMode') }}</h3
            ><button
              v-for="kind in pricingKinds"
              :key="kind"
              type="button"
              class="catalog-filter"
              :class="{ 'catalog-filter-active': catalog.pricing.value === kind }"
              @click="catalog.pricing.value = kind"
              >{{ t(`marketplace.${pricingLabels[kind]}`) }}</button
            ></section
          >
          <section class="mt-5"
            ><h3 class="catalog-filter-title">{{ t('marketplace.filterBrand') }}</h3>
            <div class="catalog-brand-list"
              ><button
                type="button"
                class="catalog-filter"
                :class="{ 'catalog-filter-active': catalog.brand.value === 'all' }"
                @click="catalog.brand.value = 'all'"
                ><span>{{ t('marketplace.allBrands') }}</span
                ><span class="catalog-filter-count">{{
                  catalog.allModels.value.length
                }}</span></button
              ><button
                v-for="brand in catalog.brands.value"
                :key="brand"
                type="button"
                class="catalog-filter"
                :class="{ 'catalog-filter-active': catalog.brand.value === brand }"
                @click="catalog.brand.value = brand"
                ><ProviderIcon :brand="brand" size="18px" hide-fallback /><span
                  class="min-w-0 flex-1 truncate"
                  >{{ brand }}</span
                ><span class="catalog-filter-count">{{ brandCounts.get(brand) }}</span></button
              ></div
            >
          </section>
        </aside>
        <div class="min-w-0 space-y-4">
          <div class="catalog-toolbar">
            <div class="flex flex-wrap items-center gap-3"
              ><button
                type="button"
                ref="filterToggle"
                class="btn btn-secondary catalog-mobile-filter"
                :aria-expanded="mobileFiltersOpen"
                @click="mobileFiltersOpen = !mobileFiltersOpen"
                ><Icon name="filter" size="sm" />{{ t('marketplace.catalogFilters') }}</button
              ><p class="text-sm"
                ><strong class="tabular-nums">{{ catalog.models.value.length }}</strong>
                {{ t('marketplace.catalogModels')
                }}<span class="ml-1 text-gray-400">/ {{ catalog.allModels.value.length }}</span></p
              ></div
            >
            <div class="flex flex-wrap items-center gap-2"
              ><label class="sr-only" for="catalog-sort">{{ t('marketplace.catalogSort') }}</label
              ><select
                id="catalog-sort"
                v-model="catalog.sort.value"
                class="input w-auto text-base sm:text-sm"
                ><option value="default">{{ t('marketplace.catalogDefaultOrder') }}</option
                ><option value="name">{{ t('marketplace.catalogNameOrder') }}</option></select
              ><div
                class="flex rounded-control border border-gray-200 p-1 dark:border-dark-600"
                :aria-label="t('marketplace.catalogTokenScale')"
                ><button
                  v-for="unit in ['M', 'K'] as const"
                  :key="unit"
                  type="button"
                  class="catalog-view-button"
                  :class="{ 'catalog-view-active': tokenUnit === unit }"
                  :aria-pressed="tokenUnit === unit"
                  @click="tokenUnit = unit"
                  >/1{{ unit }}</button
                ></div
              ><div
                class="flex rounded-control border border-gray-200 p-1 dark:border-dark-600"
                :aria-label="t('marketplace.catalogView')"
                ><button
                  type="button"
                  class="catalog-view-button"
                  :class="{ 'catalog-view-active': viewMode === 'cards' }"
                  :aria-pressed="viewMode === 'cards'"
                  @click="viewMode = 'cards'"
                  >{{ t('marketplace.catalogCards') }}</button
                ><button
                  type="button"
                  class="catalog-view-button"
                  :class="{ 'catalog-view-active': viewMode === 'table' }"
                  :aria-pressed="viewMode === 'table'"
                  @click="viewMode = 'table'"
                  >{{ t('marketplace.catalogTable') }}</button
                ></div
              ><button
                type="button"
                class="btn btn-secondary btn-icon"
                :disabled="catalog.loading.value"
                :aria-label="t('common.refresh')"
                @click="refresh"
                ><Icon name="refresh" size="sm" /></button
            ></div>
          </div>
          <ModelMarketplaceSkeleton v-if="catalog.loading.value" />
          <div v-else-if="catalog.error.value" class="card p-6" role="alert"
            ><h2 class="font-semibold">{{ t('common.error') }}</h2
            ><p class="mt-2 text-sm text-gray-500 dark:text-dark-300">{{ catalog.error.value }}</p
            ><button type="button" class="btn btn-primary mt-4" @click="refresh">{{
              t('common.retry')
            }}</button></div
          >
          <div v-else-if="!catalog.models.value.length" class="card px-5 py-16 text-center"
            ><h2 class="font-semibold">{{ t('marketplace.emptyTitle') }}</h2
            ><p class="mt-2 text-sm text-gray-500 dark:text-dark-300">{{
              t('marketplace.emptyDescription')
            }}</p
            ><button type="button" class="btn btn-secondary mt-4" @click="catalog.reset">{{
              t('common.reset')
            }}</button></div
          >
          <template v-else>
            <div v-if="viewMode === 'cards'" class="catalog-model-grid">
              <article
                v-for="entry in catalog.visible.value"
                :key="entry.id"
                class="catalog-model-card"
                :data-model-id="entry.id"
              >
                <div class="flex items-start gap-3"
                  ><ProviderIcon :brand="entry.brands[0] ?? ''" size="28px" hide-fallback /><div
                    class="min-w-0 flex-1"
                    ><h2 class="break-words text-base font-semibold">{{ entry.displayName }}</h2
                    ><p class="mt-1 truncate text-xs text-gray-500 dark:text-dark-300">{{
                      entry.brands.join(' · ')
                    }}</p></div
                  ></div
                >
                <div class="flex items-center justify-between gap-2"
                  ><code
                    class="min-w-0 truncate text-xs text-gray-500 dark:text-dark-300"
                    :title="entry.id"
                    >{{ entry.id }}</code
                  ><button
                    type="button"
                    class="shrink-0 text-gray-500 dark:text-dark-300"
                    :aria-label="`${t('common.copy')} ${entry.id}`"
                    @click="copyToClipboard(entry.id)"
                    ><Icon name="copy" size="sm" /></button
                ></div>
                <ModelCapabilityTags
                  v-if="catalog.representatives.value.get(entry.id)"
                  :model="catalog.representatives.value.get(entry.id)!"
                />
                <div class="mt-auto border-t border-gray-100 pt-4 dark:border-dark-600"
                  ><ModelPriceSummary :price="entry.price" :token-unit="tokenUnit"
                /></div>
                <button
                  type="button"
                  class="catalog-detail-button"
                  @click="catalog.openModel(entry.id)"
                  ><span>{{ t('marketplace.catalogOffers', { count: entry.groupCount }) }}</span
                  ><span class="inline-flex items-center gap-1"
                    >{{ t('marketplace.catalogDetails') }}<Icon name="arrowRight" size="sm" /></span
                ></button>
              </article>
            </div>
            <div v-else class="card overflow-x-auto"
              ><table class="w-full min-w-[540px] text-left text-sm"
                ><thead class="bg-gray-50 text-xs text-gray-500 dark:text-dark-300 dark:bg-dark-800"
                  ><tr
                    ><th class="p-4">{{ t('marketplace.catalogModel') }}</th
                    ><th class="p-4">{{ t('marketplace.catalogPrice') }}</th
                    ><th class="p-4">{{ t('marketplace.filterGroup') }}</th
                    ><th class="p-4"
                      ><span class="sr-only">{{ t('marketplace.catalogDetails') }}</span></th
                    ></tr
                  ></thead
                ><tbody
                  ><tr
                    v-for="entry in catalog.visible.value"
                    :key="entry.id"
                    class="border-t border-gray-100 dark:border-dark-600"
                    :data-model-id="entry.id"
                    ><td class="p-4"
                      ><button
                        type="button"
                        class="break-words text-left font-semibold"
                        @click="catalog.openModel(entry.id)"
                        >{{ entry.displayName }}</button
                      ><p
                        class="mt-1 max-w-64 truncate font-mono text-xs text-gray-500 dark:text-dark-300"
                        :title="entry.id"
                        >{{ entry.id }}</p
                      ></td
                    ><td class="p-4"
                      ><ModelPriceSummary :price="entry.price" :token-unit="tokenUnit" /></td
                    ><td class="p-4 tabular-nums">{{ entry.groupCount }}</td
                    ><td class="p-4"
                      ><button
                        type="button"
                        class="btn btn-secondary"
                        @click="catalog.openModel(entry.id)"
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
        </div>
      </div>
    </section>
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
        ><select id="catalog-offer" v-model="catalog.offerGroupID.value" class="input"
          ><option
            v-for="offer in catalog.offers.value"
            :key="offer.group.id"
            :value="offer.group.id"
            >{{ offer.group.name }}</option
          ></select
        >
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
  </component>
</template>

<style scoped>
/* Catalog surfaces use compact comparison lanes; the local table alone scrolls
   at narrow widths while the page, search, filters, and cards reflow naturally. */
.catalog-page {
  @apply min-w-0 space-y-6;
}
.catalog-public {
  @apply mx-auto max-w-[1560px] px-4 py-8 sm:px-6 lg:px-8;
}
.catalog-heading {
  @apply flex flex-wrap items-end justify-between gap-5;
}
.catalog-search {
  @apply flex min-w-0 flex-1 basis-80 items-center gap-2 rounded-control border border-gray-200 bg-white px-3 py-2.5 dark:border-dark-600 dark:bg-dark-800;
}
.catalog-workspace {
  @apply grid min-w-0 grid-cols-1 gap-5;
}
.catalog-sidebar {
  @apply hidden self-start rounded-surface border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800;
}
.catalog-sidebar-close {
  display: inline-flex;
}
.catalog-sidebar-open {
  @apply block;
}
.catalog-brand-list {
  max-height: 14rem;
  overflow-y: auto;
}
.catalog-filter-title {
  @apply mb-2 text-xs font-medium text-gray-500 dark:text-dark-300;
}
.catalog-filter {
  @apply mb-1 flex min-h-11 w-full sm:min-h-10 items-center gap-2 rounded-control px-2.5 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700;
}
.catalog-filter-active {
  background-color: var(--tr-color-brand);
  color: var(--tr-color-brand-contrast);
}
.catalog-filter-active:hover {
  background-color: var(--tr-color-brand);
  color: var(--tr-color-brand-contrast);
}
.catalog-filter:focus-visible,
.catalog-view-button:focus-visible,
.catalog-detail-button:focus-visible,
.catalog-group-grid > button:focus-visible {
  outline: 2px solid var(--tr-color-brand);
  outline-offset: 2px;
}
.catalog-filter-count {
  @apply ml-auto text-xs tabular-nums opacity-70;
}
.catalog-group-filter {
  @apply border border-gray-200 dark:border-dark-600;
}
.catalog-toolbar {
  @apply flex flex-wrap items-center justify-between gap-3 rounded-surface border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800;
}
.catalog-view-button {
  @apply min-h-11 rounded-control sm:min-h-8 px-2.5 text-xs;
}
.catalog-view-active {
  background-color: var(--tr-color-brand);
  color: var(--tr-color-brand-contrast);
}
.catalog-model-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr));
  gap: 1rem;
}
.catalog-model-card {
  @apply flex min-w-0 flex-col gap-3 rounded-surface border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800;
}
.catalog-detail-button {
  @apply mt-1 flex min-h-11 items-center justify-between sm:min-h-10 gap-2 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:text-dark-300 dark:border-dark-600;
}
.catalog-group-grid {
  @apply grid grid-cols-1 gap-4 sm:grid-cols-2;
}
@media (min-width: 1000px) {
  .catalog-workspace {
    grid-template-columns: 190px minmax(0, 1fr);
  }
  .catalog-sidebar {
    display: block;
    position: sticky;
    top: calc(var(--header-h) + 1rem);
    max-height: calc(100vh - var(--header-h) - 2rem);
    overflow-y: auto;
  }
  .catalog-mobile-filter,
  .catalog-sidebar-close {
    display: none;
  }
}
</style>
