import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores'
import { keysAPI } from '@/api/keys'
import type { ApiKey } from '@/api/keys.types'
import { readChatModels, streamChat } from '@/api/chat'
import {
  decodeHistory,
  readHistory,
  readLegacyHistory,
  writeHistory,
  type ChatSession,
} from '@/utils/chatHistory'

// useChatWorkspace.ts owns a user's local conversations and gateway request
// lifetime. Account replacement clears credentials and aborts in-flight work
// before another account's history loads. Model discovery belongs to the selected
// key, while generation checks reject responses from an earlier selection.
//
// A conversation persists its user message before transmission and its partial
// response after completion, cancellation, or failure. Failed persistence leaves
// the in-memory conversation available for export. Requests are never retried
// automatically because partial upstream work may already have incurred a charge.
// Key values are held only in this mounted composable and excluded from history.

// useChatWorkspace creates account-scoped local history and cancellable chat operations.
// Invoke during component setup. The component receives reactive state and user
// actions; unmount cancels owned requests. Callers must let this composable own
// history writes so late responses cannot publish another account's content.
// Browser storage failures are shown alongside the retained in-memory draft,
// and the export action remains available even when local persistence fails.
// @project-doc docs/domains/chat_workspace.md#conversation_lifecycle
export function useChatWorkspace() {
  const auth = useAuthStore()
  const { t } = useI18n()
  // Page state retains only the current account's credentials and conversations.
  const sessions = ref<ChatSession[]>([])
  const activeID = ref('')
  const keys = ref<ApiKey[]>([])
  const keyID = ref<number | null>(null)
  const models = ref<string[]>([])
  const model = ref('')
  const prompt = ref('')
  const system = ref('')
  const loading = ref(false)
  const sending = ref(false)
  const error = ref('')
  const storageError = ref('')
  const tokens = ref<number | null>(null)
  const elapsed = ref<number | null>(null)
  const firstToken = ref<number | null>(null)
  let scope = 0
  let keyScope = 0
  let request: AbortController | undefined
  let discovery: AbortController | undefined
  const active = computed(() => sessions.value.find((session) => session.id === activeID.value))
  const selectedKey = computed(() => keys.value.find((key) => key.id === keyID.value))

  // persist retains the current account's records and exposes recoverable storage failures.
  function persist() {
    if (!auth.user?.id) return
    try {
      writeHistory(auth.user.id, sessions.value)
      storageError.value = ''
    } catch {
      storageError.value = t('chat.storageFailed')
    }
  }

  // loadKeys reads all enabled personal credentials with a bounded page count.
  // Results from a previous account are discarded. Discovery starts only after
  // the current credential list has been installed and a selection exists.
  async function loadKeys() {
    const current = scope
    loading.value = true
    error.value = ''
    try {
      const available: ApiKey[] = []
      for (let page = 1; page <= 100; page++) {
        const loaded = await keysAPI.list(page, 100, { scope: 'personal', status: 'active' })
        if (current !== scope) return
        available.push(...loaded.items.filter((key) => key.status === 'active'))
        if (page * 100 >= loaded.total) break
      }
      keys.value = available
      keyID.value = available[0]?.id ?? null
    } catch {
      if (current === scope) error.value = t('chat.keysFailed')
    } finally {
      if (current === scope) loading.value = false
    }
  }

  // discoverModels replaces choices for the selected key and rejects stale completion.
  // Changing keys aborts the previous request before querying the gateway. An
  // unavailable or empty catalog leaves sending disabled with a recovery action.
  async function discoverModels() {
    const current = ++keyScope
    discovery?.abort()
    discovery = new AbortController()
    models.value = []
    if (!selectedKey.value) return
    loading.value = true
    error.value = ''
    try {
      const loaded = await readChatModels(selectedKey.value.key, discovery.signal)
      if (current !== keyScope) return
      models.value = loaded
      if (!loaded.includes(model.value)) model.value = loaded[0] ?? ''
    } catch {
      if (current === keyScope) error.value = t('chat.modelsFailed')
    } finally {
      if (current === keyScope) loading.value = false
    }
  }

  // newChat selects an empty draft without creating an unused history record.
  function newChat() {
    if (!sending.value) {
      activeID.value = ''
      prompt.value = ''
      system.value = ''
      error.value = ''
    }
  }

  // selectChat opens a stored conversation and its system prompt when idle.
  function selectChat(id: string) {
    if (sending.value) return
    if (!sessions.value.some((session) => session.id === id)) return
    activeID.value = id
    const conversation = active.value
    if (conversation && models.value.includes(conversation.model)) model.value = conversation.model
    system.value =
      conversation?.messages.find((message) => message.role === 'system')?.content ?? ''
    error.value = ''
  }

  // send persists a prompt and streams its billed response into the same conversation.
  // A captured account generation owns every emitted fragment and final write.
  // Explicit stop retains partial output; network failure marks the response as
  // incomplete and leaves history available. The user must initiate any retry,
  // which is a separate request and can incur its own gateway charge.
  // Controls prevent changing conversations while their response remains active.
  async function send() {
    if (sending.value || !prompt.value.trim() || !model.value || !selectedKey.value || !auth.user)
      return
    const current = scope
    const text = prompt.value.trim()
    if (text.length > 100000 || (active.value?.messages.length ?? 0) >= 498) {
      error.value = t('chat.limit')
      return
    }
    let conversation = active.value
    if (!conversation) {
      conversation = {
        id: crypto.randomUUID(),
        title: text.slice(0, 60),
        model: model.value,
        createdAt: Date.now(),
        updatedAt: Date.now(),
        messages: [],
      }
      sessions.value.unshift(conversation)
      activeID.value = conversation.id
      conversation = active.value!
    }
    conversation.messages = conversation.messages.filter((message) => message.role !== 'system')
    if (system.value.trim())
      conversation.messages.unshift({ role: 'system', content: system.value.trim() })
    conversation.model = model.value
    conversation.messages.push({ role: 'user', content: text })
    const wireMessages = conversation.messages.map((message) => ({ ...message }))
    conversation.messages.push({ role: 'assistant', content: '' })
    const answer = conversation.messages[conversation.messages.length - 1]
    prompt.value = ''
    sending.value = true
    error.value = ''
    tokens.value = null
    elapsed.value = null
    firstToken.value = null
    request = new AbortController()
    const signal = request.signal
    const started = performance.now()
    persist()
    try {
      await streamChat(selectedKey.value.key, model.value, wireMessages, signal, (delta) => {
        if (current !== scope) return
        if ((delta.content || delta.reasoning) && firstToken.value == null)
          firstToken.value = Math.round(performance.now() - started)
        answer.content += delta.content
        answer.reasoning = (answer.reasoning ?? '') + delta.reasoning
        if (delta.tokens != null) tokens.value = delta.tokens
      })
    } catch {
      if (current === scope) error.value = t(signal.aborted ? 'chat.stopped' : 'chat.sendFailed')
    } finally {
      if (current === scope) {
        conversation.updatedAt = Date.now()
        elapsed.value = Math.round(performance.now() - started)
        sending.value = false
        request = undefined
        persist()
      }
    }
  }

  // stop cancels only the active generation while preserving already received text.
  function stop() {
    request?.abort()
  }

  // rename saves a nonempty display title without altering conversation contents.
  function rename(title: string) {
    if (active.value && title.trim()) {
      active.value.title = title.trim().slice(0, 160)
      persist()
    }
  }

  // remove deletes a locally confirmed conversation while the workspace is idle.
  function remove(id: string) {
    if (!sending.value) {
      sessions.value = sessions.value.filter((session) => session.id !== id)
      if (activeID.value === id) activeID.value = ''
      persist()
    }
  }

  // importSessions merges explicitly selected records while preserving current duplicate IDs.
  // An invalid file leaves all existing records intact. Imported records exclude
  // credentials through decodeHistory and remain scoped to the current account.
  function importSessions(raw?: string) {
    try {
      const imported = raw == null ? readLegacyHistory() : decodeHistory(raw)
      const known = new Set(sessions.value.map((session) => session.id))
      sessions.value = [...sessions.value, ...imported.filter((session) => !known.has(session.id))]
        .sort((a, b) => b.updatedAt - a.updatedAt)
        .slice(0, 50)
      persist()
      if (!imported.length) error.value = t('chat.noLegacy')
    } catch {
      error.value = t('chat.importFailed')
    }
  }

  // Account replacement invalidates asynchronous work before loading private browser
  // records, so late fragments cannot enter a newly authenticated account's history.
  watch(
    () => auth.user?.id,
    (id) => {
      scope++
      keyScope++
      request?.abort()
      discovery?.abort()
      sessions.value = []
      keys.value = []
      keyID.value = null
      models.value = []
      activeID.value = ''
      sending.value = false
      prompt.value = ''
      system.value = ''
      error.value = ''
      storageError.value = ''
      if (!id) return
      try {
        sessions.value = readHistory(id)
      } catch {
        storageError.value = t('chat.storageFailed')
      }
      void loadKeys()
    },
    { immediate: true },
  )
  watch(keyID, () => {
    void discoverModels()
  })
  onBeforeUnmount(() => {
    // Save the visible fragment before invalidating its generation; the aborted
    // request's finalizer must not write into a replacement page or account.
    if (sending.value) persist()
    scope++
    keyScope++
    request?.abort()
    discovery?.abort()
  })
  return {
    sessions,
    activeID,
    active,
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
    loadKeys,
    discoverModels,
    newChat,
    selectChat,
    send,
    stop,
    rename,
    remove,
    importSessions,
  }
}
