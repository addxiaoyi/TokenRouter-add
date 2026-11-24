import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import SupportView from '../SupportView.vue'

// SupportView.spec.ts checks private conversation lifecycle and retry behavior.
// Mock APIs expose submission identities and delayed reads while mounted forms
// preserve validation and user actions. No real ticket, notification, or account
// is created by these component fixtures.

// API and routing doubles keep requests observable and isolate application navigation.
const api = vi.hoisted(() => ({
  list: vi.fn(),
  thread: vi.fn(),
  create: vi.fn(),
  reply: vi.fn(),
  status: vi.fn(),
  settings: vi.fn(),
  saveSettings: vi.fn(),
  retryDelivery: vi.fn(),
}))
const push = vi.hoisted(() => vi.fn())
const route = reactive({
  fullPath: '/support-tickets',
  params: {} as Record<string, string>,
  query: {} as Record<string, string>,
  meta: { requiresAdmin: false },
})
const auth = reactive({ user: { id: 7 } })
vi.mock('@/api/support', () => ({ supportAPI: () => api }))
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ push }) }))
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

// mountSupport retains form behavior while replacing unrelated shell and modal layout.
function mountSupport() {
  return mount(SupportView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { props: ['show'], template: '<section v-if="show"><slot /></section>' },
        Icon: true,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  route.params = {}
  route.query = {}
  route.meta = { requiresAdmin: false }
  route.fullPath = '/support-tickets'
  auth.user = { id: 7 }
  api.list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
})

describe('support conversations', () => {
  // A lost submission response leaves the draft available for a retry.
  // Both attempts use one client identity and navigate only after persistence succeeds.
  it('preserves a failed creation draft and retries with the same identity', async () => {
    api.create
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ ticket: { id: 9 }, messages: [], has_more: false, next_id: 0 })
    const view = mountSupport()
    await flushPromises()
    await view
      .findAll('button')
      .find((button) => button.text() === 'support.newTicket')!
      .trigger('click')
    await view.get('#support-subject').setValue('Need help')
    await view.get('#support-description').setValue('A request failed')
    const form = view.get('#support-subject').element.closest('form')!
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(view.get('#support-description').element).toHaveProperty('value', 'A request failed')
    expect(view.text()).toContain('support.saveFailed')
    const identity = api.create.mock.calls[0][2]
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(api.create.mock.calls[1][2]).toBe(identity)
    expect(push).toHaveBeenCalledWith('/support-tickets/9')
    view.unmount()
  })

  // Account replacement occurs while a private thread is still loading.
  // The old response cannot display another account's subject after the replacement.
  it('discards a late private thread response after account replacement', async () => {
    let resolveOld!: (value: unknown) => void
    api.thread
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveOld = resolve
          }),
      )
      .mockResolvedValueOnce({
        ticket: {
          id: 4,
          subject: 'Current account',
          status: 'closed',
          created_at: '2026-11-21T00:00:00Z',
          updated_at: '2026-11-21T00:00:00Z',
          message_count: 0,
        },
        messages: [],
        has_more: false,
        next_id: 0,
      })
    route.params = { id: '4' }
    route.fullPath = '/support-tickets/4'
    const view = mountSupport()
    await flushPromises()
    auth.user = { id: 8 }
    await flushPromises()
    resolveOld({ ticket: { id: 4, subject: 'Private old account' }, messages: [] })
    await flushPromises()
    expect(view.text()).toContain('Current account')
    expect(view.text()).not.toContain('Private old account')
    view.unmount()
  })
})
