import { computed, ref, watch, onBeforeUnmount, type Ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { readMonitoring, type MonitoringSnapshot } from '@/api/monitoring'

// useGroupMonitoring.ts supplies authorized group observations to selection surfaces.
// Group selectors and marketplace details share the same private endpoint and
// observation window. Loading measurements does not change which groups may be
// selected, and unavailable observations remain separate from the group catalog.
//
// Each request captures its account and generation before reading. A changed
// identity, disabled consumer, or unmount retires that generation before its
// completion can publish private observations. Same-account errors retain the
// previous snapshot with explicit failure state, while account changes clear it
// synchronously so an old group's measurements cannot appear in another account.

// useGroupMonitoring loads a twenty-four-hour observation window for an enabled consumer.
// Call during component setup. The caller presents loading and failure state and
// offers refresh; group authorization remains owned by the endpoint on every read.
export function useGroupMonitoring(enabled: Ref<boolean>) {
  const auth = useAuthStore()
  const snapshot = ref<MonitoringSnapshot | null>(null)
  const loading = ref(false)
  const failed = ref(false)
  let generation = 0

  // byID matches observations to catalog identities without granting extra selection
  // access; callers still construct their selectable groups from the authorized catalog.
  const byID = computed(
    () => new Map(snapshot.value?.groups.map((group) => [group.id, group]) ?? []),
  )

  // refresh reads observations while allowing only the latest account scope to publish.
  // Disabled consumers skip the request. A failed read keeps same-account observations
  // and exposes failure state so selection remains possible with an explicit warning.
  // Capturing the account as well as a generation also protects responses completing
  // between synchronous identity invalidation and the following request. Loading state
  // belongs to the same owner, preventing an older completion from hiding newer work.
  async function refresh(): Promise<void> {
    if (!enabled.value || !auth.user?.id || loading.value) return
    const current = ++generation
    const userID = auth.user.id
    loading.value = true
    failed.value = false
    try {
      const observed = await readMonitoring(24)
      if (current === generation && userID === auth.user?.id) snapshot.value = observed
    } catch {
      if (current === generation) failed.value = true
    } finally {
      if (current === generation) loading.value = false
    }
  }

  // Clear private observations before a new account can render; retiring pending
  // requests also lets the next enabled scope start without waiting for old work.
  watch(
    [() => auth.user?.id, enabled],
    () => {
      generation++
      snapshot.value = null
      loading.value = false
      failed.value = false
      void refresh()
    },
    { immediate: true, flush: 'sync' },
  )
  onBeforeUnmount(() => {
    // Pending reads may finish after navigation; retire their owner before releasing
    // this consumer so no completion can publish measurements into a removed scope.
    generation++
  })

  return { byID, loading, failed, snapshot, refresh }
}
