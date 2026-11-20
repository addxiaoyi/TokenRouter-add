import { ref } from 'vue'

// useTheme.ts shares the resolved dark state and selected theme mode across the
// application. Explicit light or dark choices are stored in localStorage, while system
// mode removes that preference and resolves the operating system color scheme.
// Initialization synchronizes the document class before registering one persistent media-query listener.
//
// Theme changes suppress transitions across two animation frames so reactive component
// updates and painting use the chosen palette together. A later switch cancels the
// currently pending release frame and starts that interval again. The listener reacts only
// in system mode, and consumers share module state rather than owning separate listeners
// or cleanup.

// ThemeMode selects explicit light, explicit dark, or operating-system theme resolution.
export type ThemeMode = 'light' | 'dark' | 'system'

// ThemePalette selects the editorial Liquid Glass color world and optical light field.
export type ThemePalette = 'terracotta' | 'pearl' | 'indigo'

// themeStorageKey, paletteStorageKey, themeSwitchingClass, and systemDarkQuery identify
// persisted preferences, transition suppression, and system palette detection.
const themeStorageKey = 'theme'
const paletteStorageKey = 'theme_palette'
const themeSwitchingClass = 'theme-switching'
const systemDarkQuery = '(prefers-color-scheme: dark)'
// isDark, themeMode, and themePalette are module-shared reactive values.
const isDark = ref(document.documentElement.classList.contains('dark')) // check-ui-allow: reactive theme primitive owns document state
const themeMode = ref<ThemeMode>('system')
const themePalette = ref<ThemePalette>('terracotta')
// themeSwitchFrame owns the pending transition-release frame; systemThemeMedia retains the
// single process-lifetime browser preference listener after initialization.
let themeSwitchFrame: number | undefined
let systemThemeMedia: MediaQueryList | undefined

// readSavedThemeMode reads the stored explicit theme choice from localStorage. Values
// other than light or dark select system mode; storage access errors propagate to the
// initialization caller.
function readSavedThemeMode(): ThemeMode {
  const savedTheme = localStorage.getItem(themeStorageKey)
  return savedTheme === 'dark' || savedTheme === 'light' ? savedTheme : 'system'
}

// systemPrefersDark reports the operating system preference using matchMedia when
// available. Browsers without that API return false, which resolves system mode to the
// light palette during initialization.
function systemPrefersDark(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia(systemDarkQuery).matches
}

// resolveIsDark converts a theme mode to the current dark-palette flag.
function resolveIsDark(mode: ThemeMode): boolean {
  return mode === 'system' ? systemPrefersDark() : mode === 'dark'
}

// applyTheme updates the shared reactive flag and document dark class.
function applyTheme(nextIsDark: boolean) {
  isDark.value = nextIsDark
  document.documentElement.classList.toggle('dark', nextIsDark)
}

// watchSystemTheme installs one module-owned listener for operating system palette
// changes. Repeated calls reuse the existing listener, and events apply a palette only
// while system mode is selected and the color differs.
function watchSystemTheme() {
  if (systemThemeMedia || typeof window.matchMedia !== 'function') return
  systemThemeMedia = window.matchMedia(systemDarkQuery)
  systemThemeMedia.addEventListener('change', (event) => {
    if (themeMode.value !== 'system' || event.matches === isDark.value) return
    suspendThemeTransitions()
    applyTheme(event.matches)
  })
}

// readSavedThemePalette reads the stored editorial palette choice from localStorage.
function readSavedThemePalette(): ThemePalette {
  const saved = localStorage.getItem(paletteStorageKey)
  return saved === 'pearl' || saved === 'indigo' || saved === 'terracotta' ? saved : 'terracotta'
}

// applyPalette updates the shared reactive palette and document data-palette attribute.
function applyPalette(nextPalette: ThemePalette) {
  themePalette.value = nextPalette
  document.documentElement.dataset.palette = nextPalette
}

// initTheme reads the saved mode and palette, applies their resolved document state, and
// installs system observation. Call it during browser startup before mounting views.
export function initTheme() {
  themeMode.value = readSavedThemeMode()
  applyTheme(resolveIsDark(themeMode.value))
  applyPalette(readSavedThemePalette())
  watchSystemTheme()
}

// suspendThemeTransitions holds the document transition-suppression class across two
// animation frames. The first frame permits reactive updates to reach rendering and the
// second removes suppression after that interval.
//
// A new switch cancels whichever release frame is currently pending, then replaces the
// frame sequence. This prevents an earlier switch from removing suppression during a later
// palette update. The frame handle becomes undefined after release, distinguishing a
// completed interval from outstanding scheduled work.
function suspendThemeTransitions() {
  const root = document.documentElement
  root.classList.add(themeSwitchingClass)

  if (themeSwitchFrame !== undefined) {
    window.cancelAnimationFrame(themeSwitchFrame)
  }

  // Keep suppression across both reactive rendering and the following paint interval;
  // replacement switches cancel the active release frame.
  themeSwitchFrame = window.requestAnimationFrame(() => {
    themeSwitchFrame = window.requestAnimationFrame(() => {
      root.classList.remove(themeSwitchingClass)
      themeSwitchFrame = undefined
    })
  })
}

// setThemeMode records the chosen mode, persists explicit choices, and resolves its
// palette. System mode removes the stored choice. Storage errors propagate after themeMode
// changes but before document palette application.
//
// When the resolved palette already matches isDark, persistence still occurs and the
// function returns. Otherwise it starts transition suppression before updating reactive
// state and the document class, allowing subsequent component renders to share the same
// suppressed interval. The shared system listener remains installed across mode changes.
export function setThemeMode(mode: ThemeMode) {
  themeMode.value = mode
  // Persist the mode before applying its palette so subsequent startup can restore
  // explicit choices and system mode can clear them.
  if (mode === 'system') {
    localStorage.removeItem(themeStorageKey)
  } else {
    localStorage.setItem(themeStorageKey, mode)
  }

  const nextIsDark = resolveIsDark(mode)
  if (nextIsDark === isDark.value) return
  suspendThemeTransitions()

  applyTheme(nextIsDark)
}

// setTheme selects explicit dark or light mode from a boolean. It delegates persistence
// and palette transition handling to setThemeMode, so it replaces any previous system-following preference.
export function setTheme(nextIsDark: boolean) {
  setThemeMode(nextIsDark ? 'dark' : 'light')
}

// setThemePalette persists and applies the chosen editorial Liquid Glass palette.
export function setThemePalette(nextPalette: ThemePalette) {
  localStorage.setItem(paletteStorageKey, nextPalette)
  if (nextPalette === themePalette.value) return
  suspendThemeTransitions()
  applyPalette(nextPalette)
}

// useTheme returns the module-shared reactive theme state and mutation functions.
// Consumers should initialize the browser theme through initTheme first; toggling chooses
// an explicit palette rather than system mode.
export function useTheme() {
  return {
    isDark,
    themeMode,
    themePalette,
    setTheme,
    setThemeMode,
    setThemePalette,
    toggleTheme: () => setTheme(!isDark.value),
  }
}
