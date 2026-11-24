<template>
  <div
    v-if="missing > 0"
    class="flex flex-wrap items-center justify-between gap-3 rounded-surface border border-amber-200 p-4 text-amber-800 dark:border-amber-800 dark:text-amber-200"
    role="status"
  >
    <p class="text-sm">{{ t('admin.profit.reminder', { count: missing }) }}</p>
    <RouterLink to="/admin/profit" class="btn btn-secondary btn-sm">{{
      t('admin.profit.confirm')
    }}</RouterLink>
  </div>
  <div
    v-else-if="failed"
    class="flex flex-wrap items-center gap-3 text-sm text-gray-500"
    role="status"
  >
    <span>{{ t('admin.profit.reminderError') }}</span>
    <button type="button" class="btn btn-secondary btn-sm" @click="refresh">{{
      t('admin.profit.retry')
    }}</button>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useLocalDay } from '@/composables/useLocalDay'
import { readProfit } from '@/api/admin/profit'

// ProfitReminder.vue surfaces unconfirmed upstream multipliers on the dashboard.
// Its bounded report read exposes only the count and links to the protected profit
// page; it never requests credentials. Failed reads offer retry instead of claiming
// that every provider has a configured cost.

// This component owns one replaceable reminder request and discards responses
// after cancellation or unmount, preserving the current dashboard's lifetime.
const { t } = useI18n()
const { today } = useLocalDay()
const missing = ref(0)
const failed = ref(false)
let abort: AbortController | undefined

// refresh loads the current missing-rate count without publishing cancelled reads.
// An explicit retry replaces the previous request. Failure leaves an actionable
// message, while unmount aborts the request before it can update this component.
async function refresh(): Promise<void> {
  abort?.abort()
  const pending = new AbortController()
  abort = pending
  failed.value = false
  try {
    const report = await readProfit(
      today.value,
      today.value,
      Intl.DateTimeFormat().resolvedOptions().timeZone,
      pending.signal,
    )
    if (!pending.signal.aborted) missing.value = report.missing_providers
  } catch {
    if (!pending.signal.aborted) failed.value = true
  }
}

onMounted(refresh)
onUnmounted(() => abort?.abort())
</script>
