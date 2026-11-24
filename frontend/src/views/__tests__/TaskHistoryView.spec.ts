import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import type { TaskHistoryPage } from '@/api/taskHistory'
import TaskHistoryView from '../TaskHistoryView.vue'

// TaskHistoryView.spec.ts exercises history reads through the rendered filters,
// page controls, and account changes. The fixtures leave the view's request
// cancellation and loading states active while isolating the console shell.

const read = vi.hoisted(() => vi.fn())
const auth = reactive({ user: { id: 7 } })
const route = reactive({ meta: { requiresAdmin: false } })
vi.mock('@/api/taskHistory', () => ({ readTaskHistory: read }))
vi.mock('@/stores', () => ({ useAuthStore: () => auth }))
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => route,
}))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

// page supplies one batch row and the optional cursor for another page.
function page(id: string, cursor?: string): TaskHistoryPage {
  return {
    items: [
      {
        id,
        model: 'requested-model',
        status: 'completed',
        item_count: 2,
        success_count: 2,
        failure_count: 0,
        cancelled_count: 0,
        created_at: '2026-11-21T08:00:00Z',
      },
    ],
    next_cursor: cursor,
  }
}

// mountHistory preserves filters and row rendering while replacing sidebars.
function mountHistory() {
  return mount(TaskHistoryView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        EmptyState: true,
        Icon: true,
        Select: true,
      },
    },
  })
}

beforeEach(() => {
  read.mockReset()
  auth.user.id = 7
  route.meta.requiresAdmin = false
})

describe('task history', () => {
  // The administrator filters by submitter and advances with the returned
  // cursor. Next and previous must preserve the applied filters during navigation.
  it('retains administrator filters on catalogue paging', async () => {
    route.meta.requiresAdmin = true
    read
      .mockResolvedValueOnce(page('initial'))
      .mockResolvedValueOnce(page('first-result', '41'))
      .mockResolvedValueOnce(page('second-result'))
      .mockResolvedValueOnce(page('first-result', '41'))
    const view = mountHistory()
    await flushPromises()
    await view.get('input[inputmode="numeric"]').setValue('42')
    await view.get('form').trigger('submit')
    await flushPromises()
    expect(read.mock.calls[1]?.slice(0, 2)).toEqual([
      'admin',
      {
        batch_id: undefined,
        status: undefined,
        user_id: '42',
        cursor: undefined,
      },
    ])
    await view.get('nav').get('button:last-child').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('second-result')
    expect(read.mock.calls[2]?.[1]).toMatchObject({ user_id: '42', cursor: '41' })
    await view.get('nav').get('button:first-child').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('first-result')
    expect(read.mock.calls[3]?.[1]).toMatchObject({ user_id: '42', cursor: undefined })
    view.unmount()
  })

  // A previously authorized response arrives after the authenticated account
  // changes. The pending request must be cancelled and its tasks stay hidden.
  it('drops a task page from a previous account', async () => {
    let finishOld: (value: TaskHistoryPage) => void = () => undefined
    read
      .mockImplementationOnce(
        () =>
          new Promise<TaskHistoryPage>((resolve) => {
            finishOld = resolve
          }),
      )
      .mockResolvedValueOnce(page('current-user-batch'))
    const view = mountHistory()
    const oldSignal: AbortSignal = read.mock.calls[0]?.[2]
    auth.user.id = 8
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    finishOld(page('previous-user-batch'))
    await flushPromises()
    expect(view.text()).toContain('current-user-batch')
    expect(view.text()).not.toContain('previous-user-batch')
    view.unmount()
  })
})
