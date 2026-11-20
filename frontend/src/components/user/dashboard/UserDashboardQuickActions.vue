<template>
  <div class="card">
    <!-- Match the announcement heading while keeping navigation rows compact. -->
    <div
      class="flex min-h-[61px] items-center border-b border-black/[0.07] px-6 py-4 dark:border-dark-600"
    >
      <h2
        class="truncate text-lg font-semibold tracking-[-0.015em] text-gray-950 dark:text-dark-50"
      >
        {{ t('dashboard.quickActions') }}
      </h2>
    </div>
    <div class="space-y-1.5 p-3">
      <button
        @click="router.push('/keys')"
        class="group flex w-full items-center gap-3 rounded-control bg-transparent p-3 text-left transition duration-fast hover:bg-black/[0.04] active:scale-[0.99] dark:hover:bg-white/[0.05]"
      >
        <div
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-control border border-black/[0.06] bg-gray-100/80 transition-transform duration-fast group-hover:scale-105 dark:border-dark-600 dark:bg-dark-800"
        >
          <Icon name="key" size="md" class="text-gray-700 dark:text-dark-100" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-950 dark:text-dark-50">
            {{ t('dashboard.createApiKey') }}
          </p>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('dashboard.generateNewKey') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-gray-700 dark:text-dark-500 dark:group-hover:text-dark-200"
          :animate-on-hover="false"
        />
      </button>

      <button
        @click="router.push('/usage')"
        class="group flex w-full items-center gap-3 rounded-control bg-transparent p-3 text-left transition duration-fast hover:bg-black/[0.04] active:scale-[0.99] dark:hover:bg-white/[0.05]"
      >
        <div
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-control border border-black/[0.06] bg-gray-100/80 transition-transform duration-fast group-hover:scale-105 dark:border-dark-600 dark:bg-dark-800"
        >
          <Icon name="chart" size="md" class="text-gray-700 dark:text-dark-100" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-950 dark:text-dark-50">
            {{ t('dashboard.viewUsage') }}
          </p>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('dashboard.checkDetailedLogs') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-gray-700 dark:text-dark-500 dark:group-hover:text-dark-200"
          :animate-on-hover="false"
        />
      </button>

      <button
        v-if="paymentEnabled"
        data-testid="purchase-quick-action"
        @click="router.push('/purchase')"
        class="group flex w-full items-center gap-3 rounded-control bg-transparent p-3 text-left transition duration-fast hover:bg-black/[0.04] active:scale-[0.99] dark:hover:bg-white/[0.05]"
      >
        <div
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-control border border-black/[0.06] bg-gray-100/80 transition-transform duration-fast group-hover:scale-105 dark:border-dark-600 dark:bg-dark-800"
        >
          <Icon name="creditCard" size="md" class="text-gray-700 dark:text-dark-100" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-950 dark:text-dark-50">
            {{ t('nav.buySubscription') }}
          </p>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('dashboard.purchasePlanOrRecharge') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-gray-700 dark:text-dark-500 dark:group-hover:text-dark-200"
          :animate-on-hover="false"
        />
      </button>

      <button
        @click="router.push('/redeem')"
        class="group flex w-full items-center gap-3 rounded-control bg-transparent p-3 text-left transition duration-fast hover:bg-black/[0.04] active:scale-[0.99] dark:hover:bg-white/[0.05]"
      >
        <div
          class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-control border border-black/[0.06] bg-gray-100/80 transition-transform duration-fast group-hover:scale-105 dark:border-dark-600 dark:bg-dark-800"
        >
          <Icon name="gift" size="md" class="text-gray-700 dark:text-dark-100" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-950 dark:text-dark-50">
            {{ t('dashboard.redeemCode') }}
          </p>
          <p class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('dashboard.addBalanceWithCode') }}
          </p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-gray-700 dark:text-dark-500 dark:group-hover:text-dark-200"
          :animate-on-hover="false"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

// UserDashboardQuickActions.vue provides compact links to account tasks from the
// dashboard. Public payment settings determine purchase visibility, while the remaining
// entries lead to the key, usage, and redemption pages. Every shortcut navigates through
// the application router using the current account session.

// Router, translations, and availability stores supply destinations and feature visibility
// for the dashboard shortcut buttons.
const router = useRouter()
const { t } = useI18n()
const appStore = useAppStore()
// paymentEnabled shows the purchase shortcut only after public settings report payment
// enabled; unavailable settings keep it hidden.
const paymentEnabled = computed(() => appStore.cachedPublicSettings?.payment_enabled === true)
</script>
