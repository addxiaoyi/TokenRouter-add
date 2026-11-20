import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type RouteLocationNormalized } from 'vue-router'
import { useRoutePrefetch } from '../useRoutePrefetch'

// useRoutePrefetch.spec.ts exercises delegated link intent against Vue Router's
// real matcher with controlled lazy modules. Fake browser idle work separates
// hover dwell, cancellation, and module completion without waiting on wall time.
// Network and visibility fixtures verify that speculative imports yield to user
// constraints, while explicit navigation remains outside the scheduler's ownership.

// Browser capability fixtures and disposer ownership are reset after each test so
// delegated document events and connection hints cannot leak into unrelated cases.
const network = { saveData: false, effectiveType: '4g' }
const originalConnection = Object.getOwnPropertyDescriptor(navigator, 'connection')
const disposers: Array<() => void> = []

// createHarness builds lazy routes with independent import spies and starts link warming.
// The normalized current route supplies audience metadata without navigating or mounting
// components, so assertions can distinguish speculative loading from real route effects.
function createHarness(currentPath = '/dashboard') {
  const imports = {
    dashboard: vi.fn().mockResolvedValue({ default: {} }),
    keys: vi.fn().mockResolvedValue({ default: {} }),
    usage: vi.fn().mockResolvedValue({ default: {} }),
    models: vi.fn().mockResolvedValue({ default: {} }),
    login: vi.fn().mockResolvedValue({ default: {} }),
    admin: vi.fn().mockResolvedValue({ default: {} }),
    detail: vi.fn().mockResolvedValue({ default: {} }),
    notFound: vi.fn().mockResolvedValue({ default: {} }),
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/dashboard', component: imports.dashboard, meta: { requiresAuth: true } },
      { path: '/keys', component: imports.keys, meta: { requiresAuth: true } },
      { path: '/usage', component: imports.usage, meta: { requiresAuth: true } },
      { path: '/models', component: imports.models, meta: { requiresAuth: false } },
      { path: '/login', component: imports.login, meta: { requiresAuth: false } },
      {
        path: '/admin/dashboard',
        component: imports.dashboard,
        meta: { requiresAuth: true, requiresAdmin: true },
      },
      {
        path: '/admin/users',
        component: imports.admin,
        meta: { requiresAuth: true, requiresAdmin: true },
      },
      { path: '/detail/:id', component: imports.detail, meta: { requiresAuth: true } },
      { path: '/redirect', redirect: '/keys' },
      { path: '/eager', component: { render: () => null } },
      { path: '/:pathMatch(.*)*', component: imports.notFound },
    ],
  })
  const prefetch = useRoutePrefetch(router)
  prefetch.triggerPrefetch(router.resolve(currentPath) as RouteLocationNormalized)
  disposers.push(prefetch.dispose)
  return { router, imports, prefetch }
}

// appendLink creates an ordinary connected destination with nested SVG artwork.
// Optional attributes allow external-tab, disabled, and download exclusions to be
// tested through the same delegated browser event path as actual navigation links.
function appendLink(href: string, attributes: Record<string, string> = {}): HTMLAnchorElement {
  const anchor = document.createElement('a')
  anchor.href = href
  anchor.innerHTML = '<svg><path /></svg>'
  for (const [name, value] of Object.entries(attributes)) anchor.setAttribute(name, value)
  document.body.append(anchor)
  return anchor
}

// hover dispatches bubbling pointer intent from a link or its SVG child element.
function hover(element: Element): void {
  element.dispatchEvent(new MouseEvent('pointerover', { bubbles: true }))
}

beforeEach(() => {
  vi.useFakeTimers()
  network.saveData = false
  network.effectiveType = '4g'
  Object.defineProperty(navigator, 'connection', { configurable: true, value: network })
  vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true)
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  vi.stubGlobal('requestIdleCallback', (callback: IdleRequestCallback) =>
    setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 20 }), 0),
  )
  vi.stubGlobal('cancelIdleCallback', (handle: number) => clearTimeout(handle))
})

afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose()
  document.body.replaceChildren()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  if (originalConnection) Object.defineProperty(navigator, 'connection', originalConnection)
  else Reflect.deleteProperty(navigator, 'connection')
})

describe('useRoutePrefetch', () => {
  // Reaching a dashboard creates the scheduler without interacting with links.
  // Even after all idle timers, no guessed neighbor or unrelated administrator page loads.
  it('keeps every route lazy until the user expresses link intent', async () => {
    const { imports } = createHarness()
    await vi.runAllTimersAsync()
    for (const load of Object.values(imports)) expect(load).not.toHaveBeenCalled()
  })

  // Pointer movement crosses SVG children before the short hover dwell completes.
  // Exactly one matched destination loads afterward, with no router navigation or component mount.
  it('warms SVG link destinations after dwell without navigating', async () => {
    const { imports, router, prefetch } = createHarness()
    const anchor = appendLink('/keys?create=1#quota')
    hover(anchor.querySelector('path')!)
    await vi.advanceTimersByTimeAsync(119)
    expect(imports.keys).not.toHaveBeenCalled()
    await vi.runAllTimersAsync()
    expect(imports.keys).toHaveBeenCalledTimes(1)
    expect(prefetch.prefetchedRoutes.value.has('/keys')).toBe(true)
    expect(router.currentRoute.value.matched).toHaveLength(0)
  })

  // Keyboard focus selects a destination without any pointer hover dwell.
  // The component still waits for an idle task before its module loader executes.
  it('prioritizes keyboard focus in the next idle task', async () => {
    const { imports } = createHarness()
    const anchor = appendLink('/usage')
    hover(anchor)
    anchor.focus()
    expect(imports.usage).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(imports.usage).toHaveBeenCalledTimes(1)
  })

  // The pointer briefly touches one destination before moving to another link.
  // Only the latest sustained intent may import, avoiding speculative downloads during crossings.
  it('replaces pending hover work with the newest link', async () => {
    const { imports } = createHarness()
    hover(appendLink('/keys'))
    await vi.advanceTimersByTimeAsync(50)
    hover(appendLink('/usage'))
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    expect(imports.usage).toHaveBeenCalledTimes(1)
  })

  // Leaving the intended link before dwell ends removes its queued import.
  // Moving between SVG descendants instead retains the same deliberate destination intent.
  it('cancels departed links while retaining movement inside their SVG', async () => {
    const { imports } = createHarness()
    const anchor = appendLink('/keys')
    hover(anchor)
    anchor.dispatchEvent(new MouseEvent('pointerout', { bubbles: true }))
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    hover(anchor)
    anchor.dispatchEvent(
      new MouseEvent('pointerout', { bubbles: true, relatedTarget: anchor.querySelector('path') }),
    )
    await vi.runAllTimersAsync()
    expect(imports.keys).toHaveBeenCalledTimes(1)
  })

  // Cancelled navigation work must never dispatch after a destination transition starts.
  // A successful next route merely rearms intent handling instead of importing its own neighbors.
  it('cancels queued work before navigation and rearms afterward', async () => {
    const { imports, router, prefetch } = createHarness()
    hover(appendLink('/keys'))
    prefetch.cancelPendingPrefetch()
    prefetch.triggerPrefetch(router.resolve('/usage') as RouteLocationNormalized)
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    expect(imports.usage).not.toHaveBeenCalled()
  })

  // A pending module remains unresolved while the user focuses two later destinations.
  // The single active slot serializes imports and retains only the latest waiting link.
  it('serializes imports and keeps only the latest waiting destination', async () => {
    const { imports } = createHarness()
    let finishKeys!: () => void
    imports.keys.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishKeys = resolve
        }),
    )
    appendLink('/keys').focus()
    await vi.runAllTimersAsync()
    appendLink('/usage').focus()
    appendLink('/models').focus()
    await vi.runAllTimersAsync()
    expect(imports.usage).not.toHaveBeenCalled()
    expect(imports.models).not.toHaveBeenCalled()
    finishKeys()
    await vi.runAllTimersAsync()
    expect(imports.models).toHaveBeenCalledTimes(1)
    expect(imports.usage).not.toHaveBeenCalled()
  })

  // Different detail URLs share one dynamic route component despite distinct parameters.
  // Successful prefetches deduplicate by matched record while unrelated route loaders remain untouched.
  it('deduplicates targets across query, fragment, and parameter variants', async () => {
    const { imports } = createHarness()
    hover(appendLink('/detail/first?tab=usage#today'))
    await vi.runAllTimersAsync()
    hover(appendLink('/detail/second?tab=billing'))
    await vi.runAllTimersAsync()
    expect(imports.detail).toHaveBeenCalledTimes(1)
  })

  // Optional warming fails once while normal router state remains unchanged.
  // The warning identifies the destination, and a later deliberate hover retries the rejected module.
  it('reports failed imports and retries them on later intent', async () => {
    const { imports, prefetch } = createHarness()
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const failure = new Error('Temporary module network failure')
    imports.keys.mockRejectedValueOnce(failure)
    const anchor = appendLink('/keys')
    hover(anchor)
    await vi.runAllTimersAsync()
    expect(warning).toHaveBeenCalledWith('[Prefetch] Could not load /keys:', failure)
    expect(prefetch.prefetchedRoutes.value.has('/keys')).toBe(false)
    hover(anchor)
    await vi.runAllTimersAsync()
    expect(imports.keys).toHaveBeenCalledTimes(2)
    expect(prefetch.prefetchedRoutes.value.has('/keys')).toBe(true)
  })

  // A reset occurs while an imported route is still awaiting its module response.
  // Its old completion cannot repopulate reset state, and later imports still wait for settlement.
  it('keeps reset state empty when an older import completes', async () => {
    const { imports, prefetch } = createHarness()
    let finishKeys!: () => void
    imports.keys.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishKeys = resolve
        }),
    )
    appendLink('/keys').focus()
    await vi.runAllTimersAsync()
    prefetch.resetPrefetchState()
    appendLink('/usage').focus()
    await vi.runAllTimersAsync()
    expect(imports.usage).not.toHaveBeenCalled()
    finishKeys()
    await vi.runAllTimersAsync()
    expect(prefetch.prefetchedRoutes.value.has('/keys')).toBe(false)
    expect(prefetch.prefetchedRoutes.value.has('/usage')).toBe(true)
  })

  // Data saving and reported mobile network classes disable optional route downloads.
  // Ordinary route activation remains the router's responsibility and is never intercepted here.
  it.each(['save-data', 'slow-2g', '2g', '3g'])(
    'skips constrained connections: %s',
    async (hint) => {
      const { imports } = createHarness()
      network.saveData = hint === 'save-data'
      network.effectiveType = hint === 'save-data' ? '4g' : hint
      appendLink('/keys').focus()
      await vi.runAllTimersAsync()
      expect(imports.keys).not.toHaveBeenCalled()
    },
  )

  // Visibility or connectivity changes after hover must still prevent speculative dispatch.
  // Rechecking at the idle callback protects requests queued while the page was eligible.
  it.each(['hidden', 'offline', 'save-data'])(
    'rechecks eligibility before dispatch: %s',
    async (hint) => {
      const { imports } = createHarness()
      hover(appendLink('/keys'))
      if (hint === 'hidden') vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
      if (hint === 'offline') vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
      if (hint === 'save-data') network.saveData = true
      await vi.runAllTimersAsync()
      expect(imports.keys).not.toHaveBeenCalled()
    },
  )

  // The browser invokes an idle callback with no usable evaluation time remaining.
  // The scheduler declines that slice, and a fresh link intent can retry when idle time returns.
  it('does not force module evaluation into an exhausted idle slice', async () => {
    const { imports } = createHarness()
    vi.stubGlobal('requestIdleCallback', (callback: IdleRequestCallback) =>
      setTimeout(() => callback({ didTimeout: true, timeRemaining: () => 0 }), 0),
    )
    const anchor = appendLink('/keys')
    hover(anchor)
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    vi.stubGlobal('requestIdleCallback', (callback: IdleRequestCallback) =>
      setTimeout(() => callback({ didTimeout: false, timeRemaining: () => 20 }), 0),
    )
    hover(anchor)
    await vi.runAllTimersAsync()
    expect(imports.keys).toHaveBeenCalledTimes(1)
  })

  // Some browsers expose no requestIdleCallback implementation for optional scheduling.
  // A separate cancellable timer task still follows hover dwell and loads only the intended route.
  it('supports the timer fallback without idle browser APIs', async () => {
    const { imports } = createHarness()
    vi.stubGlobal('requestIdleCallback', undefined)
    vi.stubGlobal('cancelIdleCallback', undefined)
    hover(appendLink('/keys'))
    await vi.runAllTimersAsync()
    expect(imports.keys).toHaveBeenCalledTimes(1)
  })

  // External, disabled, download, and new-tab links belong to native browser behavior.
  // The scheduler must leave every one untouched even when keyboard focus expresses intent.
  it.each([
    ['/keys', { download: '' }],
    ['/keys', { target: '_blank' }],
    ['/keys', { rel: 'external' }],
    ['/keys', { 'aria-disabled': 'true' }],
    ['https://external.example/keys', {}],
    ['javascript:void(0)', {}],
    ['http://[', {}],
  ])('ignores ineligible links: %s %j', async (href, attributes) => {
    const { imports } = createHarness()
    appendLink(href as string, attributes as Record<string, string>).focus()
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
  })

  // Public pages must not speculate authenticated modules, and user pages must not warm admin modules.
  // Reaching an administrator route enables its audience without bypassing any real navigation guard.
  it('keeps speculative modules inside the successfully reached audience', async () => {
    const { imports, router, prefetch } = createHarness('/login')
    appendLink('/keys').focus()
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    prefetch.triggerPrefetch(router.resolve('/dashboard') as RouteLocationNormalized)
    appendLink('/admin/users').focus()
    await vi.runAllTimersAsync()
    expect(imports.admin).not.toHaveBeenCalled()
    prefetch.triggerPrefetch({
      ...router.resolve('/admin/dashboard'),
      meta: { requiresAuth: true, requiresAdmin: true },
    } as RouteLocationNormalized)
    appendLink('/admin/users').focus()
    await vi.runAllTimersAsync()
    expect(imports.admin).toHaveBeenCalledTimes(1)
  })

  // Redirect-only records, eagerly loaded components, and unknown catch-all pages need no warming.
  // Their focus events cannot accidentally preload the destination or evaluate unrelated fallback modules.
  it.each(['/redirect', '/eager', '/missing', '/dashboard#chart'])(
    'skips non-lazy or current routes: %s',
    async (href) => {
      const { imports } = createHarness()
      appendLink(href).focus()
      await vi.runAllTimersAsync()
      for (const load of Object.values(imports)) expect(load).not.toHaveBeenCalled()
    },
  )

  // Page exit disposes delegated listeners while a hover timer is still pending.
  // Later focus remains inert until a successful navigation rearms the scheduler explicitly.
  it('disposes pending work on page exit and supports later reactivation', async () => {
    const { imports, router, prefetch } = createHarness()
    hover(appendLink('/keys'))
    window.dispatchEvent(new Event('pagehide'))
    await vi.runAllTimersAsync()
    appendLink('/usage').focus()
    await vi.runAllTimersAsync()
    expect(imports.keys).not.toHaveBeenCalled()
    expect(imports.usage).not.toHaveBeenCalled()
    prefetch.triggerPrefetch(router.resolve('/dashboard') as RouteLocationNormalized)
    appendLink('/usage').focus()
    await vi.runAllTimersAsync()
    expect(imports.usage).toHaveBeenCalledTimes(1)
  })

  // Missing router instances cannot resolve lazy component records or register useful intent work.
  // Calling the public lifecycle methods remains safe and retains an empty completed-target cache.
  it('leaves missing router instances inert', async () => {
    const prefetch = useRoutePrefetch()
    disposers.push(prefetch.dispose)
    prefetch.triggerPrefetch({ path: '/dashboard', meta: {} } as RouteLocationNormalized)
    appendLink('/keys').focus()
    await vi.runAllTimersAsync()
    expect(prefetch.prefetchedRoutes.value.size).toBe(0)
  })
})
