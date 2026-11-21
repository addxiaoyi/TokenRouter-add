<template>
  <div class="ba-theme-shell relative min-h-screen text-gray-900 dark:text-dark-50">
    <!-- Non-home architectural topography backdrop with slow ambient motion and a soft glow. -->
    <LazyVisual class="pointer-events-none fixed inset-0 overflow-hidden bg-[var(--page-bg)]">
      <Topography
        :color="topographyColor"
        :line-count="18"
        :line-width="1.05"
        :major-lines="true"
        :elevation="0.85"
        :foreground="0.18"
        :scale="1.15"
        :detail="0.3"
        :tilt="34"
        :speed="0.25"
        direction="forward"
        :morph="0.2"
        :glow="0.6"
        :depth-of-field="0.4"
        :fog="0.4"
        :brightness="isDark ? 0.85 : 0.95"
        :opacity="isDark ? 0.36 : 0.32"
        :mouse-interaction="true"
        hover-effect="lift"
        :mouse-radius="1.2"
        :mouse-strength="0.85"
        :trail="0.45"
        :parallax="0.65"
        click-effect="pulse"
        :intro="false"
        :light-mode="!isDark"
        :dpr="1"
        :max-fps="30"
        :max-cells="42000"
      />
    </LazyVisual>

    <!-- Minimal floating editorial header with brand, locale, and theme toggle -->
    <header
      class="site-header fixed inset-x-0 top-0 z-header border-b border-black/[0.06] dark:border-white/[0.07]"
    >
      <div class="flex h-[var(--header-h)] items-center justify-between px-6 lg:px-10">
        <router-link
          to="/home"
          class="inline-flex items-center gap-2.5 text-sm font-medium tracking-[-0.01em] text-gray-900 transition-colors hover:text-gray-600 dark:text-dark-50 dark:hover:text-dark-300"
        >
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-control border border-white/80 bg-white/80 p-0.5 shadow-sm dark:border-white/[0.12] dark:bg-white/[0.08]"
          >
            <img
              :src="siteLogo || '/logo.svg'"
              :alt="siteName"
              class="h-full w-full object-contain"
            />
          </span>
          <span>{{ siteName }}</span>
        </router-link>

        <div class="flex items-center gap-2">
          <LocaleSwitcher variant="text" />
          <button
            type="button"
            data-testid="auth-theme-toggle"
            class="header-status-icon-button"
            :aria-label="isDark ? t('nav.lightMode') : t('nav.darkMode')"
            :title="isDark ? t('nav.lightMode') : t('nav.darkMode')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
        </div>
      </div>
    </header>

    <!-- Full-bleed Seamless Editorial Canvas split: sculptural optical stage on left, gallery form on right -->
    <div
      v-content-reveal="motionRoute?.path"
      class="relative z-10 grid min-h-screen pt-[var(--header-h)] lg:grid-cols-12"
    >
      <!-- Left 7 columns: open gallery typography + floating translucent Liquid Glass routing showcase -->
      <section
        class="relative hidden flex-col justify-between overflow-hidden border-r border-black/[0.06] p-12 lg:col-span-7 lg:flex xl:p-16 dark:border-white/[0.07]"
      >
        <div class="relative z-10 max-w-xl pt-4">
          <span
            class="inline-flex items-center gap-2 rounded-full border border-white/75 bg-white/55 px-3 py-1 font-mono text-xs text-gray-900/65 backdrop-blur-md dark:border-white/[0.12] dark:bg-white/[0.06] dark:text-dark-50/70"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-primary-500"></span>
            {{ siteName }} · Unified AI Gateway
          </span>
          <h1
            class="mt-7 text-balance text-4xl font-light leading-[1.1] tracking-[-0.038em] text-gray-900 xl:text-[3.1rem] dark:text-dark-50"
          >
            {{ siteSubtitle }}
          </h1>
        </div>

        <!-- Floating translucent Liquid Glass routing showcase visibly refracting the optical orbs behind it -->
        <div class="glass-card relative z-10 mt-12 p-6 xl:p-7">
          <div
            class="flex items-center justify-between border-b border-black/[0.07] pb-4 font-mono text-xs dark:border-white/[0.09]"
          >
            <div class="flex items-center gap-2">
              <span class="h-2.5 w-2.5 rounded-full bg-black/15 dark:bg-white/20"></span>
              <span class="h-2.5 w-2.5 rounded-full bg-black/15 dark:bg-white/20"></span>
              <span class="h-2.5 w-2.5 rounded-full bg-black/15 dark:bg-white/20"></span>
              <span class="ml-2 font-medium text-gray-900 dark:text-dark-50">
                POST /v1/messages
              </span>
            </div>
            <span
              class="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2.5 py-0.5 text-emerald-700 dark:text-emerald-300"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-emerald-500"></span>
              200 OK · 128ms
            </span>
          </div>

          <div class="mt-4 space-y-2.5">
            <div
              v-for="lane in [
                { brand: 'anthropic', model: 'claude-opus-4-6', latency: '128ms', active: true },
                { brand: 'openai', model: 'gpt-5.4', latency: '154ms', active: false },
                { brand: 'gemini', model: 'gemini-3.1-pro', latency: '112ms', active: false },
              ]"
              :key="lane.model"
              class="flex items-center justify-between rounded-control border px-4 py-3 text-xs transition-colors"
              :class="
                lane.active
                  ? 'border-white/80 bg-white/75 text-gray-900 shadow-sm dark:border-white/[0.16] dark:bg-white/[0.09] dark:text-dark-50'
                  : 'border-black/[0.05] bg-white/30 text-gray-900/65 dark:border-white/[0.06] dark:bg-white/[0.02] dark:text-dark-50/60'
              "
            >
              <div class="flex items-center gap-3">
                <ProviderIcon :brand="lane.brand" size="16px" />
                <span class="font-mono font-medium">{{ lane.model }}</span>
              </div>
              <div class="flex items-center gap-3">
                <span class="font-mono tabular-nums text-gray-900/50 dark:text-dark-50/50">
                  {{ lane.latency }}
                </span>
                <span
                  class="rounded-full px-2.5 py-0.5 font-mono text-xs"
                  :class="
                    lane.active
                      ? 'bg-gray-900 text-gray-50 dark:bg-dark-50 dark:text-dark-950'
                      : 'bg-black/[0.05] text-gray-900/55 dark:bg-white/[0.07] dark:text-dark-50/55'
                  "
                >
                  {{ lane.active ? 'Primary' : 'Standby' }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Right 5 columns: airy gallery form workspace -->
      <section
        class="flex flex-col justify-between px-6 py-10 sm:px-10 lg:col-span-5 lg:px-14 lg:py-12"
      >
        <div
          class="glass-card mx-auto my-auto w-full max-w-md p-8 sm:p-10 lg:border-0 lg:bg-transparent lg:p-0 lg:shadow-none lg:backdrop-blur-none dark:lg:bg-transparent"
        >
          <!-- Mobile/tablet brand lockup -->
          <div class="mb-7 flex items-center gap-2.5 lg:hidden">
            <span
              class="flex h-8 w-8 items-center justify-center overflow-hidden rounded-control border border-white/80 bg-white/80 p-1 shadow-sm dark:border-white/[0.12] dark:bg-white/[0.08]"
            >
              <img
                :src="siteLogo || '/logo.svg'"
                :alt="siteName"
                class="h-full w-full object-contain"
              />
            </span>
            <span class="text-base font-medium text-gray-900 dark:text-dark-50">{{
              siteName
            }}</span>
          </div>

          <slot />

          <div class="mt-8 space-y-3 border-t border-black/[0.07] pt-5 dark:border-white/[0.08]">
            <div class="text-left text-sm">
              <slot name="footer" />
            </div>
            <div class="text-left text-xs text-gray-900/40 dark:text-dark-50/40">
              &copy; {{ currentYear }} {{ siteName }}. {{ t('common.rightsReserved') }}
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { vContentReveal } from '@/directives/contentReveal'
import { useRoute as useMotionRoute } from 'vue-router'
import { computed, defineAsyncComponent, onMounted } from 'vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import LazyVisual from '@/components/common/LazyVisual.vue'

// Topography loads asynchronously so authentication forms paint immediately.
const Topography = defineAsyncComponent(() => import('@/components/common/Topography.vue'))
import { useTheme } from '@/composables/useTheme'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

// AuthLayout.vue frames authentication forms with site branding, a minimal top bar,
// a background, and footer slots. Public settings determine the displayed name,
// sanitized logo, and optional subtitle override, while mounting requests settings
// so custom branding replaces the default fallback when available.

// t supplies authentication layout translations using the installed application locale;
// form content and footer links arrive through the component slots.
const { t } = useI18n()

// motionRoute supplies the path used to reveal authentication content again when
// navigation changes between pages that share this layout.
const motionRoute = useMotionRoute()

// appStore supplies cached public branding and its loading status, while useTheme
// powers the floating header theme toggle on public auth pages.
const appStore = useAppStore()
const { isDark, themePalette, toggleTheme } = useTheme()

// topographyColor picks muted cartographic ink tones without neon saturation.
const topographyColor = computed(() => {
  const palette = themePalette.value
  if (isDark.value) {
    if (palette === 'indigo') return '#7682a4'
    return '#827e74'
  }
  if (palette === 'indigo') return '#64748b'
  return '#87867f'
})

// Branding values use configured public settings; the logo passes URL sanitization and
// the subtitle honors an explicit empty override when one is present.
const siteName = computed(() => appStore.siteName || 'TokenRouter')
const siteLogo = computed(() =>
  sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }),
)
const siteSubtitle = computed(() =>
  appStore.cachedPublicSettings?.site_text_overrides?.includes('site_subtitle')
    ? appStore.cachedPublicSettings.site_subtitle || ''
    : t('home.heroDescription'),
)

// currentYear evaluates the local calendar year when the computed value is first read
// for the copyright line; it has no reactive clock source.
const currentYear = computed(() => new Date().getFullYear())

// Request public settings on mount so the branding block can become visible after the
// store completes its configured loading process.
onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
