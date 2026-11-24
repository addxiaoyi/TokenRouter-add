<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import AppHeader from '@/components/layout/AppHeader.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores'
import {
  listQuality,
  qualityTargets,
  readQuality,
  startQuality,
  type QualityReport,
  type QualityTarget,
} from '@/api/modelQuality'

// ModelQualityView.vue presents fixed-suite model assessments and administrator
// execution progress. Public visitors see sanitized outcomes without provider
// identity or raw answers. Administration offers explicit paid test submission
// and recovers active jobs from the server after a page reload.
//
// One polling timer follows a running job and is cleared on unmount. Account or
// route changes invalidate outstanding requests before replacing private target
// choices. Incomplete reports retain no score, and detailed question outcomes
// distinguish wrong answers from operational failures. A submission identity
// survives transport failure so retry does not create another charged assessment.

// Page scope separates public report browsing from authenticated target selection.
const { t } = useI18n()
const auth = useAuthStore()
const route = useRoute()
const admin = computed(() => route.meta.requiresAdmin === true)
const reports = ref<QualityReport[]>([])
const targets = ref<QualityTarget[]>([])
const providerID = ref<number | null>(null)
const model = ref('')
const label = ref('')
const search = ref('')
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const active = ref<QualityReport | null>(null)
const selected = ref<QualityReport | null>(null)
let generation = 0
let clientID = crypto.randomUUID()
let timer: ReturnType<typeof setTimeout> | undefined
const providerOptions = computed(() =>
  targets.value.map((target) => ({ value: target.id, label: target.name })),
)
const modelOptions = computed(() =>
  (targets.value.find((target) => target.id === providerID.value)?.models ?? []).map((value) => ({
    value,
    label: value,
  })),
)
const visible = computed(() =>
  reports.value.filter((report) =>
    `${report.model} ${report.label}`
      .toLocaleLowerCase()
      .includes(search.value.toLocaleLowerCase()),
  ),
)

// load replaces current reports and private target choices for the active scope.
// Public routes never fetch administrator targets. Late results cannot replace
// a different account's scope, and failures keep an explicit retry action visible.
async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  if (timer) clearTimeout(timer)
  try {
    const history = await listQuality()
    if (current !== generation) return
    reports.value = history
    if (admin.value) {
      const loaded = await qualityTargets()
      if (current !== generation) return
      targets.value = loaded.targets
      active.value = loaded.active
      providerID.value = targets.value[0]?.id ?? null
      schedule()
    }
  } catch {
    if (current === generation) error.value = t('quality.loadFailed')
  } finally {
    if (current === generation) loading.value = false
  }
}

// schedule owns the next progress poll only while a job remains active.
function schedule() {
  if (timer) clearTimeout(timer)
  if (active.value && ['queued', 'running'].includes(active.value.status))
    timer = setTimeout(() => {
      void poll()
    }, 2000)
}

// poll refreshes one active report without overlapping prior progress requests.
// Failed reads retain the last progress and continue bounded polling. Completion
// refreshes public history, while account changes reject both responses.
async function poll() {
  if (!active.value) return
  const current = generation
  try {
    const report = await readQuality(active.value.id)
    if (current !== generation) return
    active.value = report
    if (!['queued', 'running'].includes(report.status)) {
      const history = await listQuality()
      if (current === generation) reports.value = history
      return
    }
  } catch {
    if (current === generation) error.value = t('quality.progressFailed')
  }
  if (current === generation) schedule()
}

// start submits a configured model assessment after the visible cost notice.
// Repeated clicks are blocked while admission is pending, and the UUID remains
// stable after transport failure. Successful admission begins server-owned polling.
async function start() {
  if (
    busy.value ||
    !providerID.value ||
    !model.value ||
    (active.value && ['queued', 'running'].includes(active.value.status))
  )
    return
  const current = generation
  busy.value = true
  error.value = ''
  try {
    const report = await startQuality(providerID.value, model.value, label.value, clientID)
    if (current !== generation) return
    active.value = report
    clientID = crypto.randomUUID()
    schedule()
  } catch {
    if (current === generation) error.value = t('quality.startFailed')
  } finally {
    if (current === generation) busy.value = false
  }
}

// closeDetails dismisses the selected public report without changing its stored results.
function closeDetails() {
  selected.value = null
}

// Model choices follow the selected provider; changing a submission creates a new identity.
watch(providerID, () => {
  model.value = modelOptions.value[0]?.value ?? ''
})
watch([providerID, model, label], () => {
  clientID = crypto.randomUUID()
})
watch(
  () => [route.path, auth.user?.id],
  () => {
    targets.value = []
    active.value = null
    selected.value = null
    busy.value = false
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
  if (timer) clearTimeout(timer)
})
</script>

<template>
  <component
    :is="auth.isAuthenticated ? AppLayout : 'div'"
    :class="auth.isAuthenticated ? '' : 'ba-theme-shell min-h-screen pt-[var(--header-h)]'"
  >
    <AppHeader v-if="!auth.isAuthenticated" public-page />
    <section
      :class="auth.isAuthenticated ? 'space-y-4' : 'mx-auto max-w-6xl space-y-4 px-4 py-8 sm:px-6'"
    >
      <div v-if="!auth.isAuthenticated" class="page-heading"
        ><h1 class="page-title">{{ t('quality.title') }}</h1
        ><p class="page-description">{{ t('quality.description') }}</p></div
      >
      <div class="card space-y-3 p-5"
        ><p class="text-sm leading-6 text-gray-500">{{ t('quality.explanation') }}</p
        ><div class="flex flex-wrap items-center gap-3"
          ><label class="min-w-0 flex-1"
            ><span class="sr-only">{{ t('quality.search') }}</span
            ><input
              v-model="search"
              class="input w-full"
              :placeholder="t('quality.search')" /></label
          ><button class="btn btn-secondary" :disabled="loading" @click="load"
            ><Icon name="refresh" size="sm" />{{ t('quality.refresh') }}</button
          ><RouterLink
            v-if="auth.user?.role === 'admin' && !admin"
            class="btn btn-primary"
            to="/admin/model-quality"
            >{{ t('quality.manage') }}</RouterLink
          ></div
        ></div
      >
      <p v-if="error" role="alert" class="card p-4 text-sm text-red-600 dark:text-red-400">{{
        error
      }}</p>
      <form v-if="admin" class="card space-y-4 p-5" @submit.prevent="start"
        ><h2 class="font-semibold">{{ t('quality.newTest') }}</h2
        ><div class="grid grid-cols-1 gap-4 lg:grid-cols-2"
          ><label class="min-w-0 space-y-2"
            ><span class="text-sm">{{ t('quality.provider') }}</span
            ><Select
              v-model="providerID"
              :options="providerOptions"
              searchable
              :disabled="busy || loading" /></label
          ><label class="min-w-0 space-y-2"
            ><span class="text-sm">{{ t('quality.model') }}</span
            ><Select
              v-model="model"
              :options="modelOptions"
              searchable
              :disabled="busy || loading" /></label></div
        ><label class="block space-y-2"
          ><span class="text-sm">{{ t('quality.label') }}</span
          ><input
            v-model="label"
            class="input w-full"
            maxlength="80"
            :placeholder="model"
            :disabled="busy" /></label
        ><p class="text-xs leading-6 text-gray-500">{{ t('quality.costNotice') }}</p
        ><p v-if="!loading && !targets.length" class="text-sm text-gray-500">{{
          t('quality.noTargets')
        }}</p
        ><button
          class="btn btn-primary"
          :disabled="
            busy || loading || !model || (!!active && ['queued', 'running'].includes(active.status))
          "
          >{{ busy ? t('quality.starting') : t('quality.start') }}</button
        ></form
      >
      <section v-if="active" class="card space-y-3 p-5" aria-live="polite"
        ><div class="flex flex-wrap justify-between gap-3"
          ><h2 class="font-semibold">{{ active.label }}</h2
          ><span class="text-sm">{{ t(`quality.${active.status}`) }}</span></div
        ><progress class="w-full" :value="active.probes.length" :max="active.total" /><p
          class="text-sm text-gray-500"
          >{{ t('quality.progress', { done: active.probes.length, total: active.total }) }}</p
        ><button class="btn btn-secondary" type="button" @click="selected = active">{{
          t('quality.details')
        }}</button></section
      >
      <div v-if="loading" role="status" class="card p-10 text-center text-sm text-gray-500">{{
        t('common.loading')
      }}</div>
      <div v-else-if="!visible.length" class="card p-12 text-center text-sm text-gray-500">{{
        t('quality.empty')
      }}</div>
      <div v-else class="space-y-3"
        ><article
          v-for="report in visible"
          :key="report.id"
          class="card flex flex-wrap items-center justify-between gap-5 p-5"
          ><div class="min-w-0 flex-1"
            ><h2 class="break-words font-semibold">{{ report.label }}</h2
            ><p class="mt-1 break-words text-xs text-gray-500"
              >{{ report.model }} · {{ report.suite_version }}</p
            ><p class="mt-2 text-xs text-gray-500">{{
              new Date(report.checked_at).toLocaleString()
            }}</p></div
          ><div class="text-right"
            ><p class="text-2xl font-semibold tabular-nums">{{
              report.score == null ? '—' : `${report.score}/100`
            }}</p
            ><p class="mt-1 text-xs text-gray-500">{{ t(`quality.${report.status}`) }}</p
            ><p v-if="report.delta != null" class="mt-1 text-xs">{{
              t('quality.delta', { value: report.delta > 0 ? `+${report.delta}` : report.delta })
            }}</p
            ><p
              v-if="report.signal === 'retest_needed'"
              class="mt-1 text-xs text-amber-600 dark:text-amber-400"
              >{{ t('quality.retest') }}</p
            ></div
          ><button class="btn btn-secondary" @click="selected = report">{{
            t('quality.details')
          }}</button></article
        ></div
      >
    </section>
    <BaseDialog
      :show="!!selected"
      :title="selected?.label ?? ''"
      trap-focus
      width="wide"
      @close="closeDetails"
      ><div v-if="selected" class="space-y-4"
        ><p class="text-sm text-gray-500">{{ t('quality.detailNote') }}</p
        ><ul class="divide-y divide-gray-100 dark:divide-dark-700"
          ><li
            v-for="probe in selected.probes"
            :key="probe.id"
            class="flex flex-wrap items-center justify-between gap-3 py-3"
            ><span class="min-w-0 flex-1 text-sm">{{ probe.title }}</span
            ><span class="text-xs text-gray-500">{{ probe.duration_ms }} ms</span
            ><span
              class="text-sm font-medium"
              :class="
                probe.status === 'pass'
                  ? 'text-green-600 dark:text-green-400'
                  : probe.status === 'fail'
                    ? 'text-amber-600 dark:text-amber-400'
                    : 'text-red-600 dark:text-red-400'
              "
              >{{ t(`quality.${probe.status}`) }}</span
            ></li
          ></ul
        ><p v-if="selected.baseline_at" class="text-xs text-gray-500">{{
          t('quality.baseline', { time: new Date(selected.baseline_at).toLocaleString() })
        }}</p></div
      ></BaseDialog
    >
  </component>
</template>
