<script setup lang="ts">
import { useI18n } from 'vue-i18n'

// AsyncSurfaceNotice.vue reports delayed or failed loading of an optional interface
// surface without blocking the current page. Failed chunks offer a page refresh so
// the browser can resolve the current deployment's asset URLs before trying again.

// error is supplied by Vue's asynchronous component wrapper after loading fails.
defineProps<{ error?: Error /* Failed component import or loading timeout. */ }>()

// t supplies the shared loading, recovery, and refresh labels for this notice.
const { t } = useI18n()

// reloadPage requests current document and chunk URLs after an import failure.
function reloadPage(): void {
  window.location.reload()
}
</script>

<template>
  <div
    :role="error ? 'alert' : 'status'"
    :aria-busy="!error"
    class="fixed inset-x-4 top-[calc(var(--header-h)+1rem)] z-toast mx-auto flex max-w-md items-center justify-between gap-4 rounded-surface border border-gray-200 bg-white p-4 text-sm text-gray-900 shadow-lg dark:border-dark-700 dark:bg-dark-900 dark:text-dark-100"
  >
    <span>{{ error ? t('common.interfaceLoadFailed') : t('common.loading') }}</span>
    <button
      v-if="error"
      type="button"
      class="btn btn-secondary btn-sm shrink-0"
      @click="reloadPage"
    >
      {{ t('common.refresh') }}
    </button>
  </div>
</template>
