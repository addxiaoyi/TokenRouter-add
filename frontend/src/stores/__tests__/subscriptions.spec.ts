import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useSubscriptionStore } from '@/stores/subscriptions'

// subscriptions.spec.ts verifies subscription cache freshness, shared requests,
// invalidation, and polling lifecycle. Fake clocks cross freshness and polling
// intervals, while a deferred endpoint exposes concurrent reads. Assertions distinguish
// cached records, returned responses, and the shared presence selector.

// mockGetActiveSubscriptions controls server completion and rejection so cache admission
// and polling behavior can be asserted independently of network timing.
const mockGetActiveSubscriptions = vi.fn()

vi.mock('@/api/subscriptions', () => ({
  default: {
    getActiveSubscriptions: (...args: any[]) => mockGetActiveSubscriptions(...args),
  },
}))

// fakeSubscriptions supplies two active records in different groups so list replacement,
// presence selectors, and clearing have observable results.
const fakeSubscriptions = [
  {
    id: 1,
    user_id: 1,
    group_id: 1,
    status: 'active' as const,
    daily_usage_usd: 5,
    weekly_usage_usd: 20,
    monthly_usage_usd: 50,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    expires_at: '2025-01-01',
  },
  {
    id: 2,
    user_id: 1,
    group_id: 2,
    status: 'active' as const,
    daily_usage_usd: 10,
    weekly_usage_usd: 40,
    monthly_usage_usd: 100,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2024-02-01',
    updated_at: '2024-02-01',
    expires_at: '2025-02-01',
  },
]

describe('useSubscriptionStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.useFakeTimers()
    vi.clearAllMocks()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true)
  })

  afterEach(() => {
    useSubscriptionStore().$dispose()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  describe('fetchActiveSubscriptions', () => {
    // The subscription endpoint returns active records for the account. Fetch publishes
    // those records, returns the response data, and clears loading after the request
    // completes.
    it('成功获取活跃订阅', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      const result = await store.fetchActiveSubscriptions()

      expect(result).toEqual(fakeSubscriptions)
      expect(store.activeSubscriptions).toEqual(fakeSubscriptions)
      expect(store.loading).toBe(false)
    })

    // A successful subscription fetch is followed by another ordinary read within
    // freshness. The second caller receives cached records and no additional API request
    // is issued.
    it('缓存有效时返回缓存数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()
      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      const result = await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      expect(result).toEqual(fakeSubscriptions)
    })

    // The subscription cache ages past its sixty-second freshness window. The next
    // ordinary read requests replacement data and returns the updated subscription list
    // to its caller.
    it('缓存过期后重新请求', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      vi.advanceTimersByTime(61_000)

      const updatedSubs = [fakeSubscriptions[0]]
      mockGetActiveSubscriptions.mockResolvedValue(updatedSubs)

      const result = await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
      expect(result).toEqual(updatedSubs)
    })

    // A caller forces a subscription refresh while cached data is still fresh. The
    // endpoint is requested again and the caller receives the replacement records.
    it('force=true 强制重新请求', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()

      const updatedSubs = [fakeSubscriptions[0]]
      mockGetActiveSubscriptions.mockResolvedValue(updatedSubs)

      const result = await store.fetchActiveSubscriptions(true)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
      expect(result).toEqual(updatedSubs)
    })

    // Two ordinary subscription reads begin before their shared response resolves. A
    // single API request supplies both callers with the same returned subscription data.
    it('并发请求共享同一个 Promise（去重）', async () => {
      let resolvePromise: (v: any) => void
      mockGetActiveSubscriptions.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve
          }),
      )
      const store = useSubscriptionStore()
      const p1 = store.fetchActiveSubscriptions()
      const p2 = store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      resolvePromise!(fakeSubscriptions)

      const [r1, r2] = await Promise.all([p1, p2])
      expect(r1).toEqual(fakeSubscriptions)
      expect(r2).toEqual(fakeSubscriptions)
    })

    // The active-subscription endpoint rejects a request with a network error. Fetch
    // propagates the error to its caller rather than returning an empty successful list.
    it('API 错误时抛出异常', async () => {
      mockGetActiveSubscriptions.mockRejectedValue(new Error('Network error'))
      const store = useSubscriptionStore()

      await expect(store.fetchActiveSubscriptions()).rejects.toThrow('Network error')
    })
  })

  describe('hasActiveSubscriptions', () => {
    // A successful subscription fetch publishes nonempty active records. The presence
    // selector becomes true so account consumers can expose the available subscription-
    // related interface state.
    it('有订阅时返回 true', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()

      expect(store.hasActiveSubscriptions).toBe(true)
    })

    // A fresh subscription store has no displayed active records. The presence selector
    // returns false before any successful fetch can populate the shared subscription
    // list.
    it('无订阅时返回 false', () => {
      const store = useSubscriptionStore()
      expect(store.hasActiveSubscriptions).toBe(false)
    })

    // A populated subscription store is cleared after a successful read. The presence
    // selector changes from true to false as the displayed records are removed.
    it('清除后返回 false', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(store.hasActiveSubscriptions).toBe(true)

      store.clear()
      expect(store.hasActiveSubscriptions).toBe(false)
    })
  })

  describe('invalidateCache', () => {
    // A fresh subscription response is explicitly invalidated before another ordinary
    // read. The next call requests data again rather than reusing the previous freshness
    // timestamp.
    it('失效缓存后下次请求重新获取数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      store.invalidateCache()

      await store.fetchActiveSubscriptions()
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
    })
  })

  describe('clear', () => {
    // A store containing two active subscription records is cleared. Its displayed list
    // becomes empty and the presence selector immediately reflects that no records
    // remain.
    it('清除所有订阅数据', async () => {
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const store = useSubscriptionStore()

      await store.fetchActiveSubscriptions()
      expect(store.activeSubscriptions).toHaveLength(2)

      store.clear()

      expect(store.activeSubscriptions).toHaveLength(0)
      expect(store.hasActiveSubscriptions).toBe(false)
    })
  })

  describe('startPolling / stopPolling', () => {
    // Polling startup is requested twice for the same store. Advancing one polling
    // interval produces a single API request, protecting the single-owned-timer
    // behavior.
    it('startPolling 不会创建重复 interval', () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])

      store.startPolling()
      store.startPolling()
      vi.advanceTimersByTime(5 * 60 * 1000)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)

      store.stopPolling()
    })

    // Polling starts and is stopped before its first interval elapses. Advancing time
    // afterward produces no API request, proving the owned timer has been released.
    it('stopPolling 停止定期刷新', () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])

      store.startPolling()
      store.stopPolling()

      vi.advanceTimersByTime(10 * 60 * 1000)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
    })

    // Hidden documents retain polling ownership while their automatic timer is paused.
    // Becoming visible after a long absence performs one ordinary refresh and reestablishes one timer.
    it('pauses hidden polling and refreshes once when visible again', async () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
      store.startPolling()
      store.startPolling()
      await vi.advanceTimersByTimeAsync(15 * 60 * 1000)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
      visibility.mockReturnValue('visible')
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(0)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBe(1)
    })

    // Connectivity may recover while the document is still in the background.
    // Neither offline ticks nor that early online event fetch until the document also becomes visible.
    it('requires both visibility and connectivity before resuming requests', async () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])
      const online = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
      const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
      store.startPolling()
      await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
      online.mockReturnValue(true)
      window.dispatchEvent(new Event('online'))
      await vi.advanceTimersByTimeAsync(0)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
      visibility.mockReturnValue('visible')
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(0)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      online.mockReturnValue(false)
      window.dispatchEvent(new Event('offline'))
      expect(vi.getTimerCount()).toBe(0)
    })

    // The browser returns to a fresh cache, then emits visibility and connectivity events after expiry.
    // Fresh records avoid traffic, and simultaneous recovery notifications join one pending replacement request.
    it('uses cache freshness and pending-request sharing when browser events repeat', async () => {
      vi.setSystemTime(0)
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      await store.fetchActiveSubscriptions()
      store.startPolling()
      document.dispatchEvent(new Event('visibilitychange'))
      window.dispatchEvent(new Event('online'))
      await vi.advanceTimersByTimeAsync(0)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(61_000)
      document.dispatchEvent(new Event('visibilitychange'))
      window.dispatchEvent(new Event('online'))
      window.dispatchEvent(new Event('online'))
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(2)
      await vi.advanceTimersByTimeAsync(0)
      expect(vi.getTimerCount()).toBe(1)
    })

    // A user-initiated refresh completes shortly before the automatic five-minute timer.
    // Polling reuses its fresh response instead of forcing another subscription download during that freshness window.
    it('reuses fresh explicit refreshes instead of forcing each polling tick', async () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue(fakeSubscriptions)
      store.startPolling()
      await vi.advanceTimersByTimeAsync(5 * 60 * 1000 - 10_000)
      await store.fetchActiveSubscriptions(true)
      await vi.advanceTimersByTimeAsync(10_000)
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
    })

    // Polling stops while browser recovery events remain possible on the page.
    // Repeated stop calls remove automatic ownership, and later events or elapsed intervals cannot restart requests.
    it('removes recovery listeners when polling is stopped', async () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])
      store.startPolling()
      store.stopPolling()
      store.stopPolling()
      window.dispatchEvent(new Event('online'))
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
    })

    // Pinia can dispose a store without the application's logout watcher running first.
    // Scope cleanup releases polling resources so later browser events cannot request subscriptions for that disposed owner.
    it('releases polling listeners and timers when Pinia disposes the store', async () => {
      const store = useSubscriptionStore()
      mockGetActiveSubscriptions.mockResolvedValue([])
      store.startPolling()
      store.$dispose()
      window.dispatchEvent(new Event('online'))
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
      expect(mockGetActiveSubscriptions).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
    })
  })

  describe('request publication races', () => {
    // Two forced reads finish in reverse order after obtaining distinct server responses.
    // Each caller receives its own response, while only the latest request generation publishes shared subscription state.
    it('keeps the latest forced response when an older request finishes afterward', async () => {
      const store = useSubscriptionStore()
      let finishOlder!: (subscriptions: typeof fakeSubscriptions) => void
      let finishNewer!: (subscriptions: typeof fakeSubscriptions) => void
      mockGetActiveSubscriptions
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              finishOlder = resolve
            }),
        )
        .mockImplementationOnce(
          () =>
            new Promise((resolve) => {
              finishNewer = resolve
            }),
        )
      const older = store.fetchActiveSubscriptions(true)
      const newer = store.fetchActiveSubscriptions(true)
      finishNewer([fakeSubscriptions[0]])
      await expect(newer).resolves.toEqual([fakeSubscriptions[0]])
      finishOlder(fakeSubscriptions)
      await expect(older).resolves.toEqual(fakeSubscriptions)
      expect(store.activeSubscriptions).toEqual([fakeSubscriptions[0]])
      expect(store.loading).toBe(false)
    })

    // Logout clears the store while its subscription response has not yet arrived.
    // Loading and polling stop immediately, and neither late completion nor browser recovery can restore that account's records.
    it('keeps cleared state and releases loading when a detached request completes', async () => {
      const store = useSubscriptionStore()
      let finish!: (subscriptions: typeof fakeSubscriptions) => void
      mockGetActiveSubscriptions.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
      const request = store.fetchActiveSubscriptions()
      store.startPolling()
      expect(store.loading).toBe(true)
      store.clear()
      expect(store.loading).toBe(false)
      finish(fakeSubscriptions)
      await expect(request).resolves.toEqual(fakeSubscriptions)
      window.dispatchEvent(new Event('online'))
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
      expect(store.activeSubscriptions).toEqual([])
      expect(mockGetActiveSubscriptions).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBe(0)
    })
  })
})
