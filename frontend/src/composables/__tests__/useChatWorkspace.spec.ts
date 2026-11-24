import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useChatWorkspace } from '../useChatWorkspace'
import { readHistory } from '@/utils/chatHistory'
import type { ChatDelta } from '@/api/chat'

// useChatWorkspace.spec.ts verifies streamed conversation ownership across account
// replacement and component cleanup. API fixtures retain real browser storage
// and Vue reactivity while keeping all gateway work local and unbilled.

// Shared mocks expose delayed fragments and key discovery for lifecycle assertions.
const api = vi.hoisted(() => ({ keys: vi.fn(), models: vi.fn(), stream: vi.fn() }))
const auth = reactive({ user: { id: 7 } })
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/keys', () => ({ keysAPI: { list: api.keys } }))
vi.mock('@/api/chat', () => ({ readChatModels: api.models, streamChat: api.stream }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

// workspace mounts lifecycle hooks and exposes the composable to the fixture.
function workspace() {
  let chat!: ReturnType<typeof useChatWorkspace>
  const view = mount(
    defineComponent({
      setup() {
        chat = useChatWorkspace()
        return () => null
      },
    }),
  )
  return {
    view,
    get chat() {
      return chat
    },
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  auth.user = { id: 7 }
  api.keys.mockResolvedValue({
    items: [{ id: 1, name: 'Fixture', key: 'fixture-key', status: 'active' }],
    total: 1,
  })
  api.models.mockResolvedValue(['gpt-4o-mini'])
})

describe('chat workspace lifecycle', () => {
  // A successful gateway stream emits multiple fragments into one reactive reply.
  // Persisted conversation text survives reload while API credentials remain excluded.
  it('persists the complete response without a credential', async () => {
    api.stream.mockImplementation(
      async (_key, _model, _messages, _signal, emit: (delta: ChatDelta) => void) => {
        emit({ content: 'Hello ', reasoning: '' })
        emit({ content: 'world', reasoning: '', tokens: 12 })
      },
    )
    const mounted = workspace()
    await flushPromises()
    mounted.chat.prompt.value = 'Question'
    await mounted.chat.send()
    expect(mounted.chat.active.value?.messages.at(-1)?.content).toBe('Hello world')
    expect(readHistory(7)[0].messages.at(-1)?.content).toBe('Hello world')
    expect(JSON.stringify(readHistory(7))).not.toContain('fixture-key')
    mounted.view.unmount()
  })

  // Account replacement occurs before a delayed gateway finishes its response.
  // Old content and completion cannot populate or persist in the newly selected account.
  it('rejects late fragments after an account change', async () => {
    let emit!: (delta: ChatDelta) => void
    let finish!: () => void
    let signal!: AbortSignal
    api.stream.mockImplementation((_key, _model, _messages, nextSignal, nextEmit) => {
      emit = nextEmit
      signal = nextSignal
      return new Promise<void>((resolve) => {
        finish = resolve
      })
    })
    const mounted = workspace()
    await flushPromises()
    mounted.chat.prompt.value = 'Private question'
    const pending = mounted.chat.send()
    auth.user = { id: 8 }
    await flushPromises()
    expect(signal.aborted).toBe(true)
    emit({ content: 'Old private answer', reasoning: '' })
    finish()
    await pending
    expect(mounted.chat.sessions.value).toEqual([])
    expect(readHistory(8)).toEqual([])
    mounted.view.unmount()
  })

  // Unmount interrupts a stream after some content has already been shown.
  // Its partial answer is saved before the generation becomes invalid for late output.
  it('saves partial output before unmount cancellation', async () => {
    let finish!: () => void
    api.stream.mockImplementation((_key, _model, _messages, signal, emit) => {
      emit({ content: 'Partial answer', reasoning: '' })
      return new Promise<void>((resolve) => {
        finish = resolve
        signal.addEventListener('abort', resolve, { once: true })
      })
    })
    const mounted = workspace()
    await flushPromises()
    mounted.chat.prompt.value = 'Question'
    const pending = mounted.chat.send()
    mounted.view.unmount()
    finish()
    await pending
    expect(readHistory(7)[0].messages.at(-1)?.content).toBe('Partial answer')
  })
})
