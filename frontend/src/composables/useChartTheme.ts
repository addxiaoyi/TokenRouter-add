import { computed, watch } from 'vue'
import { useTheme } from './useTheme'

// useChartTheme.ts provides shared chart colors, typography sizes, and reactive theme
// observations. Chart configuration consumers use semantic text and grid colors, while
// distribution and token-series palettes keep their assigned colors across light and dark
// themes.

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
