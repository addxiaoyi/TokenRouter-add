import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineAsyncComponent, defineComponent, h, nextTick } from 'vue'
import LazyVisual from '../LazyVisual.vue'

// LazyVisual.spec.ts verifies that decorative module loading waits for nearby
// viewport admission and browser idle. Controlled observer and idle notifications
// reproduce backgrounding, unresolved imports, and renderer errors while checking
// that the reserved static fallback remains available throughout those paths.

// intersection and idle represent independent browser notifications for each fixture;
// cancellation spies verify that deferred work is retired before page teardown.
let intersection: IntersectionObserverCallback
let idle: IdleRequestCallback
let hidden: boolean
const cancelIdle = vi.fn()
const disconnect = vi.fn()

enableAutoUnmount(afterEach)
beforeEach(() => {
  hidden = false
  vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
  vi.stubGlobal(
    'IntersectionObserver',
    vi.fn(function (callback: IntersectionObserverCallback) {
      intersection = callback
      return { observe: vi.fn(), disconnect }
    }),
  )
  vi.stubGlobal(
    'requestIdleCallback',
    vi.fn((callback: IdleRequestCallback) => {
      idle = callback
      return 1
    }),
  )
  vi.stubGlobal('cancelIdleCallback', cancelIdle)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

// enterViewport delivers the browser notification that admits a nearby visual.
function enterViewport() {
  intersection([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
}

// runIdle executes a queued idle callback with available browser frame budget.
function runIdle() {
  idle({ didTimeout: false, timeRemaining: () => 10 })
}

describe('LazyVisual', () => {
  // An async visual sits below the fold while its static artwork reserves space.
  // Its import starts only after admission and idle, and resolved content replaces that fallback.
  it('defers the async import until nearby idle and retains the mounted visual', async () => {
    let resolveVisual!: (visual: ReturnType<typeof defineComponent>) => void
    const loader = vi.fn(
      () =>
        new Promise<ReturnType<typeof defineComponent>>((resolve) => {
          resolveVisual = resolve
        }),
    )
    const visual = defineAsyncComponent(loader)
    const wrapper = mount(LazyVisual, {
      slots: { default: () => h(visual), fallback: '<span>Static artwork</span>' },
    })
    expect(loader).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('Static artwork')
    enterViewport()
    await nextTick()
    expect(loader).not.toHaveBeenCalled()
    runIdle()
    await nextTick()
    expect(loader).toHaveBeenCalledOnce()
    expect(wrapper.text()).toBe('Static artwork')
    resolveVisual(defineComponent({ render: () => h('span', 'Loaded artwork') }))
    await flushPromises()
    expect(wrapper.text()).toBe('Loaded artwork')
    enterViewport()
    await nextTick()
    expect(loader).toHaveBeenCalledOnce()
  })

  // A nearby visual is waiting for idle when the browser tab becomes hidden.
  // Pending work must be cancelled and remain static until visible admission occurs again.
  it('cancels pending activation when the tab is hidden', async () => {
    const content = vi.fn(() => h('span', 'Active artwork'))
    const wrapper = mount(LazyVisual, {
      slots: { default: content, fallback: '<span>Static artwork</span>' },
    })
    enterViewport()
    hidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    expect(cancelIdle).toHaveBeenCalledWith(1)
    runIdle()
    await nextTick()
    expect(content).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('Static artwork')
    hidden = false
    document.dispatchEvent(new Event('visibilitychange'))
    runIdle()
    await nextTick()
    expect(wrapper.text()).toBe('Active artwork')
  })

  // A decorative renderer throws after it is admitted into the visible page.
  // Its failure must preserve the static fallback and report the cause without breaking surrounding content.
  it('keeps fallback artwork after a child renderer fails', async () => {
    const report = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const broken = defineComponent({
      setup() {
        throw new Error('WebGL unavailable')
      },
    })
    const wrapper = mount(LazyVisual, {
      slots: { default: () => h(broken), fallback: '<span>Static artwork</span>' },
    })
    enterViewport()
    runIdle()
    await nextTick()
    expect(wrapper.text()).toBe('Static artwork')
    expect(report).toHaveBeenCalledWith(
      expect.stringContaining('LazyVisual.vue'),
      expect.any(Error),
    )
  })
})
