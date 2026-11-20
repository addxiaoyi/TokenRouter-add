// metricTones.ts assigns icon and tile colors to dashboard metric categories. Metric cards
// and the heading statistics share these values, and complete class strings let the Tailwind
// scanner retain every light and dark color variant.

// METRIC_TONES supplies complete Tailwind class strings for color-locked metric icons
// and tinted neutral icon chips across dashboard cards and live statistics.

export const METRIC_TONES = {
  // Requests and RPM carry the active editorial brand accent.
  requests: {
    icon: 'text-primary-600 dark:text-primary-400',
    tile: 'bg-primary-500/12 ring-1 ring-primary-500/15',
  },
  // Token volume and TPM use calm editorial ink on a tinted stone chip.
  tokens: {
    icon: 'text-gray-800 dark:text-dark-100',
    tile: 'bg-black/[0.045] ring-1 ring-black/[0.06] dark:bg-white/[0.07] dark:ring-white/[0.08]',
  },
  // Period and daily spending use restrained warm amber ink on a neutral stone chip.
  cost: {
    icon: 'text-amber-700 dark:text-amber-400',
    tile: 'bg-black/[0.045] ring-1 ring-black/[0.06] dark:bg-white/[0.07] dark:ring-white/[0.08]',
  },
  // Cache-hit rate uses deep sage teal on a neutral stone chip.
  cacheHitRate: {
    icon: 'text-teal-700 dark:text-teal-400',
    tile: 'bg-black/[0.045] ring-1 ring-black/[0.06] dark:bg-white/[0.07] dark:ring-white/[0.08]',
  },
  // Average request latency uses muted ink on a neutral stone chip.
  latency: {
    icon: 'text-gray-700 dark:text-dark-200',
    tile: 'bg-black/[0.045] ring-1 ring-black/[0.06] dark:bg-white/[0.07] dark:ring-white/[0.08]',
  },
} as const

// MetricTone identifies a metric with matching dashboard icon and tile colors.
export type MetricTone = keyof typeof METRIC_TONES
