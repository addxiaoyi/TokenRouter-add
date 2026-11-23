import { useI18n } from 'vue-i18n'
import { computed, ref, watch, onBeforeUnmount, onMounted } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import { getMarketplaceModels, getAvailableMarketplaceModels } from '@/api/marketplace'
import type { MarketplaceGroup, MarketplaceModel } from '@/api/marketplace.types'
import {
  buildModelIndex,
  compareMarketplaceGroups,
  groupBrandSource,
} from '@/utils/marketplaceCatalog'
import { providerBrandDisplayName } from '@/utils/providerBrand'
import { pricingKind } from '@/utils/marketplacePricing'
import { useLocaleRefresh } from './useLocaleRefresh'

// useMarketplaceCatalog.ts owns catalog loading, filtering, and model selection.
// The API supplies group-specific offers, while browsing presents one row per
// model identity. Every detail offer retains its group and original quote so
// collapsing duplicate model names does not combine incompatible routing prices.
// Filters operate before model indexing and leave the received catalog immutable.
//
// Catalog reads capture account identity and a generation before awaiting. Logout,
// account replacement, and unmount retire that generation before private data can
// publish. Filter changes reset pagination, while quote selection resolves against
// the currently loaded group set. Public visitors use the public endpoint and
// authenticated visitors use the permission-filtered endpoint on every refresh.

// MarketplaceOffer pairs a model quote with its authorized routing group.
export interface MarketplaceOffer {
  group: MarketplaceGroup // Authorized routing group metadata.
  model: MarketplaceModel // Original group-specific model quote.
}

// PricingFilter identifies the billing category used by the catalog sidebar.
export type PricingFilter = 'all' | 'token' | 'image' | 'video' | 'per_request' | 'unpriced'

// useMarketplaceCatalog exposes account-scoped model browsing and original offer details.
// Call during component setup. Consumers supply translated presentation and retry
// controls; loading failures reject publication and leave an explicit error message.
export function useMarketplaceCatalog() {
  const { t } = useI18n()
  const auth = useAuthStore()
  const app = useAppStore()
  const groups = ref<MarketplaceGroup[]>([])
  const loading = ref(true)
  const error = ref('')
  const search = ref('')
  const brand = ref('all')
  const pricing = ref<PricingFilter>('all')
  const groupID = ref<number | 'all'>('all')
  const page = ref(1)
  const pageSize = 24
  const sort = ref<'default' | 'name'>('default')
  const selectedID = ref<string | null>(null)
  const offerGroupID = ref<number | null>(null)
  let generation = 0
  let ready = false
  let disposed = false

  // Catalog ordering is shared with global search; filter counts come from the
  // whole authorized response, rather than the subset visible on the current page.
  const orderedGroups = computed(() => [...groups.value].sort(compareMarketplaceGroups))
  const allModels = computed(() => buildModelIndex(orderedGroups.value))
  const brands = computed(() => [
    ...new Set(
      orderedGroups.value.map((group) => providerBrandDisplayName(groupBrandSource(group))),
    ),
  ])
  const filteredGroups = computed(() => {
    const query = search.value.trim().toLowerCase()
    return orderedGroups.value.flatMap((group) => {
      if (groupID.value !== 'all' && group.id !== groupID.value) return []
      if (
        brand.value !== 'all' &&
        providerBrandDisplayName(groupBrandSource(group)) !== brand.value
      )
        return []
      const groupMatch =
        !query ||
        [group.name, group.description, group.display_brand, ...(group.search_terms ?? [])].some(
          (text) => text?.toLowerCase().includes(query),
        )
      const models = group.models.filter((model) => {
        if (pricing.value !== 'all' && pricingKind(model.pricing) !== pricing.value) return false
        return (
          groupMatch ||
          [model.id, model.display_name, ...(model.attributes?.search_terms ?? [])].some((text) =>
            text?.toLowerCase().includes(query),
          )
        )
      })
      return models.length ? [{ ...group, models, model_count: models.length }] : []
    })
  })
  const models = computed(() => {
    const indexed = buildModelIndex(filteredGroups.value)
    return sort.value === 'name'
      ? indexed.sort((a, b) =>
          a.displayName.localeCompare(b.displayName, undefined, { numeric: true }),
        )
      : indexed
  })
  const visible = computed(() =>
    models.value.slice((page.value - 1) * pageSize, page.value * pageSize),
  )
  const representatives = computed(() => {
    const first = new Map<string, MarketplaceModel>()
    for (const group of filteredGroups.value) {
      for (const model of group.models) {
        if (!first.has(model.id)) first.set(model.id, model)
      }
    }
    return first
  })
  const offers = computed<MarketplaceOffer[]>(() =>
    orderedGroups.value.flatMap((group) => {
      if (groupID.value !== 'all' && group.id !== groupID.value) return []
      const model = group.models.find((entry) => entry.id === selectedID.value)
      return model ? [{ group, model }] : []
    }),
  )
  const selectedOffer = computed(
    () => offers.value.find((offer) => offer.group.id === offerGroupID.value) ?? offers.value[0],
  )

  // refresh replaces the catalog only while its captured account still owns the request.
  // Same-account retries may retain received groups, but errors remain explicit.
  // Identity changes clear groups synchronously before another render can expose
  // private models. Checking both account and generation also protects responses
  // whose token refresh completes during logout or a replacement account login.
  // Resource ownership ends on unmount, so locale callbacks cannot reopen retired reads.
  async function refresh(): Promise<void> {
    if (disposed) return
    const current = ++generation
    const userID = auth.user?.id
    const authenticated = auth.isAuthenticated
    const ownsRequest = () =>
      !disposed &&
      current === generation &&
      userID === auth.user?.id &&
      authenticated === auth.isAuthenticated
    loading.value = true
    error.value = ''
    try {
      const catalog = await (authenticated
        ? getAvailableMarketplaceModels()
        : getMarketplaceModels())
      if (!ownsRequest()) return
      if (
        !Array.isArray(catalog) ||
        catalog.some((group) => !Number.isFinite(group.id) || !Array.isArray(group.models))
      ) {
        throw new Error(t('marketplace.catalogLoadFailed'))
      }
      groups.value = catalog
      if (groupID.value !== 'all' && !catalog.some((group) => group.id === groupID.value))
        groupID.value = 'all'
    } catch (cause) {
      if (!ownsRequest()) return
      error.value = cause instanceof Error ? cause.message : t('marketplace.catalogLoadFailed')
    } finally {
      if (ownsRequest()) loading.value = false
    }
  }

  // reset clears local browsing restrictions while keeping the authorized catalog.
  function reset(): void {
    search.value = ''
    brand.value = 'all'
    pricing.value = 'all'
    groupID.value = 'all'
  }

  // openModel selects an identity and starts details at its first visible offer.
  function openModel(id: string): void {
    selectedID.value = id
    offerGroupID.value = offers.value[0]?.group.id ?? null
  }

  watch([search, brand, pricing, groupID, sort], () => {
    page.value = 1
  })
  watch(
    [() => auth.user?.id, () => auth.isAuthenticated],
    () => {
      // Retire pending catalog work before clearing private rows, preventing late
      // responses from restoring models after the account's visibility has changed.
      generation++
      groups.value = []
      selectedID.value = null
      if (ready) void refresh()
    },
    { flush: 'sync' },
  )
  onBeforeUnmount(() => {
    // Locale subscriptions can outlive an in-flight read; mark this owner retired
    // before incrementing its generation so neither path can publish later content.
    disposed = true
    generation++
  })
  onMounted(async () => {
    auth.checkAuth()
    if (!app.publicSettingsLoaded) await app.fetchPublicSettings()
    ready = true
    await refresh()
  })
  useLocaleRefresh(refresh)

  return {
    groups,
    orderedGroups,
    allModels,
    loading,
    error,
    search,
    brand,
    pricing,
    groupID,
    page,
    pageSize,
    sort,
    models,
    visible,
    brands,
    representatives,
    selectedID,
    offerGroupID,
    offers,
    selectedOffer,
    refresh,
    reset,
    openModel,
  }
}
