<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { ModelPriceSummary } from '@/utils/marketplaceCatalog'
import { formatPriceNumber } from '@/utils/marketplacePricing'

// ModelPriceSummary.vue renders a model's comparable primary quote without hiding its unit.
// Catalog indexing selects the cheapest offer within one billing kind; varying
// quotes carry a visible starting-price label. Missing prices remain unavailable,
// and explicitly zero amounts remain valid free quotes across token and media units.

// Props retains the indexed quote and the caller's preferred token display unit;
// changing that unit converts presentation only and never modifies billing rates.
const props = withDefaults(
  defineProps<{
    price: ModelPriceSummary // Indexed comparable quote summary.
    tokenUnit?: 'M' | 'K' // Displayed token scale preference.
  }>(),
  { tokenUnit: 'M' },
)
const { t } = useI18n()
const { balanceUnitName } = useBalanceDisplay()
// tokenScale converts per-token quotes to the caller's display unit without
// modifying the selected offer or its monetary values retained by the catalog.
const tokenScale = computed(() => (props.tokenUnit === 'M' ? 1000000 : 1000))

// amount scales observed quotes to the selected display unit, retaining absence explicitly.
// Only token prices use the K/M conversion. Media and request quotes keep their
// original unit and use the same localized numeric formatter as other prices.
function amount(value: number | null, scale = 1): string {
  if (value == null) return '—'
  return `${formatPriceNumber(value * scale)} ${balanceUnitName.value}`
}
</script>

<template>
  <div v-if="price.kind === 'unpriced'" class="text-sm text-gray-500 dark:text-dark-300">{{
    t('marketplace.pricingUnavailable')
  }}</div>
  <div v-else class="space-y-2">
    <p class="text-xs text-gray-500 dark:text-dark-300"
      >{{ t(price.varies ? 'marketplace.catalogStartingPrice' : 'marketplace.catalogPrice') }} ·
      {{
        price.kind === 'token'
          ? `/${tokenUnit === 'M' ? '1M' : '1K'} tokens`
          : t(`marketplace.catalogUnit.${price.kind}`)
      }}</p
    >
    <dl v-if="price.kind === 'token'" class="grid grid-cols-2 gap-3 text-sm">
      <div
        ><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('marketplace.input') }}</dt
        ><dd class="mt-1 font-semibold tabular-nums">{{ amount(price.input, tokenScale) }}</dd></div
      >
      <div
        ><dt class="text-xs text-gray-500 dark:text-dark-300">{{ t('marketplace.output') }}</dt
        ><dd class="mt-1 font-semibold tabular-nums">{{
          amount(price.output, tokenScale)
        }}</dd></div
      >
    </dl>
    <p v-else class="text-sm font-semibold tabular-nums">{{ amount(price.amount) }}</p>
  </div>
</template>
