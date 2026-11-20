import { computed, readonly, ref } from 'vue'
import { isSlashShortcut, isToggleShortcut } from './searchShortcuts'

// globalSearchState.ts holds the shared state of the global search palette. The palette
// is loaded on its first opening by App.vue, while its triggers live in different headers (AppHeader and
// the guest marketplace header), so they share this module-level state instead of passing
// events through the layout tree.
//
// This module listens for keyboard shortcuts only while at least one trigger is mounted.
// Pages without a visible search box, such as login or setup, therefore keep "/" and
// Command+K for their own use. Triggers register on mount and release on unmount; the
// count tolerates overlapping mounts during route transitions, when the leaving and
// entering headers can both exist briefly.

// open records whether the palette is visible; only the functions below change it.
const open = ref(false)

// mountedTriggers counts mounted search triggers that enable the keyboard shortcuts.
const mountedTriggers = ref(0)

// globalSearchOpen exposes the palette visibility to the palette and its triggers.
export const globalSearchOpen = readonly(open)

// globalSearchAvailable reports whether a search trigger is on screen, which enables the
// palette's keyboard shortcuts.
export const globalSearchAvailable = computed(() => mountedTriggers.value > 0)

// openGlobalSearch shows the palette; calling it while open has no effect.
export function openGlobalSearch(): void {
  open.value = true
}

// closeGlobalSearch hides the palette; calling it while closed has no effect.
export function closeGlobalSearch(): void {
  open.value = false
}

// onSearchShortcut opens or toggles search while a header trigger is available.
// Other dialogs retain their keyboard ownership, and unmodified slashes in editable
// controls remain text input. Keeping this listener outside the palette lets the first
// keyboard opening load its component without downloading search results beforehand.
function onSearchShortcut(event: KeyboardEvent): void {
  if (event.isComposing || event.keyCode === 229) return
  const otherDialogOpen = !open.value && document.body.classList.contains('modal-open')
  if (isToggleShortcut(event)) {
    if (otherDialogOpen) return
    event.preventDefault()
    open.value = !open.value
  } else if (!open.value && !otherDialogOpen && isSlashShortcut(event)) {
    event.preventDefault()
    openGlobalSearch()
  }
}

// registerSearchTrigger records a mounted trigger and returns the matching release
// function. Call the release exactly once when the trigger unmounts; extra calls are
// ignored so the count cannot drop below the triggers still on screen.
export function registerSearchTrigger(): () => void {
  // Overlapping headers share one listener; releasing the last trigger also closes
  // search before navigation can leave a palette on a page with no search entry.
  if (mountedTriggers.value === 0) window.addEventListener('keydown', onSearchShortcut)
  mountedTriggers.value++
  let released = false
  return () => {
    if (released) return
    released = true
    mountedTriggers.value--
    if (mountedTriggers.value === 0) {
      window.removeEventListener('keydown', onSearchShortcut)
      closeGlobalSearch()
    }
  }
}
