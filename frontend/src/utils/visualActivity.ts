// visualActivity.ts coordinates decorative renderers with viewport and browser
// visibility. A renderer becomes active only after its element intersects the
// observed area and the document is visible. The caller owns animation frames;
// activity changes tell it when to cancel work or resume from its retained state.
//
// Intersection notifications can arrive after a tab is hidden or after cleanup.
// Every publication rechecks document visibility, and disposal rejects late
// notifications. Reduced-motion observation stays separate because a visible
// renderer still needs one static frame when the preference changes.

// observeVisualActivity publishes whether an element can perform visible work.
// Callers cancel pending frames on false and release the returned observer cleanup
// when their renderer is destroyed. The first publication is false while a
// supported observer awaits its initial intersection; browsers without that API
// use document visibility alone. The optional margin lets lazy content begin
// loading before it enters view without changing the renderer's own bounds.
export function observeVisualActivity(
  element: Element,
  publish: (active: boolean) => void,
  rootMargin = '0px',
): () => void {
  let disposed = false
  let intersecting = typeof IntersectionObserver === 'undefined'
  let previous: boolean | undefined

  // sync combines both sources before publication, so a late intersection cannot
  // resume a renderer while the browser document remains in the background.
  const sync = () => {
    if (disposed) return
    const active = intersecting && !document.hidden
    if (active === previous) return
    previous = active
    publish(active)
  }

  const observer = intersecting
    ? null
    : new IntersectionObserver(
        (entries) => {
          intersecting = entries.some((entry) => entry.isIntersecting)
          sync()
        },
        { rootMargin },
      )
  observer?.observe(element)
  document.addEventListener('visibilitychange', sync)
  sync()

  return () => {
    // Mark disposal first because queued intersection records may outlive the
    // observer disconnect; their callbacks must not reactivate an unmounted renderer.
    disposed = true
    observer?.disconnect()
    document.removeEventListener('visibilitychange', sync)
  }
}

// observeReducedMotion publishes the current preference and later system changes.
// Its caller decides how to render a static equivalent and must invoke the
// returned cleanup when that renderer is destroyed.
export function observeReducedMotion(publish: (reduced: boolean) => void): () => void {
  const query = window.matchMedia?.('(prefers-reduced-motion: reduce)')
  let disposed = false
  // changed forwards the currently selected system preference to this renderer.
  const changed = () => {
    if (!disposed) publish(query?.matches ?? false)
  }
  query?.addEventListener('change', changed)
  changed()
  return () => {
    disposed = true
    query?.removeEventListener('change', changed)
  }
}
