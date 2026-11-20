import { afterEach, describe, expect, it, vi } from 'vitest'
import { observeReducedMotion, observeVisualActivity } from '../visualActivity'

// visualActivity.spec.ts exercises viewport admission against tab visibility and
// observer cleanup. Browser notifications are controlled independently so late
// intersections cannot silently reactivate background or disposed renderers, and
// live reduced-motion changes remain observable without mounting WebGL fixtures.

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('visual activity', () => {
  // An intersection arrives while a browser tab is hidden and later returns.
  // Only visible intersections may activate work, and cleanup must reject queued callbacks.
  it('combines viewport and document visibility and ignores late notifications', () => {
    let hidden = false
    let intersection!: IntersectionObserverCallback
    const disconnect = vi.fn()
    vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
    vi.stubGlobal(
      'IntersectionObserver',
      vi.fn(function (callback: IntersectionObserverCallback) {
        intersection = callback
        return { observe: vi.fn(), disconnect }
      }),
    )
    const publish = vi.fn()
    const stop = observeVisualActivity(document.createElement('div'), publish)
    expect(publish.mock.calls).toEqual([[false]])

    hidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    intersection(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    )
    expect(publish.mock.calls).toEqual([[false]])

    hidden = false
    document.dispatchEvent(new Event('visibilitychange'))
    expect(publish.mock.calls).toEqual([[false], [true]])
    hidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    expect(publish.mock.calls).toEqual([[false], [true], [false]])

    stop()
    hidden = false
    intersection(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    )
    document.dispatchEvent(new Event('visibilitychange'))
    expect(disconnect).toHaveBeenCalledOnce()
    expect(publish).toHaveBeenCalledTimes(3)
  })

  // A browser without IntersectionObserver still needs usable decorative content.
  // It activates immediately in a visible tab and continues to pause when hidden.
  it('keeps document pausing when viewport observation is unavailable', () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const publish = vi.fn()
    const stop = observeVisualActivity(document.createElement('div'), publish)
    expect(publish).toHaveBeenLastCalledWith(true)
    hidden.mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(publish).toHaveBeenLastCalledWith(false)
    stop()
  })

  // The operating system changes its motion preference while a renderer is mounted.
  // Consumers receive the current value and later changes, then release their listener.
  it('publishes live reduced-motion preference and removes the listener', () => {
    let changed!: () => void
    const query = {
      matches: false,
      addEventListener: vi.fn((_event: string, callback: () => void) => {
        changed = callback
      }),
      removeEventListener: vi.fn(),
    }
    vi.spyOn(window, 'matchMedia').mockReturnValue(query as unknown as MediaQueryList)
    const publish = vi.fn()
    const stop = observeReducedMotion(publish)
    expect(publish).toHaveBeenLastCalledWith(false)
    query.matches = true
    changed()
    expect(publish).toHaveBeenLastCalledWith(true)
    stop()
    expect(query.removeEventListener).toHaveBeenCalledWith('change', changed)
    changed()
    expect(publish).toHaveBeenCalledTimes(2)
  })
})
