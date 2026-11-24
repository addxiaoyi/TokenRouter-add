<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useChatWorkspace } from '@/composables/useChatWorkspace'
import { useAuthStore } from '@/stores'

// ChatView.vue presents a conversation workspace with a compact local history
// rail and a persistent composer. Controls use the existing surface and brand
// tokens, while narrow layouts place history behind an explicit toggle. Message
// text remains selectable, wraps long code, and never executes model-provided HTML.
//
// The workspace composable owns account isolation and streaming. This view owns
// only presentation, local file import, download, and confirmation dialogs. File
// imports verify the active workspace before publication, and object URLs are
// revoked after the browser has received their download action. Export always
// contains conversation text and excludes the selected API credential.

// Workspace state and display filters remain private to this mounted page.
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const chat = useChatWorkspace()
const auth = useAuthStore()
const {
  sessions,
  active,
  activeID,
  keys,
  keyID,
  models,
  model,
  prompt,
  system,
  loading,
  sending,
  error,
  storageError,
  tokens,
  elapsed,
  firstToken,
} = chat
const historyOpen = ref(route.path === '/chat-history')
const search = ref('')
const title = ref('')
const renameOpen = ref(false)
const removeID = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const notice = ref('')
const visible = computed(() =>
  sessions.value.filter((session) =>
    `${session.title} ${session.model}`
      .toLocaleLowerCase()
      .includes(search.value.toLocaleLowerCase()),
  ),
)
const keyOptions = computed(() =>
  keys.value.map((key) => ({
    value: key.id,
    label: `${key.name} · ${key.group?.name ?? t('chat.defaultGroup')}`,
  })),
)
const modelOptions = computed(() => models.value.map((value) => ({ value, label: value })))
const messages = computed(
  () => active.value?.messages.filter((message) => message.role !== 'system') ?? [],
)

// chooseConversation opens an existing local record and preserves its route identity.
function chooseConversation(id: string) {
  chat.selectChat(id)
  historyOpen.value = false
  void router.replace(`/chat/${encodeURIComponent(id)}`)
}

// newConversation starts an empty draft and clears the previous route identity.
function newConversation() {
  chat.newChat()
  void router.replace('/chat')
}

// toggleHistory exposes local records without shrinking the narrow-screen conversation.
function toggleHistory() {
  historyOpen.value = !historyOpen.value
}

// openRename copies the current title into an explicitly committed dialog draft.
function openRename() {
  title.value = active.value?.title ?? ''
  renameOpen.value = true
}

// closeRename dismisses the title draft without changing the saved conversation.
function closeRename() {
  renameOpen.value = false
}

// saveTitle applies the current title draft to the active local conversation.
function saveTitle() {
  chat.rename(title.value)
  closeRename()
}

// confirmRemove removes only the conversation shown in the confirmation dialog.
function confirmRemove() {
  chat.remove(removeID.value)
  removeID.value = ''
}

// cancelRemove preserves local history after dismissing the confirmation dialog.
function cancelRemove() {
  removeID.value = ''
}

// exportHistory downloads the current account's text-only conversations as JSON.
// The object URL stays valid through the browser's download event, then releases
// its retained bytes. Storage failure does not prevent exporting in-memory text.
function exportHistory() {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(sessions.value, null, 2)], { type: 'application/json' }),
  )
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'chat-history.json'
  anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// selectImport opens a file chooser for an explicit local history import.
function selectImport() {
  fileInput.value?.click()
}

// importFile validates the selected file before handing its text to the workspace.
// Oversized files are rejected before reading. Clearing the input permits retrying
// the same repaired file without navigating away from the conversation.
async function importFile(event: Event) {
  const userID = auth.user?.id
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  if (file.size > 4 * 1024 * 1024) {
    error.value = t('chat.importFailed')
    return
  }
  try {
    const raw = await file.text()
    if (userID === auth.user?.id) chat.importSessions(raw)
  } catch {
    error.value = t('chat.importFailed')
  }
}

// importLegacy merges the original site's browser history after an explicit action.
function importLegacy() {
  chat.importSessions()
}

// copyMessage copies only the chosen response text and reports clipboard failures.
async function copyMessage(content: string) {
  try {
    await navigator.clipboard.writeText(content)
    notice.value = t('chat.copied')
  } catch {
    notice.value = t('chat.copyFailed')
  }
}

// Existing route identifiers select only records already visible in this account.
watch(
  () => route.params.id,
  (id) => {
    if (typeof id === 'string') chat.selectChat(id)
  },
  { immediate: true },
)
watch(activeID, (id) => {
  if (id && route.params.id !== id) void router.replace(`/chat/${encodeURIComponent(id)}`)
})
</script>

<template>
  <AppLayout>
    <div class="chat-workspace">
      <aside
        class="card flex min-w-0 flex-col gap-3 p-4"
        :class="{ 'chat-history-hidden': !historyOpen }"
      >
        <div class="flex items-center justify-between gap-2"
          ><h2 class="font-semibold">{{ t('chat.history') }}</h2
          ><button class="btn btn-primary" :disabled="sending" @click="newConversation"
            ><Icon name="plus" size="sm" />{{ t('chat.new') }}</button
          ></div
        >
        <label
          ><span class="sr-only">{{ t('chat.search') }}</span
          ><input v-model="search" class="input w-full" :placeholder="t('chat.search')"
        /></label>
        <p class="text-xs leading-5 text-gray-500">{{ t('chat.localOnly') }}</p>
        <nav :aria-label="t('chat.history')" class="max-h-96 space-y-1 overflow-y-auto">
          <p v-if="!visible.length" class="py-6 text-center text-sm text-gray-500">{{
            t('chat.emptyHistory')
          }}</p>
          <button
            v-for="session in visible"
            :key="session.id"
            class="w-full rounded-control px-3 py-3 text-left hover:bg-gray-100 dark:hover:bg-dark-700"
            :class="{ 'bg-gray-100 dark:bg-dark-700': session.id === activeID }"
            :disabled="sending"
            :aria-current="session.id === activeID ? 'page' : undefined"
            @click="chooseConversation(session.id)"
            ><span class="block break-words text-sm font-medium">{{ session.title }}</span
            ><span class="mt-1 block truncate text-xs text-gray-500">{{
              session.model
            }}</span></button
          >
        </nav>
        <div class="flex flex-wrap gap-2 border-t border-gray-100 pt-3 dark:border-dark-700"
          ><button
            class="btn btn-secondary btn-sm"
            :disabled="!sessions.length"
            @click="exportHistory"
            >{{ t('chat.export') }}</button
          ><button class="btn btn-secondary btn-sm" :disabled="sending" @click="selectImport">{{
            t('chat.import')
          }}</button
          ><button class="btn btn-secondary btn-sm" :disabled="sending" @click="importLegacy">{{
            t('chat.importLegacy')
          }}</button></div
        >
        <input
          ref="fileInput"
          type="file"
          accept="application/json,.json"
          class="hidden"
          @change="importFile"
        />
      </aside>
      <section class="min-w-0 space-y-4">
        <div class="card space-y-4 p-4 sm:p-5">
          <div class="flex flex-wrap items-center gap-2"
            ><button
              class="btn btn-secondary chat-history-toggle"
              :aria-expanded="historyOpen"
              @click="toggleHistory"
              ><Icon name="history" size="sm" />{{ t('chat.history') }}</button
            ><h2 class="min-w-0 flex-1 break-words font-semibold">{{
              active?.title ?? t('chat.new')
            }}</h2
            ><button
              v-if="active"
              class="btn btn-secondary btn-sm"
              :disabled="sending"
              @click="openRename"
              >{{ t('chat.rename') }}</button
            ><button
              v-if="active"
              class="btn btn-secondary btn-sm"
              :disabled="sending"
              @click="removeID = active.id"
              >{{ t('chat.delete') }}</button
            ><RouterLink class="btn btn-secondary btn-sm" to="/chat2link">{{
              t('chat.clients')
            }}</RouterLink></div
          >
          <div class="grid grid-cols-1 gap-3 xl:grid-cols-2"
            ><label class="min-w-0 space-y-2 text-sm"
              ><span>{{ t('chat.key') }}</span
              ><Select
                v-model="keyID"
                :options="keyOptions"
                :disabled="sending || loading"
                :placeholder="t('chat.selectKey')" /></label
            ><label class="min-w-0 space-y-2 text-sm"
              ><span>{{ t('chat.model') }}</span
              ><Select
                v-model="model"
                :options="modelOptions"
                searchable
                :disabled="sending || loading"
                :placeholder="t('chat.selectModel')" /></label
          ></div>
          <p v-if="loading" role="status" class="text-sm text-gray-500">{{
            t('common.loading')
          }}</p>
          <p v-else-if="!keys.length" class="text-sm text-gray-500"
            >{{ t('chat.noKeys') }}
            <RouterLink class="text-link" to="/keys">{{ t('chat.manageKeys') }}</RouterLink
            ><button class="btn btn-secondary btn-sm ml-2" @click="chat.loadKeys">{{
              t('chat.retry')
            }}</button></p
          >
          <p v-else-if="!models.length" class="text-sm text-gray-500"
            >{{ t('chat.noModels') }}
            <button class="btn btn-secondary btn-sm" @click="chat.discoverModels">{{
              t('chat.retry')
            }}</button></p
          >
          <details
            ><summary class="cursor-pointer text-sm text-gray-500">{{ t('chat.system') }}</summary
            ><label class="mt-3 block"
              ><span class="sr-only">{{ t('chat.system') }}</span
              ><textarea
                v-model="system"
                class="input min-h-24 w-full"
                :disabled="sending"
                maxlength="10000"
              /></label
          ></details>
        </div>
        <p
          v-if="storageError"
          role="alert"
          class="card p-4 text-sm text-red-600 dark:text-red-400"
          >{{ storageError }}</p
        >
        <p v-if="error" role="alert" class="card p-4 text-sm text-red-600 dark:text-red-400">{{
          error
        }}</p>
        <div class="card min-h-64 p-4 sm:p-6">
          <div v-if="!messages.length" class="mx-auto max-w-lg py-10 text-center"
            ><Icon name="chat" size="xl" class="mx-auto text-primary-600" /><h2
              class="mt-4 text-xl font-semibold"
              >{{ t('chat.start') }}</h2
            ><p class="mt-3 text-sm leading-6 text-gray-500">{{ t('chat.startHint') }}</p></div
          >
          <ol v-else class="space-y-8"
            ><li v-for="(message, index) in messages" :key="index" class="min-w-0"
              ><div class="mb-2 flex items-center gap-2 text-xs font-medium text-gray-500"
                ><Icon :name="message.role === 'user' ? 'user' : 'chat'" size="sm" /><span>{{
                  t(`chat.${message.role}`)
                }}</span
                ><button
                  v-if="message.content"
                  class="btn btn-secondary btn-sm ml-auto"
                  @click="copyMessage(message.content)"
                  >{{ t('chat.copy') }}</button
                ></div
              ><details
                v-if="message.reasoning"
                class="mb-3 rounded-surface bg-gray-50 p-3 dark:bg-dark-800"
                ><summary class="cursor-pointer text-sm text-gray-500">{{
                  t('chat.reasoning')
                }}</summary
                ><p class="mt-3 whitespace-pre-wrap break-words text-sm leading-6">{{
                  message.reasoning
                }}</p></details
              ><p class="whitespace-pre-wrap break-words text-sm leading-7">{{
                message.content ||
                (sending && index === messages.length - 1 ? t('chat.waiting') : t('chat.noOutput'))
              }}</p></li
            ></ol
          >
        </div>
        <form class="card space-y-3 p-4 sm:p-5" @submit.prevent="chat.send"
          ><label class="block space-y-2"
            ><span class="text-sm font-medium">{{ t('chat.prompt') }}</span
            ><textarea
              v-model="prompt"
              class="input min-h-28 w-full text-base"
              :disabled="sending"
              :placeholder="t('chat.promptHint')"
              maxlength="100000"
            /></label
          ><div class="flex flex-wrap items-center justify-between gap-3"
            ><p class="max-w-lg text-xs leading-5 text-gray-500">{{ t('chat.billingNote') }}</p
            ><button v-if="sending" type="button" class="btn btn-secondary" @click="chat.stop"
              ><Icon name="square" size="sm" />{{ t('chat.stop') }}</button
            ><button
              v-else
              class="btn btn-primary"
              :disabled="loading || !model || !keyID || !prompt.trim()"
              ><Icon name="arrowUp" size="sm" />{{ t('chat.send') }}</button
            ></div
          ><p v-if="elapsed != null" class="text-xs text-gray-500">{{
            t('chat.metrics', { elapsed, first: firstToken ?? '—', tokens: tokens ?? '—' })
          }}</p
          ><p v-if="notice" role="status" class="text-xs text-gray-500">{{ notice }}</p></form
        >
      </section>
    </div>
    <BaseDialog :show="renameOpen" :title="t('chat.rename')" trap-focus @close="closeRename"
      ><form class="space-y-4" @submit.prevent="saveTitle"
        ><label class="block space-y-2"
          ><span>{{ t('chat.titleLabel') }}</span
          ><input v-model="title" class="input w-full" maxlength="160" required /></label
        ><button class="btn btn-primary">{{ t('chat.save') }}</button></form
      ></BaseDialog
    >
    <BaseDialog :show="!!removeID" :title="t('chat.delete')" trap-focus @close="cancelRemove"
      ><p class="text-sm leading-6">{{ t('chat.deleteHint') }}</p
      ><div class="mt-5 flex justify-end gap-2"
        ><button class="btn btn-secondary" @click="cancelRemove">{{ t('chat.cancel') }}</button
        ><button class="btn btn-primary" @click="confirmRemove">{{ t('chat.delete') }}</button></div
      ></BaseDialog
    >
  </AppLayout>
</template>

<style scoped>
/* The history rail receives its own track only when conversation controls fit. */
.chat-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1rem;
}
.chat-history-hidden {
  display: none;
}
@media (min-width: 1280px) {
  .chat-workspace {
    grid-template-columns: 16rem minmax(0, 1fr);
  }
  .chat-history-hidden {
    display: flex;
  }
  .chat-history-toggle {
    display: none;
  }
}
</style>
