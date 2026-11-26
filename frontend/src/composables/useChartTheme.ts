import { computed, watch } from 'vue'
import { Chart as ChartJS } from 'chart.js'
import { useTheme } from './useTheme'

// useChartTheme.ts provides shared chart colors, typography sizes, and reactive theme
// observations. Chart configuration consumers use semantic text and grid colors, while
// distribution and token-series palettes keep their assigned colors across light and dark
// themes. Charts inherit the application's resolved font family instead of relying
// on canvas defaults, which do not inherit styles from surrounding DOM elements.
//
// Chart creation can precede the web font download. A shared initialization hook
// requests that family and redraws each live chart after loading, allowing its
// measured labels to match page typography. Destroyed instances are excluded from
// this completion path, so navigating away while fonts load cannot update a
// disposed chart. Existing theme subscriptions keep their component-owned lifetime.

// CHART_PALETTE assigns an editorial, restrained set of ranked distribution colors
// harmonized with the warm gallery stone and smoked obsidian surfaces.
export const CHART_PALETTE = [
  '#C46346',
  '#2A7A72',
  '#C48A3B',
  '#4E627A',
  '#8B5E7B',
  '#6F8054',
  '#A8795B',
  '#4B5588',
  '#9E4E4E',
  '#567878',
  '#9A7D4A',
  '#6E667A',
] as const

// CHART_OTHER_COLOR identifies the aggregate Others slice independently of the ranked
// distribution palette colors.
export const CHART_OTHER_COLOR = '#9C968B'

// CHART_SERIES_COLORS assigns restrained editorial tones to token input, output,
// cache writes, cache reads, and cache hit-rate series.
export const CHART_SERIES_COLORS = {
  input: '#C46346',
  output: '#2A7A72',
  cacheCreation: '#C48A3B',
  cacheRead: '#4E627A',
  cacheHitRate: '#8B5E7B',
} as const

// CHART_TICK_FONT_SIZE sets the shared pixel size used for labels along chart axes.
export const CHART_TICK_FONT_SIZE = 10

// CHART_LEGEND_FONT_SIZE sets the shared pixel size used for chart series legend text.
export const CHART_LEGEND_FONT_SIZE = 11

// Shared registration covers charts that only import palette constants as well
// as consumers that call the reactive theme composable during component setup.
ChartJS.register({
  id: 'application-font',
  // beforeInit applies the document's resolved font before canvas labels are
  // measured and requests its web font. A chart created during downloading can
  // initially measure fallback glyphs, so successful loading triggers a redraw
  // without animation. The completion checks Chart.js's live instance registry
  // because a route change can destroy this chart before the font arrives.
  // Failed loading retains the browser fallback and reports the cause.
  beforeInit(chart) {
    const family = getComputedStyle(document.documentElement).fontFamily
    if (!family) return
    ChartJS.defaults.font.family = family
    if (!document.fonts) return

    void document.fonts.load(`400 ${CHART_TICK_FONT_SIZE}px ${family}`).then(
      () => {
        // Font completion outlives route mounts; redraw only the registered
        // instance, preventing updates against a chart destroyed during navigation.
        if (ChartJS.instances[chart.id] === chart) chart.update('none')
      },
      (error) => {
        console.warn('[useChartTheme.ts] Application font failed to load', error)
      },
    )
  },
})

// useChartTheme exposes reactive theme colors and a chart redraw subscription. Consumers
// rebuild chart options or update their chart when notified; the returned watcher stop
// function permits early disposal.
export function useChartTheme() {
  const { isDark, themePalette } = useTheme()

  const colors = computed(() =>
    isDark.value
      ? { text: '#E7E5DF', muted: '#9C988F', grid: 'rgba(255, 255, 255, 0.08)' }
      : { text: '#2B2926', muted: '#757067', grid: 'rgba(24, 22, 20, 0.08)' },
  )

  // onThemeChange subscribes a callback to changes in either the resolved dark theme
  // or the active editorial palette. It returns Vue's stop function, and a watcher
  // created during setup also follows that component's disposal.
  const onThemeChange = (callback: (dark: boolean) => void) => {
    return watch([isDark, themePalette], ([dark]) => callback(dark))
  }

  return { isDark, themePalette, colors, onThemeChange }
}
