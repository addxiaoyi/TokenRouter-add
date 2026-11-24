<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore } from '@/stores'
import { keysAPI } from '@/api/keys'
import { buildGatewayUrl } from '@/api/url'
import type { ApiKey } from '@/api/keys.types'

// ChatConnectionsView.vue prepares explicit transfers of an API key to a web
// client. Loading the page reads owned active keys but creates no remote client
// navigation. The user first prepares links, reads the destination, and chooses
// which website receives the selected key through its supported configuration URL.
//
// Links live only in component memory and clear when account or selection changes.
// A generation check discards delayed account reads. No credentials are persisted
// in chat history or written to the clipboard by page load. The gateway origin
// comes from the same URL resolver used by built-in chat requests.

// Account scope owns key records, while explicit actions own prepared client links.
const { t } = useI18n()
const auth = useAuthStore()
const keys = ref<ApiKey[]>([])
const keyID = ref<number | null>(null)
const loading = ref(false)
const failed = ref(false)
const notice = ref('')
const links = ref<{ name: string; url: string }[]>([])
let generation = 0
const base = buildGatewayUrl('/v1')
const options = computed(() => keys.value.map((key) => ({ value: key.id, label: key.name })))

// load obtains all enabled personal keys and rejects responses from previous accounts.
// Paging is bounded to avoid unlimited work on a malformed server response.
// Failure keeps preparation disabled and exposes a user-triggered retry action.
async function load() {
  const current = ++generation
  keys.value = []
  keyID.value = null
  links.value = []
  failed.value = false
  loading.value = true
  try {
    const available: ApiKey[] = []
    for (let page = 1; page <= 100; page++) {
      const loaded = await keysAPI.list(page, 100, { scope: 'personal', status: 'active' })
      if (current !== generation) return
      available.push(...loaded.items.filter((key) => key.status === 'active'))
      if (page * 100 >= loaded.total) break
    }
    keys.value = available
    keyID.value = available[0]?.id ?? null
  } catch {
    if (current === generation) failed.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

// prepare builds client configuration URLs only after the user requests key transfer.
// The generated links name their destinations and remain private in component memory.
// The caller opens a link explicitly; preparation makes no third-party request.
function prepare() {
  const key = keys.value.find((item) => item.id === keyID.value)?.key
  if (!key) return
  links.value = [
    {
      name: 'NextChat',
      url: `https://app.nextchat.dev/#/?settings=${encodeURIComponent(JSON.stringify({ key, url: base }))}`,
    },
    {
      name: 'LobeChat',
      url: `https://chat-preview.lobehub.com/?${new URLSearchParams({ apiKey: key, apiBaseUrl: base })}`,
    },
  ]
}

// copyAddress copies the public gateway address without including any API credential.
async function copyAddress() {
  try {
    await navigator.clipboard.writeText(base)
    notice.value = t('chat.copied')
  } catch {
    notice.value = t('chat.copyFailed')
  }
}

// Changing selection invalidates prepared credentials before another key can be used.
watch(keyID, () => {
  links.value = []
  notice.value = ''
})
watch(
  () => auth.user?.id,
  (id) => {
    if (id) void load()
    else {
      generation++
      keys.value = []
      links.value = []
      keyID.value = null
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
  links.value = []
  keys.value = []
})
</script>

<template>
  <AppLayout>
    <section class="card max-w-3xl space-y-6 p-5 sm:p-6">
      <div class="space-y-2"
        ><h2 class="font-semibold">{{ t('chat.baseURL') }}</h2
        ><code class="block break-all rounded-surface bg-gray-50 p-3 text-sm dark:bg-dark-800">{{
          base
        }}</code
        ><button class="btn btn-secondary" @click="copyAddress"
          ><Icon name="copy" size="sm" />{{ t('chat.copyAddress') }}</button
        ></div
      >
      <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-else-if="failed" role="alert" class="space-y-3"
        ><p class="text-sm text-red-600 dark:text-red-400">{{ t('chat.connectionFailed') }}</p
        ><button class="btn btn-secondary" @click="load">{{ t('chat.retry') }}</button></div
      >
      <p v-else-if="!keys.length" class="text-sm text-gray-500"
        >{{ t('chat.noKeys') }}
        <RouterLink class="text-link" to="/keys">{{ t('chat.manageKeys') }}</RouterLink></p
      >
      <template v-else
        ><label class="block space-y-2"
          ><span class="text-sm font-medium">{{ t('chat.key') }}</span
          ><Select v-model="keyID" :options="options" /></label
        ><p class="rounded-surface bg-gray-50 p-4 text-sm leading-6 dark:bg-dark-800">{{
          t('chat.clientNotice')
        }}</p
        ><button class="btn btn-primary" :disabled="!keyID" @click="prepare">{{
          t('chat.prepare')
        }}</button
        ><div class="flex flex-wrap gap-3"
          ><a
            v-for="link in links"
            :key="link.name"
            class="btn btn-secondary"
            :href="link.url"
            target="_blank"
            rel="noopener noreferrer"
            referrerpolicy="no-referrer"
            >{{ t('chat.openClient', { client: link.name })
            }}<Icon name="externalLink" size="sm" /></a></div
      ></template>
      <p v-if="notice" role="status" class="text-sm text-gray-500">{{ notice }}</p>
      <RouterLink class="text-link" to="/chat">{{ t('chat.openWorkspace') }}</RouterLink>
    </section>
  </AppLayout>
</template>
