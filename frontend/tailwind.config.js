// tailwind.config.js maps application design tokens to generated utility classes.
// Radius, overlay, motion, and panel-size utilities read the same CSS variables
// as handwritten styles. Color ramps and font stacks remain shared across pages,
// while dark edge intensity is configured separately from text and surface colors.

// darkEdgeColor creates a Tailwind color resolver for a configured edge alpha.
// The returned resolver multiplies opacityValue by that alpha, keeping modifiers
// such as /70 relative to the selected edge intensity rather than replacing it.
const darkEdgeColor =
  (opacity) =>
  ({ opacityValue = '1' }) =>
    `rgb(252 252 254 / calc(${opacity} * ${opacityValue}))`

// darkEdges supplies shared border, divide, and ring opacities while text and surface utilities use separate color ramps.
const darkEdges = {
  400: darkEdgeColor(0.28),
  500: darkEdgeColor(0.14),
  600: darkEdgeColor(0.085),
  700: darkEdgeColor(0.055),
  800: darkEdgeColor(0.04),
  900: darkEdgeColor(0.025),
}

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    // Radius utilities reference the same root variables as handwritten styles.
    // The UI check accepts these semantic tiers and rejects legacy numeric-scale names.
    borderRadius: {
      none: '0px',
      compact: 'var(--radius-compact)',
      control: 'var(--radius-control)',
      surface: 'var(--radius-surface)',
      dialog: 'var(--radius-dialog)',
      full: '9999px',
    },
    extend: {
      borderColor: { dark: darkEdges },
      divideColor: { dark: darkEdges },
      ringColor: { dark: darkEdges },
      opacity: {
        6: '0.06',
        8: '0.08',
      },
      // Overlay utilities reference root z-index variables. The tour's externally
      // constrained layer remains in its integration stylesheet instead of becoming
      // a utility that unrelated application panels could depend on.
      zIndex: {
        'chart-tooltip': 'var(--z-chart-tooltip)',
        'sidebar-overlay': 'var(--z-sidebar-overlay)',
        sidebar: 'var(--z-sidebar)',
        header: 'var(--z-header)',
        modal: 'var(--z-modal)',
        'modal-nested': 'var(--z-modal-nested)',
        tooltip: 'var(--z-tooltip)',
        announcement: 'var(--z-announcement)',
        'announcement-raised': 'var(--z-announcement-raised)',
        'announcement-top': 'var(--z-announcement-top)',
        'menu-overlay': 'var(--z-menu-overlay)',
        toast: 'var(--z-toast)',
        'action-menu': 'var(--z-action-menu)',
        'teleport-tooltip': 'var(--z-teleport-tooltip)',
        'help-tooltip': 'var(--z-help-tooltip)',
        'teleport-dropdown': 'var(--z-teleport-dropdown)',
      },
      // Panel height tiers reference root variables shared by menus, dialogs, and handwritten layout rules.
      maxHeight: {
        'menu-sm': 'var(--max-h-menu-sm)',
        menu: 'var(--max-h-menu)',
        panel: 'var(--max-h-panel)',
      },
      colors: {
        // Dynamic editorial accent ramp driven by root RGB tokens so multi-theme palettes switch live.
        primary: {
          50: 'rgb(var(--color-primary-50) / <alpha-value>)',
          100: 'rgb(var(--color-primary-100) / <alpha-value>)',
          200: 'rgb(var(--color-primary-200) / <alpha-value>)',
          300: 'rgb(var(--color-primary-300) / <alpha-value>)',
          400: 'rgb(var(--color-primary-400) / <alpha-value>)',
          500: 'rgb(var(--color-primary-500) / <alpha-value>)',
          600: 'rgb(var(--color-primary-600) / <alpha-value>)',
          700: 'rgb(var(--color-primary-700) / <alpha-value>)',
          800: 'rgb(var(--color-primary-800) / <alpha-value>)',
          900: 'rgb(var(--color-primary-900) / <alpha-value>)',
          950: 'rgb(var(--color-primary-950) / <alpha-value>)',
        },
        accent: {
          50: 'rgb(var(--color-primary-50) / <alpha-value>)',
          100: 'rgb(var(--color-primary-100) / <alpha-value>)',
          200: 'rgb(var(--color-primary-200) / <alpha-value>)',
          300: 'rgb(var(--color-primary-300) / <alpha-value>)',
          400: 'rgb(var(--color-primary-400) / <alpha-value>)',
          500: 'rgb(var(--color-primary-500) / <alpha-value>)',
          600: 'rgb(var(--color-primary-600) / <alpha-value>)',
          700: 'rgb(var(--color-primary-700) / <alpha-value>)',
          800: 'rgb(var(--color-primary-800) / <alpha-value>)',
          900: 'rgb(var(--color-primary-900) / <alpha-value>)',
          950: 'rgb(var(--color-primary-950) / <alpha-value>)',
        },
        // Warm gallery alabaster & obsidian stone neutral scale (Anthropic / OpenAI editorial craft).
        gray: {
          50: '#FAF9F6',
          100: '#F3F1EC',
          200: '#E7E4DD',
          300: '#D5D0C7',
          400: '#A5A097',
          500: '#767169',
          600: '#56524C',
          700: '#413E39',
          800: '#272522',
          900: '#181715',
          950: '#0E0E0D',
        },
        slate: {
          50: '#FAF9F6',
          100: '#F3F1EC',
          200: '#E7E4DD',
          300: '#D5D0C7',
          400: '#A5A097',
          500: '#767169',
          600: '#56524C',
          700: '#413E39',
          800: '#272522',
          900: '#181715',
          950: '#0E0E0D',
        },
        // @project-doc docs/architecture/frontend_ui_conventions.md#dark_colors
        // Warm obsidian neutral ramp for dark editorial surfaces; border, divide, and ring intensity comes from darkEdges.
        dark: {
          50: '#FAF9F6', // Heading and emphasis text.
          100: '#EFECE6', // Body, navigation, and header foreground.
          200: '#D8D4CC', // Secondary strong foreground.
          300: '#A8A39A', // Secondary text foreground.
          400: '#7B766E', // Hints and table headings.
          500: '#5A5650', // Icons and disabled foreground.
          600: '#3D3A36', // Strong neutral fill.
          700: '#2A2825', // Chips, borders, and disabled fills.
          800: '#211F1D', // Row hover and nested surfaces.
          900: '#181715', // Cards, dialogs, and dropdowns.
          950: '#0E0E0D', // Page canvas, inputs, secondary controls, and code surfaces.
        },
      },
      fontFamily: {
        // Modern all-sans editorial typography: Plus Jakarta Sans for Latin, PingFang SC / Noto Sans SC for CJK.
        sans: [
          '"Plus Jakarta Sans Variable"',
          '"PingFang SC"',
          '"Noto Sans SC Variable"',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Helvetica Neue',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif',
        ],
        mono: [
          '"Geist Mono Variable"',
          'ui-monospace',
          'SFMono-Regular',
          'Menlo',
          'Monaco',
          'Consolas',
          'monospace',
        ],
      },
      boxShadow: {
        // Refined ambient shadows supporting translucent Liquid Glass panels without heavy blur clutter.
        DEFAULT: '0 1px 2px rgb(24 23 21 / 0.04), 0 4px 12px -4px rgb(24 23 21 / 0.05)',
        sm: '0 1px 2px rgb(24 23 21 / 0.04)',
        glass: '0 2px 6px rgb(24 23 21 / 0.03), 0 16px 40px -12px rgb(24 23 21 / 0.08)',
        'glass-sm': '0 1px 3px rgb(24 23 21 / 0.04), 0 8px 20px -8px rgb(24 23 21 / 0.06)',
        glow: '0 0 24px rgb(var(--color-primary-500) / 0.16)',
        'glow-lg': '0 0 40px rgb(var(--color-primary-500) / 0.22)',
        card: '0 2px 6px rgb(24 23 21 / 0.03), 0 12px 32px -10px rgb(24 23 21 / 0.07)',
        'card-hover': '0 6px 16px -4px rgb(24 23 21 / 0.06), 0 22px 44px -12px rgb(24 23 21 / 0.11)',
        'inner-glow': 'inset 0 1px 0 rgb(255 255 255 / 0.85)',
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #00D2FF 0%, #0B8FD8 100%)',
        'gradient-dark': 'linear-gradient(135deg, #17171A 0%, #0A0A0B 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(0, 210, 255, 0.14) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(139, 221, 248, 0.12) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(18, 167, 232, 0.1) 0px, transparent 50%)',
      },
      // Transition utilities read the shared root timing variables also used by handwritten motion styles.
      transitionDuration: {
        DEFAULT: 'var(--motion-fast)',
        fast: 'var(--motion-fast)',
        normal: 'var(--motion-normal)',
        layout: 'var(--motion-layout)',
      },
      transitionTimingFunction: {
        DEFAULT: 'var(--motion-ease)',
        standard: 'var(--motion-ease)',
        exit: 'var(--motion-ease-exit)',
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(0, 210, 255, 0.28)' },
          '100%': { boxShadow: '0 0 30px rgba(18, 167, 232, 0.4)' },
        },
      },
      backdropBlur: {
        xs: '2px',
      },
    },
  },
  plugins: [],
}
