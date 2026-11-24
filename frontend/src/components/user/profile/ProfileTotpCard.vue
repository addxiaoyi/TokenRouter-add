<template>
  <!-- Keep the status card in place while checking whether the administrator enabled two-factor authentication. -->
  <div v-if="(!status && loading) || status?.feature_enabled" class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.totp.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.totp.description') }}
      </p>
    </div>
    <div class="px-6 py-5">
      <!-- Loading state -->
      <div
        v-if="loading && !status"
        class="flex items-center justify-between gap-4"
        role="status"
        :aria-label="t('common.loading')"
        aria-busy="true"
        data-loading-skeleton
      >
        <div class="flex min-w-0 items-center gap-3" aria-hidden="true">
          <Skeleton variant="circle" :width="20" :height="20" class="shrink-0" />
          <div class="min-w-0 space-y-2">
            <Skeleton :width="98" :height="16" class="max-w-full" />
            <Skeleton :width="126" :height="14" class="max-w-full" />
          </div>
        </div>
        <Skeleton :width="64" :height="36" class="shrink-0" />
      </div>

      <!-- 2FA Enabled -->
      <div
        v-else-if="status?.enabled"
        class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
      >
        <div class="flex min-w-0 items-start gap-3">
          <Icon
            name="shieldCheck"
            size="md"
            :animate-on-hover="false"
            class="mt-0.5 shrink-0 text-green-600 dark:text-green-400"
          />
          <div class="min-w-0 space-y-1">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('profile.totp.enabled') }}
            </p>
            <p
              v-if="status.enabled_at"
              class="break-words text-xs leading-5 text-gray-500 dark:text-gray-400"
            >
              {{ t('profile.totp.enabledAt') }}: {{ formatDate(status.enabled_at) }}
            </p>
          </div>
        </div>
        <button
          type="button"
          class="btn btn-outline-danger justify-self-end"
          :disabled="loading"
          @click="showDisableDialog = true"
        >
          {{ t('profile.totp.disable') }}
        </button>
      </div>

      <!-- 2FA Not Enabled -->
      <div v-else-if="status" class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
        <div class="flex min-w-0 items-start gap-3">
          <Icon
            name="shield"
            size="md"
            :animate-on-hover="false"
            class="mt-0.5 shrink-0 text-gray-400"
          />
          <div class="min-w-0 space-y-1">
            <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('profile.totp.notEnabled') }}
            </p>
            <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('profile.totp.notEnabledHint') }}
            </p>
          </div>
        </div>
        <button
          type="button"
          class="btn btn-primary justify-self-end"
          :disabled="loading"
          @click="showSetupModal = true"
        >
          {{ t('profile.totp.enable') }}
        </button>
      </div>
    </div>

    <!-- Setup Modal -->
    <TotpSetupModal
      v-if="setupPresent"
      :show="showSetupModal"
      @after-leave="setupAfterLeave"
      @close="showSetupModal = false"
      @success="handleSetupSuccess"
    />

    <!-- Disable Dialog -->
    <TotpDisableDialog
      v-if="disablePresent"
      :show="showDisableDialog"
      @after-leave="disableAfterLeave"
      @close="showDisableDialog = false"
      @success="handleDisableSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import Skeleton from '@/components/common/Skeleton.vue'
import { useLeavingPresence } from '@/composables/useLeavingPresence'
import Icon from '@/components/icons/Icon.vue'
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { totpAPI } from '@/api'
import type { TotpStatus } from '@/api/totp.types'
import TotpSetupModal from './TotpSetupModal.vue'
import TotpDisableDialog from './TotpDisableDialog.vue'

// ProfileTotpCard.vue shows the account TOTP status and opens its setup or disable flow.
// Successful dialogs trigger a status refresh, and leaving-presence state keeps each dialog
// mounted until its shell completes the exit transition and cleanup.

// Translations describe the current two-factor status and the setup or disable action
// selected from the profile security section.
const { t } = useI18n()

// loading and status control the card summary; dialog visibility and leaving-presence state
// retain each credential dialog until its exit transition completes.
const loading = ref(true)
const status = ref<TotpStatus | null>(null)
const showSetupModal = ref(false)
const { present: setupPresent, afterLeave: setupAfterLeave } = useLeavingPresence(
  () => showSetupModal.value,
)
const showDisableDialog = ref(false)
const { present: disablePresent, afterLeave: disableAfterLeave } = useLeavingPresence(
  () => showDisableDialog.value,
)

// loadStatus refreshes the account's TOTP state and clears the loading indicator after
// completion. Request failure logs the cause while retaining any previously fetched status
// for the card.
const loadStatus = async () => {
  loading.value = true
  try {
    status.value = await totpAPI.getStatus()
  } catch (error) {
    console.error('Failed to load TOTP status:', error)
  } finally {
    loading.value = false
  }
}

// handleSetupSuccess closes the setup modal and refreshes the card's TOTP status.
const handleSetupSuccess = () => {
  showSetupModal.value = false
  loadStatus()
}

// handleDisableSuccess closes the disable dialog and refreshes the card's TOTP status.
const handleDisableSuccess = () => {
  showDisableDialog.value = false
  loadStatus()
}

// formatDate converts the API Unix timestamp into the browser's displayed date and time.
const formatDate = (timestamp: number) => {
  // Backend returns Unix timestamp in seconds, convert to milliseconds
  const date = new Date(timestamp * 1000)
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(() => {
  loadStatus()
})
</script>
