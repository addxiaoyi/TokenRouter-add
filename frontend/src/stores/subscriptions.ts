import { defineStore } from 'pinia'
import { ref, computed, onScopeDispose } from 'vue'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserSubscription } from '@/api/subscriptions.types'

// subscriptions.ts caches active subscriptions and manages periodic refreshes for
// authenticated views. Ordinary reads reuse fresh data or join an active request; forced
// reads start another request and advance a module-level generation shared by store
// instances.
//
// Each response retains its own result for its caller, but only the latest generation
// publishes subscription state. Clearing the store advances the generation so older
// responses cannot restore cleared data. Promise identity controls loading cleanup
// independently from publication, allowing an older completion to leave a newer request
// active. Polling owns one interval and its visibility/connectivity listeners. Hidden
// or offline documents release the timer; resuming uses an ordinary cache-aware read
// so closely spaced browser events share fresh data or one in-flight request.

// CACHE_TTL_MS sets the sixty-second freshness window used by ordinary reads before
// requesting another subscription list.
const CACHE_TTL_MS = 60_000
// POLL_INTERVAL_MS spaces automatic refreshes while the authenticated document is
// visible and online; browser readiness events can refresh an expired cache sooner.
const POLL_INTERVAL_MS = 5 * 60 * 1000
// requestGeneration is shared across store instances and invalidates publication when a
// newer request starts or a store clears its data.
let requestGeneration = 0

// useSubscriptionStore exposes cached active subscriptions and their polling controls to
// Pinia consumers. Authentication owners start polling and call clear when the session
// ends to invalidate pending publication.
export const useSubscriptionStore = defineStore('subscriptions', () => {
  // Subscription refs retain displayed records, request activity, and the timestamp used
  // to decide whether ordinary reads can reuse cached data.
  const activeSubscriptions = ref<UserSubscription[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const lastFetchedAt = ref<number | null>(null)

  // activePromise lets ordinary callers share a pending request; forced calls replace
  // this owner so older completions cannot clear newer loading state.
  let activePromise: Promise<UserSubscription[]> | null = null

  // pollingEnabled owns browser listeners even while hidden/offline state pauses its
  // timer. pollerInterval is null whenever no automatic refresh interval is scheduled.
  let pollingEnabled = false
  let pollerInterval: ReturnType<typeof setInterval> | null = null

  // hasActiveSubscriptions derives the presence of usable subscriptions from the current
  // displayed records for account consumers.
  const hasActiveSubscriptions = computed(() => activeSubscriptions.value.length > 0)

  // fetchActiveSubscriptions returns cached data within sixty seconds or obtains a
  // server response. Ordinary reads join an active promise, while force starts another
  // request and increments the shared generation. Only the latest generation publishes
  // data, although each caller still receives its own response. Promise identity decides
  // which completion clears loading. Request errors propagate after logging, and
  // clearing the store invalidates publication without cancelling transport.
  async function fetchActiveSubscriptions(force = false): Promise<UserSubscription[]> {
    const now = Date.now()
    // Ordinary reads reuse the displayed subscriptions during their freshness window,
    // avoiding another request until expiry or explicit invalidation.
    if (
      !force &&
      loaded.value &&
      lastFetchedAt.value !== null &&
      now - lastFetchedAt.value < CACHE_TTL_MS
    ) {
      return activeSubscriptions.value
    }
    // Ordinary concurrent reads join the active request; force deliberately bypasses
    // this owner and starts a new publication generation.
    if (activePromise && !force) {
      return activePromise
    }

    const currentGeneration = ++requestGeneration
    loading.value = true
    const requestPromise = subscriptionsAPI
      .getActiveSubscriptions()
      .then((data) => {
        // A newer request or clear operation invalidates this response for shared state,
        // while the caller still receives its own resolved data.
        if (currentGeneration === requestGeneration) {
          activeSubscriptions.value = data
          loaded.value = true
          lastFetchedAt.value = Date.now()
        }
        return data
      })
      .catch((error) => {
        console.error('Failed to fetch active subscriptions:', error)
        throw error
      })
      .finally(() => {
        // An older request may settle after replacement, so only the promise that still
        // owns loading can release it.
        if (activePromise === requestPromise) {
          loading.value = false
          activePromise = null
        }
      })

    activePromise = requestPromise

    return activePromise
  }

  // canPoll reports whether browser readiness permits automatic subscription refreshes.
  function canPoll(): boolean {
    return document.visibilityState !== 'hidden' && navigator.onLine !== false
  }

  // refreshPolling requests subscriptions only while its polling owner remains active.
  // Ordinary reads preserve the freshness window and share pending work, avoiding
  // duplicate requests when visibility and online events arrive close together.
  function refreshPolling(): void {
    if (!pollingEnabled || !canPoll()) return
    fetchActiveSubscriptions().catch((error) => {
      console.error('Subscription polling failed:', error)
    })
  }

  // schedulePolling replaces the automatic timer according to current browser readiness.
  // Hidden or offline documents retain listener ownership without a running interval;
  // later readiness events establish one fresh interval instead of accumulating timers.
  function schedulePolling(): void {
    if (pollerInterval !== null) clearInterval(pollerInterval)
    pollerInterval = null
    if (pollingEnabled && canPoll()) {
      pollerInterval = setInterval(refreshPolling, POLL_INTERVAL_MS)
    }
  }

  // onPollingReadinessChange reconciles the timer and refreshes only an eligible document.
  // An online event while hidden still leaves polling paused. When visibility returns,
  // the ordinary read refreshes only expired state or joins its already active request.
  function onPollingReadinessChange(): void {
    schedulePolling()
    refreshPolling()
  }

  // startPolling acquires one set of browser listeners and a five-minute refresh timer.
  // It does not fetch immediately: the authentication owner performs its initial read.
  // Repeated starts preserve the existing owner, including a timer paused while hidden.
  function startPolling(): void {
    if (pollingEnabled) return
    pollingEnabled = true
    document.addEventListener('visibilitychange', onPollingReadinessChange)
    window.addEventListener('online', onPollingReadinessChange)
    window.addEventListener('offline', onPollingReadinessChange)
    schedulePolling()
  }

  // stopPolling releases the automatic timer and its browser event listeners.
  // Repeated calls after cleanup do nothing. Cached records remain available, and
  // requests already in flight continue under their existing publication generation.
  function stopPolling(): void {
    pollingEnabled = false
    if (pollerInterval !== null) clearInterval(pollerInterval)
    pollerInterval = null
    document.removeEventListener('visibilitychange', onPollingReadinessChange)
    window.removeEventListener('online', onPollingReadinessChange)
    window.removeEventListener('offline', onPollingReadinessChange)
  }

  // clear invalidates pending publication, removes cached subscriptions, and stops the
  // polling interval. Advancing the module-level generation first prevents older
  // responses from restoring the cleared records. The active promise is detached, but
  // its transport continues and its caller can still receive the response. Loading
  // clears immediately because the detached promise no longer owns this store's UI;
  // a subsequent request acquires its own loading state and cleanup ownership.
  function clear() {
    // Advance the publication generation before clearing visible data so responses
    // already in flight cannot repopulate the cleared subscription list.
    requestGeneration++
    activePromise = null
    loading.value = false
    activeSubscriptions.value = []
    loaded.value = false
    lastFetchedAt.value = null
    stopPolling()
  }

  // invalidateCache removes the freshness timestamp so the next idle read requests data.
  // Existing subscription records remain visible, and an active request can still be
  // joined by an ordinary caller.
  function invalidateCache() {
    lastFetchedAt.value = null
  }

  // Pinia disposes this setup scope independently from logout, so release browser
  // listeners and the timer even when the authentication owner's clear hook is absent.
  onScopeDispose(stopPolling)

  return {
    activeSubscriptions,
    loading,
    hasActiveSubscriptions,
    fetchActiveSubscriptions,
    startPolling,
    stopPolling,
    clear,
    invalidateCache,
  }
})
