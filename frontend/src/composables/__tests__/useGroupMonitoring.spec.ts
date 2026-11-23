import { describe, expect, it, vi } from 'vitest'
import { reactive, ref, defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { useGroupMonitoring } from '../useGroupMonitoring'
import type { MonitoringSnapshot } from '@/api/monitoring'

// useGroupMonitoring.spec.ts verifies observation ownership across account changes.
// Delayed API promises reproduce a private response completing after replacement,
// while the mounted composable keeps real synchronous invalidation and cleanup.
const read = vi.hoisted(() => vi.fn())
const auth = reactive({ user: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/monitoring', () => ({ readMonitoring: read }))

describe('group observation scope', () => {
  // A prior account's observation read completes after another account starts reading.
  // Its measurements cannot replace the current account's group map or loading state.
  it('rejects a late snapshot after account replacement', async () => {
    let resolve!: (snapshot: MonitoringSnapshot) => void
    read
      .mockImplementationOnce(
        () =>
          new Promise((yes) => {
            resolve = yes
          }),
      )
      .mockResolvedValueOnce({ enabled: true, start: '', end: '', groups: [] })
    let monitoring!: ReturnType<typeof useGroupMonitoring>
    const view = mount(
      defineComponent({
        setup() {
          monitoring = useGroupMonitoring(ref(true))
          return () => null
        },
      }),
    )
    await flushPromises()
    auth.user = { id: 2 }
    await flushPromises()
    resolve({ enabled: true, start: 'old', end: 'old', groups: [] })
    await flushPromises()
    expect(monitoring.snapshot.value?.end).not.toBe('old')
    expect(monitoring.loading.value).toBe(false)
    view.unmount()
  })
})
