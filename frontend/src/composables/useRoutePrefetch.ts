import { readonly, ref } from 'vue'
import type { RouteLocationNormalized, Router } from 'vue-router'

// useRoutePrefetch.ts warms lazy route components after deliberate link hover or
// keyboard focus. Delegated events cover links rendered by RouterLink, including
// their SVG children, without adding handlers to every navigation component.
// Speculation stays disabled in hidden documents, offline sessions, and connections
// reporting slow speeds or enabled data saving. Visiting a page never downloads its neighbors.
//
// One import runs at a time, with only the latest pending link retained. Navigation,
// pointer departure, and disposal cancel work before dispatch; JavaScript imports
// already started cannot be aborted. Successful destination records are deduplicated
// across links and parameters. Reset advances a generation so an older completion
// cannot repopulate cleared state, while its active slot remains occupied until the
// import settles. Prefetch never mounts components or runs navigation guards.

// ComponentImportFn represents a route-owned lazy component module loader.
type ComponentImportFn = () => Promise<unknown>

// PrefetchConnection contains optional browser hints used to limit speculative traffic.
interface PrefetchConnection {
  saveData?: boolean // user data-saving preference
  effectiveType?: string // reported effective connection class
}

// PrefetchIntent keeps the latest connected link and its lazy destination together.
interface PrefetchIntent {
  anchor: HTMLAnchorElement // link owning this intent
  path: string // matched route record path
  load: ComponentImportFn // route-owned component loader
}

// HOVER_DELAY_MS filters incidental pointer crossings before loading a destination;
// IDLE_BUDGET_MS leaves a small idle margin before evaluating another route module.
const HOVER_DELAY_MS = 120
const IDLE_BUDGET_MS = 5

// canPrefetch reports whether browser visibility and network hints permit speculation.
// Browsers without connection information retain intent prefetching; explicit data
// saving, 2G, and 3G reports defer module loading until actual navigation.
function canPrefetch(): boolean {
  if (document.visibilityState === 'hidden' || navigator.onLine === false) return false
  const connection = (navigator as Navigator & { connection?: PrefetchConnection }).connection
  return !connection?.saveData && !['slow-2g', '2g', '3g'].includes(connection?.effectiveType ?? '')
}

// scheduleIdle defers one import without forcing it through a busy frame timeout.
// Its returned cancellation function captures the matching scheduling API, including
// the timer fallback used by browsers without requestIdleCallback support.
function scheduleIdle(callback: (deadline?: IdleDeadline) => void): () => void {
  if (typeof window.requestIdleCallback === 'function') {
    const handle = window.requestIdleCallback(callback)
    return () => window.cancelIdleCallback(handle)
  }
  // The hover dwell already establishes intent; a separate task yields to current
  // event handling when this browser cannot expose an actual idle-time budget.
  const handle = setTimeout(callback, 0)
  return () => clearTimeout(handle)
}

// useRoutePrefetch owns one router's optional link-intent module warming state.
// Call triggerPrefetch after successful navigation and cancelPendingPrefetch before
// the next navigation starts. Imports do not provide authorization: existing route
// guards remain responsible for access, and speculative imports conservatively stay
// within the current route's public, authenticated, or administrator audience.
//
// A running import holds its slot until settlement even after reset or disposal.
// Only its captured generation may publish success. Pending work waits for that
// slot, and rejected imports remain retryable with a warning that includes the path.
// Page exit removes delegated listeners; a later successful navigation can attach
// them again after browser history restoration.
// @project-doc docs/architecture/frontend_modules.md#page_loading
export function useRoutePrefetch(router?: Router) {
  // Completed target records, current audience, and queued intent belong to this
  // router instance; a generation separates resets from previously dispatched work.
  const prefetchedRoutes = ref(new Set<string>())
  let currentRoute: RouteLocationNormalized | null = null
  let pending: PrefetchIntent | null = null
  let hoverTimer: ReturnType<typeof setTimeout> | null = null
  let cancelIdle: (() => void) | null = null
  let activePath: string | null = null
  let generation = 0
  let listening = false

  // cancelPendingPrefetch removes dwell and idle work before another destination wins.
  // A dispatched import keeps running because dynamic import has no cancellation API;
  // its successful result may still warm the current generation's destination cache.
  function cancelPendingPrefetch(): void {
    if (hoverTimer !== null) clearTimeout(hoverTimer)
    cancelIdle?.()
    hoverTimer = null
    cancelIdle = null
    pending = null
  }

  // resolveIntent finds a same-origin, ordinary link with an eligible lazy record.
  // Query and fragment suffixes are resolved by Vue Router, while the record path
  // deduplicates different URLs sharing a component. Redirects and catch-all pages
  // are left to real navigation, as are destinations beyond the current audience.
  // Public pages therefore warm public links only, and a user page cannot warm an
  // administrator module. Returning the loader never evaluates it or changes router
  // state, so resolving query-bearing links cannot execute authentication or API work.
  function resolveIntent(event: Event): PrefetchIntent | null {
    if (!router || !currentRoute || !(event.target instanceof Element)) return null
    const anchor = event.target.closest<HTMLAnchorElement>('a[href]')
    if (
      !anchor ||
      anchor.hasAttribute('download') ||
      (anchor.target && anchor.target !== '_self') ||
      anchor.relList.contains('external') ||
      anchor.closest('[inert], [aria-disabled="true"]')
    ) {
      return null
    }
    if (anchor.origin !== window.location.origin) return null
    const target = router.resolve(`${anchor.pathname}${anchor.search}${anchor.hash}`)
    const record = target.matched[target.matched.length - 1]
    if (!record || record.redirect || record.path.includes(':pathMatch(')) return null
    if (target.path === currentRoute.path) return null
    if (target.meta.requiresAdmin && !currentRoute.meta.requiresAdmin) return null
    if (target.meta.requiresAuth !== false && currentRoute.meta.requiresAuth === false) return null
    const component = record.components?.default
    if (
      typeof component !== 'function' ||
      'displayName' in component ||
      'props' in component ||
      '__vccOpts' in component
    ) {
      return null
    }
    return { anchor, path: record.path, load: component as ComponentImportFn }
  }

  // dispatchPrefetch consumes the latest pending destination when the import slot is free.
  // Visibility, connection hints, DOM attachment, and deduplication are checked again
  // immediately before dispatch because they may change during hover or earlier work.
  // A rejection warns without breaking navigation and leaves the destination retryable.
  //
  // Reset cannot abort JavaScript's module request, so publication checks the captured
  // generation. Releasing the active slot always precedes scheduling the latest intent,
  // preventing a new hover from importing concurrently with an older unresolved route.
  async function dispatchPrefetch(deadline?: IdleDeadline): Promise<void> {
    cancelIdle = null
    const intent = pending
    if (!intent || activePath !== null) return
    pending = null
    if (!canPrefetch()) return
    // An exhausted idle slice leaves navigation responsive; another explicit link
    // intent can try again instead of forcing a module evaluation into this frame.
    if (deadline && deadline.timeRemaining() <= IDLE_BUDGET_MS) return
    if (!intent.anchor.isConnected || prefetchedRoutes.value.has(intent.path)) return
    const importGeneration = generation
    activePath = intent.path
    try {
      await intent.load()
      // An obsolete completion must not restore state cleared by reset or page exit;
      // its downloaded module remains in the browser's independent module cache.
      if (importGeneration === generation) prefetchedRoutes.value.add(intent.path)
    } catch (error) {
      // Module warming is optional, so report the rejected destination while allowing
      // its ordinary navigation or a later deliberate hover to attempt loading again.
      console.warn(`[Prefetch] Could not load ${intent.path}:`, error)
    } finally {
      activePath = null
      queueIdlePrefetch()
    }
  }

  // queueIdlePrefetch schedules retained intent only after earlier imports release the slot.
  // The hover timer owns admission until its dwell completes; focus intent can skip
  // that timer but still needs the same free import slot. Only one idle callback
  // belongs to the latest pending link. Replacing the link cancels that callback,
  // while an active import leaves the pending destination for its completion path
  // to schedule without creating concurrent module evaluations.
  function queueIdlePrefetch(): void {
    if (!pending || activePath !== null || hoverTimer !== null || cancelIdle) return
    cancelIdle = scheduleIdle((deadline) => void dispatchPrefetch(deadline))
  }

  // onLinkIntent replaces queued speculation with the latest hovered or focused link.
  // Hover waits for a short dwell, keyboard focus can use the next idle slice, and
  // touch pointer events leave loading to the user's actual navigation action.
  // Focus takes priority even when the pointer already queued that same link.
  // Repeated pointer events among descendants retain the existing dwell rather than
  // delaying it again. An already active destination needs no second queued import.
  function onLinkIntent(event: Event): void {
    if ((event as PointerEvent).pointerType === 'touch') return
    const intent = resolveIntent(event)
    if (intent?.anchor === pending?.anchor && event.type !== 'focusin') return
    cancelPendingPrefetch()
    if (!intent || !canPrefetch()) return
    if (prefetchedRoutes.value.has(intent.path) || activePath === intent.path) return
    pending = intent
    if (event.type === 'focusin') {
      queueIdlePrefetch()
      return
    }
    hoverTimer = setTimeout(() => {
      hoverTimer = null
      queueIdlePrefetch()
    }, HOVER_DELAY_MS)
  }

  // onLinkDeparture cancels queued work when the user leaves its owning link.
  // Moving among a link's child elements or retaining keyboard focus keeps that
  // same intent active, including pointer movements across nested SVG artwork.
  // Events from unrelated elements do not own this cancellation. Keyboard focus
  // remains evidence of intent even after the pointer leaves, while a genuine
  // blur to another destination lets its later focus event replace the pending link.
  function onLinkDeparture(event: Event): void {
    if (!pending) return
    if (!(event.target instanceof Node) || !pending.anchor.contains(event.target)) return
    const nextTarget = (event as FocusEvent).relatedTarget
    if (nextTarget instanceof Node && pending.anchor.contains(nextTarget)) return
    if (pending.anchor.contains(document.activeElement)) return
    cancelPendingPrefetch()
  }

  // onVisibilityChange drops queued imports when the document moves into the background.
  // Returning to the foreground requires another link intent, avoiding stale work
  // that competes with the newly visible page's own rendering and data refresh.
  function onVisibilityChange(): void {
    if (document.visibilityState === 'hidden') cancelPendingPrefetch()
  }

  // resetPrefetchState clears completed targets and cancels work not yet dispatched.
  // Advancing the generation prevents an active import from publishing its completion
  // into the newly empty cache. The active slot remains occupied until settlement,
  // since clearing that slot here would let newly expressed intent start another
  // import concurrently. A later hover may still retry the old destination because
  // browser module caching and this scheduler's bookkeeping have separate lifetimes.
  function resetPrefetchState(): void {
    generation += 1
    cancelPendingPrefetch()
    prefetchedRoutes.value.clear()
  }

  // dispose releases this router's delegated events and pending speculative work.
  // It is idempotent and runs on page exit; triggerPrefetch can reactivate listeners
  // after history restoration without carrying over completed-target bookkeeping.
  function dispose(): void {
    resetPrefetchState()
    document.removeEventListener('pointerover', onLinkIntent)
    document.removeEventListener('focusin', onLinkIntent)
    document.removeEventListener('pointerout', onLinkDeparture)
    document.removeEventListener('focusout', onLinkDeparture)
    document.removeEventListener('visibilitychange', onVisibilityChange)
    window.removeEventListener('pagehide', dispose)
    listening = false
    currentRoute = null
  }

  // triggerPrefetch records the successfully reached audience and enables link intent.
  // It cancels previous pending work but performs no imports itself. Missing routers
  // remain inert, and listeners attach at most once until disposal or page exit.
  function triggerPrefetch(route: RouteLocationNormalized): void {
    cancelPendingPrefetch()
    currentRoute = route
    if (!router || listening) return
    document.addEventListener('pointerover', onLinkIntent, { passive: true })
    document.addEventListener('focusin', onLinkIntent)
    document.addEventListener('pointerout', onLinkDeparture, { passive: true })
    document.addEventListener('focusout', onLinkDeparture)
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener('pagehide', dispose)
    listening = true
  }

  return {
    prefetchedRoutes: readonly(prefetchedRoutes),
    triggerPrefetch,
    cancelPendingPrefetch,
    resetPrefetchState,
    dispose,
  }
}
