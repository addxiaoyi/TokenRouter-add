<template>
  <div
    v-if="missing > 0"
    class="flex flex-wrap items-center justify-between gap-3 rounded-surface border border-amber-200 p-4 text-amber-800 dark:border-amber-800 dark:text-amber-200"
    role="status"
  >
    <p class="text-sm">{{ t('admin.profit.reminder', { count: missing }) }}</p>
    <RouterLink
      :to="{ path: '/admin/profit', query: { rates: 'missing' } }"
      class="btn btn-secondary btn-sm"
      >{{ t('admin.profit.confirm') }}</RouterLink
    >
    <details v-if="showProviders" class="basis-full text-sm">
      <summary class="cursor-pointer">{{ t('admin.profit.providers') }}</summary>
      <ul class="mt-3 max-h-panel space-y-2 overflow-y-auto">
        <li
          v-for="row in pendingProviders"
          :key="row.id"
          class="flex flex-wrap items-baseline gap-x-3 gap-y-1"
        >
          <span class="font-medium">{{ row.name }} #{{ row.id }}</span>
          <a
            v-if="row.site"
            :href="row.site"
            target="_blank"
            rel="noopener noreferrer"
            class="break-all underline"
          >
            {{ row.site }}
          </a>
          <span v-else>{{ t('admin.profit.noSite') }}</span>
          <span class="font-mono">{{
            row.has_key ? row.key_hint || t('admin.profit.keyConfigured') : t('admin.profit.noKey')
          }}</span>
        </li>
      </ul>
    </details>
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
import { readProfit, type ProfitProvider } from '@/api/admin/profit'

// ProfitReminder.vue surfaces unconfirmed upstream multipliers in administration.
// Settings can expand the provider list with website origins and masked key hints.
// Both surfaces link to the filtered profit page for manual confirmation. Failed
// reads offer a retry, since a failed request does not establish complete rates.

defineProps<{
  showProviders?: boolean // Allow inspection of all pending providers.
}>()

// This component owns one replaceable reminder request and discards responses
// after cancellation or unmount, preserving the current dashboard's lifetime.
const { t } = useI18n()
const { today } = useLocalDay()
const missing = ref(0)
const pendingProviders = ref<ProfitProvider[]>([])
const failed = ref(false)
let abort: AbortController | undefined

// refresh loads pending providers and their count without publishing cancelled reads.
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
    if (pending.signal.aborted) return
    pendingProviders.value = report.providers.filter((row) => !row.deleted && !row.configured)
    missing.value = report.missing_providers
  } catch {
    if (!pending.signal.aborted) failed.value = true
  }
}

onMounted(refresh)
onUnmounted(() => abort?.abort())
</script>
