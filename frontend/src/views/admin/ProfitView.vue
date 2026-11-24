<template>
  <AppLayout>
    <template #page-heading-actions>
      <button
        class="btn btn-secondary"
        type="button"
        :disabled="loading || saving !== null"
        @click="refresh"
      >
        <Icon name="refresh" size="sm" />{{ t('common.refresh') }}
      </button>
    </template>

    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="max-w-2xl text-sm text-gray-500 dark:text-gray-400">{{
          t('admin.profit.description')
        }}</p>
        <DateRangePicker
          v-model:start-date="start"
          v-model:end-date="end"
          date-only
          @change="refresh"
        />
      </div>

      <div v-if="loading && !report" role="status" class="card p-8 text-center text-gray-500">{{
        t('common.loading')
      }}</div>
      <div v-else-if="error" role="alert" class="card space-y-3 p-6">
        <p>{{ error }}</p>
        <button class="btn btn-secondary" type="button" @click="refresh">{{
          t('admin.profit.retry')
        }}</button>
      </div>

      <template v-if="report">
        <section v-if="report.missing_providers" role="status" class="profit-notice">
          <Icon name="exclamationTriangle" size="md" class="shrink-0" />
          <div class="min-w-0 flex-1">
            <h2 class="font-semibold">{{
              t('admin.profit.reminder', { count: report.missing_providers })
            }}</h2>
            <p class="mt-1 text-sm">{{ t('admin.profit.reminderHint') }}</p>
          </div>
          <label class="flex shrink-0 items-center gap-2 text-sm">
            <input v-model="missingOnly" type="checkbox" class="checkbox" />{{
              t('admin.profit.missingOnly')
            }}
          </label>
        </section>

        <section
          class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
          :aria-label="t('admin.profit.summary')"
        >
          <div class="profit-metric">
            <p class="profit-label">{{ t('admin.profit.revenue') }}</p>
            <p class="profit-number">{{ money(report.revenue) }}</p>
            <p class="profit-caption">{{
              t('admin.profit.requests', { count: report.requests })
            }}</p>
          </div>
          <div class="profit-metric">
            <p class="profit-label">{{ t('admin.profit.upstreamCost') }}</p>
            <p class="profit-number">{{
              costText(report.upstream_cost, report.confirmed_requests, report.missing_requests)
            }}</p>
            <p class="profit-caption">{{
              t('admin.profit.coverage', {
                known: report.confirmed_requests,
                total: report.requests,
              })
            }}</p>
          </div>
          <div class="profit-metric">
            <p class="profit-label">{{ t('admin.profit.grossProfit') }}</p>
            <p
              class="profit-number"
              :class="report.gross_profit < 0 ? 'text-red-600 dark:text-red-400' : ''"
            >
              {{
                costText(report.gross_profit, report.confirmed_requests, report.missing_requests)
              }}
            </p>
            <p class="profit-caption">{{ t('admin.profit.formula') }}</p>
          </div>
          <div class="profit-metric">
            <p class="profit-label">{{ t('admin.profit.systemCost') }}</p>
            <p class="profit-number">{{
              costText(
                report.system_cost,
                report.system_requests - report.missing_system_requests,
                report.missing_system_requests,
              )
            }}</p>
            <p class="profit-caption">{{
              t('admin.profit.systemHint', { count: report.missing_system_requests })
            }}</p>
          </div>
        </section>

        <p v-if="report.missing_requests" class="text-sm text-amber-700 dark:text-amber-300">
          {{
            t('admin.profit.incomplete', {
              count: report.missing_requests,
              revenue: money(report.revenue - report.covered_revenue),
            })
          }}
        </p>

        <section class="space-y-4" :aria-labelledby="'profit-providers-title'">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h2 id="profit-providers-title" class="text-lg font-semibold">{{
              t('admin.profit.providers')
            }}</h2>
            <label class="flex w-full items-center gap-2 sm:w-80">
              <span class="sr-only">{{ t('admin.profit.search') }}</span>
              <input
                v-model="search"
                class="input"
                type="search"
                :placeholder="t('admin.profit.search')"
              />
            </label>
          </div>
          <p v-if="providers.length === 0" class="card p-8 text-center text-gray-500">{{
            t('admin.profit.emptyProviders')
          }}</p>

          <article v-for="row in providers" :key="row.id" class="profit-provider">
            <div class="min-w-0 space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="break-words font-semibold">{{ row.name }}</h3>
                <span class="text-xs text-gray-500">#{{ row.id }}</span>
                <span
                  v-if="!row.configured && !row.deleted"
                  class="text-xs font-medium text-amber-700 dark:text-amber-300"
                  >{{ t('admin.profit.needsRate') }}</span
                >
              </div>
              <a
                v-if="row.site"
                :href="row.site"
                target="_blank"
                rel="noopener noreferrer"
                class="block break-all text-sm underline underline-offset-4"
                >{{ row.site }}</a
              >
              <span v-else class="text-sm text-gray-500">{{ t('admin.profit.noSite') }}</span>
              <div class="flex flex-wrap items-center gap-2 text-sm">
                <span class="font-mono">{{
                  row.has_key
                    ? row.key_hint || t('admin.profit.keyConfigured')
                    : t('admin.profit.noKey')
                }}</span>
                <button
                  v-if="row.has_key && !row.deleted"
                  class="btn btn-secondary btn-sm"
                  type="button"
                  :disabled="revealing !== null"
                  @click="revealKey(row)"
                >
                  {{ revealing === row.id ? t('common.loading') : t('admin.profit.viewKey') }}
                </button>
              </div>
              <div v-if="secret?.id === row.id" class="space-y-2">
                <label :for="`profit-key-${row.id}`" class="sr-only">{{
                  t('admin.profit.upstreamKey')
                }}</label>
                <input
                  :id="`profit-key-${row.id}`"
                  class="input font-mono"
                  type="text"
                  readonly
                  :value="secret.text"
                  autocomplete="off"
                />
                <div class="flex gap-2">
                  <button class="btn btn-secondary btn-sm" type="button" @click="copyKey">{{
                    t('admin.profit.copyKey')
                  }}</button>
                  <button class="btn btn-secondary btn-sm" type="button" @click="hideKey">{{
                    t('admin.profit.hideKey')
                  }}</button>
                </div>
              </div>
            </div>

            <dl class="grid grid-cols-3 gap-3">
              <div
                ><dt class="profit-label">{{ t('admin.profit.revenue') }}</dt
                ><dd class="mt-1 break-all font-mono text-sm">{{ money(row.revenue) }}</dd></div
              >
              <div
                ><dt class="profit-label">{{ t('admin.profit.upstreamCost') }}</dt
                ><dd class="mt-1 break-all font-mono text-sm">{{
                  costText(row.upstream_cost, row.confirmed_requests, row.missing_requests)
                }}</dd></div
              >
              <div
                ><dt class="profit-label">{{ t('admin.profit.grossProfit') }}</dt
                ><dd
                  class="mt-1 break-all font-mono text-sm"
                  :class="row.gross_profit < 0 ? 'text-red-600 dark:text-red-400' : ''"
                  >{{
                    costText(row.gross_profit, row.confirmed_requests, row.missing_requests)
                  }}</dd
                ></div
              >
              <p class="col-span-3 text-xs text-gray-500"
                >{{
                  t('admin.profit.coverage', { known: row.confirmed_requests, total: row.requests })
                }}
                · {{ t('admin.profit.systemCost') }} {{ money(row.system_cost) }}</p
              >
            </dl>

            <div v-if="!row.deleted" class="space-y-2">
              <label :for="`profit-rate-${row.id}`" class="profit-label">{{
                t('admin.profit.rate')
              }}</label>
              <div class="flex gap-2">
                <input
                  :id="`profit-rate-${row.id}`"
                  v-model="drafts[row.id]"
                  class="input min-w-0"
                  type="text"
                  inputmode="decimal"
                  placeholder="0.25"
                  :disabled="saving !== null"
                />
                <button
                  class="btn btn-primary shrink-0"
                  type="button"
                  :disabled="saving !== null"
                  @click="saveRate(row)"
                  >{{ saving === row.id ? t('common.loading') : t('admin.profit.confirm') }}</button
                >
              </div>
              <p class="text-xs text-gray-500">{{ t('admin.profit.rateHint') }}</p>
            </div>
            <p v-else class="text-sm text-gray-500">{{ t('admin.profit.deleted') }}</p>
          </article>
        </section>

        <section class="space-y-3">
          <h2 class="text-lg font-semibold">{{ t('admin.profit.history') }}</h2>
          <p v-if="report.days.length === 0" class="card p-8 text-center text-gray-500">{{
            t('admin.profit.emptyHistory')
          }}</p>
          <div
            v-else
            class="overflow-x-auto rounded-surface border border-gray-200 dark:border-dark-600"
          >
            <table class="w-full text-sm">
              <thead class="text-left text-gray-500"
                ><tr
                  ><th class="p-3">{{ t('admin.profit.date') }}</th
                  ><th class="p-3">{{ t('admin.profit.revenue') }}</th
                  ><th class="p-3">{{ t('admin.profit.upstreamCost') }}</th
                  ><th class="p-3">{{ t('admin.profit.grossProfit') }}</th
                  ><th class="p-3">{{ t('admin.profit.systemCost') }}</th
                  ><th class="p-3">{{ t('admin.profit.missingRecords') }}</th></tr
                ></thead
              >
              <tbody class="divide-y divide-gray-200 dark:divide-dark-600"
                ><tr v-for="day in report.days" :key="day.date"
                  ><td class="whitespace-nowrap p-3">{{ day.date }}</td
                  ><td class="p-3 font-mono">{{ money(day.revenue) }}</td
                  ><td class="p-3 font-mono">{{
                    costText(day.upstream_cost, day.confirmed_requests, day.missing_requests)
                  }}</td
                  ><td class="p-3 font-mono">{{
                    costText(day.gross_profit, day.confirmed_requests, day.missing_requests)
                  }}</td
                  ><td class="p-3 font-mono">{{ money(day.system_cost) }}</td
                  ><td class="p-3">{{ day.missing_requests }}</td></tr
                ></tbody
              >
            </table>
          </div>
          <p class="text-xs text-gray-500">{{ t('admin.profit.retention') }}</p>
        </section>
      </template>
    </div>
    <TotpStepUpDialog :controller="stepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useProfitReport } from './profit/useProfitReport'

// ProfitView.vue presents customer margins, internal overhead, and upstream cost
// reminders in the administrator console. Unknown historical cost remains distinct
// from a confirmed zero. Provider cards expose masked key hints until explicit
// site-configured verification and place rate confirmation beside the upstream website.

// The page consumes one controller so refresh, edits, and credential cleanup share
// the same component lifetime; the existing dialog owns the verification interface.
const { t } = useI18n()
const {
  start,
  end,
  report,
  loading,
  error,
  search,
  missingOnly,
  providers,
  drafts,
  saving,
  revealing,
  secret,
  stepUp,
  refresh,
  saveRate,
  revealKey,
  hideKey,
  copyKey,
  money,
} = useProfitReport()
missingOnly.value = useRoute().query.rates === 'missing'

// costText keeps an unknown subtotal visually distinct from a measured zero cost.
function costText(amount: number, confirmed: number, missing: number): string {
  return confirmed === 0 && missing > 0 ? t('admin.profit.unknown') : money(amount)
}
</script>

<style scoped>
.profit-notice {
  @apply flex flex-wrap items-start gap-3 rounded-surface border border-amber-200 bg-transparent p-4 text-amber-800 dark:border-amber-800 dark:text-amber-200;
}
.profit-metric {
  @apply min-w-0 rounded-surface border border-gray-200 bg-transparent p-5 dark:border-dark-600;
}
.profit-label {
  @apply text-xs font-medium text-gray-500 dark:text-gray-400;
}
.profit-number {
  @apply mt-3 break-all font-mono text-2xl font-semibold tracking-tight;
}
.profit-caption {
  @apply mt-2 text-xs text-gray-500 dark:text-gray-400;
}
.profit-provider {
  @apply grid gap-5 rounded-surface border border-gray-200 bg-transparent p-4 dark:border-dark-600 xl:grid-cols-3;
}
</style>
