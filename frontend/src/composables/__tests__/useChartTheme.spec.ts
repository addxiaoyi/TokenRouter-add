import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { effectScope, nextTick } from 'vue'
import { setTheme, setThemePalette } from '@/composables/useTheme'
import {
  useChartTheme,
  CHART_OTHER_COLOR,
  CHART_PALETTE,
  CHART_SERIES_COLORS,
} from '@/composables/useChartTheme'

// useChartTheme.spec.ts checks shared chart palettes and reactive theme integration. The
// cases exercise actual theme changes and watcher disposal, and retain exact color
// assertions so chart consumers can rely on stable semantic series and distribution
// colors.

describe('useChartTheme', () => {
  beforeEach(() => {
    setTheme(false)
    setThemePalette('terracotta')
  })
  afterEach(() => {
    setTheme(false)
    setThemePalette('terracotta')
  })

  // A chart theme is created inside a disposable effect scope before light and dark
  // modes are selected. Every editorial palette retains reactive text and grid colors
  // so changing mode cannot leave chart labels on the previous surface.
  it.each(['terracotta', 'pearl', 'indigo'] as const)(
    'updates semantic colors with light and dark mode in the %s palette',
    (palette) => {
      setThemePalette(palette)
      const scope = effectScope()
      const theme = scope.run(() => useChartTheme())!

      try {
        setTheme(false)
        expect(theme.colors.value).toEqual({
          text: '#2B2926',
          muted: '#757067',
          grid: 'rgba(24, 22, 20, 0.08)',
        })

        setTheme(true)
        expect(theme.colors.value).toEqual({
          text: '#E7E5DF',
          muted: '#9C988F',
          grid: 'rgba(255, 255, 255, 0.08)',
        })
      } finally {
        scope.stop()
      }
    },
  )

  // A subscribed chart observes mode and palette changes before its watcher stops.
  // Later changes leave notifications unchanged after disposal, including a palette
  // switch that keeps the same resolved dark mode.
  it('notifies mode and palette changes until its watcher is stopped', async () => {
    const scope = effectScope()
    const theme = scope.run(() => useChartTheme())!
    const seen: boolean[] = []
    const stop = theme.onThemeChange((dark) => seen.push(dark))

    try {
      setTheme(true)
      // Redraw callbacks run in Vue's queued watcher flush, so each selection must
      // settle before the next change can be checked independently.
      await nextTick()
      setThemePalette('pearl')
      await nextTick()
      setTheme(false)
      await nextTick()
      expect(seen).toEqual([true, true, false])

      stop()
      setTheme(true)
      setThemePalette('indigo')
      await nextTick()
      expect(seen).toEqual([true, true, false])
    } finally {
      scope.stop()
    }
  })

  // Distribution charts assign twelve restrained editorial colors in ranking order.
  // Exact values and the separate Others tone protect category identity and prevent
  // the aggregate remainder from borrowing a ranked category's color.
  it('keeps the editorial distribution palette and Others color stable', () => {
    expect(CHART_PALETTE).toEqual([
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
    ])
    expect(CHART_OTHER_COLOR).toBe('#9C968B')
    expect(CHART_PALETTE).not.toContain(CHART_OTHER_COLOR)
  })

  // Token charts distinguish input, output, cache creation, cache reads, and cache hit
  // rate through assigned colors. Each semantic series must retain its expected shared
  // value.
  it('keeps semantic token series colors stable', () => {
    expect(CHART_SERIES_COLORS).toEqual({
      input: '#C46346',
      output: '#2A7A72',
      cacheCreation: '#C48A3B',
      cacheRead: '#4E627A',
      cacheHitRate: '#8B5E7B',
    })
  })
})
