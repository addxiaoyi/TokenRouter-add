<template>
  <div class="ba-theme-shell" :class="shellClass">
    <!-- Skip link lets keyboard users jump directly past fixed header and sidebar chrome. -->
    <a
      href="#main-content"
      class="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-tooltip focus:rounded-control focus:bg-[#141413] focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-[#faf9f5] focus:shadow-lg dark:focus:bg-[#faf9f5] dark:focus:text-[#141413]"
    >
      Skip to content
    </a>

    <!-- Non-home architectural topography backdrop rendered with zero bloom/neon glow. -->
    <div class="pointer-events-none fixed inset-0 overflow-hidden bg-[var(--page-bg)]">
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
        :speed="0"
        direction="forward"
        :morph="0"
        :glow="0"
        :depth-of-field="0"
        :fog="0.4"
        :brightness="isDarkTheme ? 0.85 : 0.95"
        :opacity="isDarkTheme ? 0.36 : 0.32"
        :mouse-interaction="true"
        hover-effect="lift"
        :mouse-radius="1.2"
        :mouse-strength="0.85"
        :trail="0.45"
        :parallax="0.65"
        click-effect="pulse"
        :intro="false"
        :light-mode="!isDarkTheme"
        :dpr="1"
        :max-fps="24"
        :max-cells="24000"
        :ignore-reduced-motion="true"
      />
    </div>

    <!--
      In console mode the contextual header spans the main column while the full-height
      Codex sidebar owns brand and account controls.
    -->
    <AppHeader :console-mode="!hideSidebar" />

    <!-- Sidebar and Main Content Area -->
    <AppSidebar v-if="!hideSidebar" />

    <div
      class="relative z-10 flex min-w-0 flex-col pt-[var(--header-h)] transition-[margin-left] duration-layout"
      :class="[
        columnClass,
        hideSidebar
          ? ''
          : sidebarCollapsed
            ? 'lg:ml-[var(--sidebar-w-collapsed)]'
            : 'lg:ml-[var(--sidebar-w)]',
      ]"
    >
      <!-- The main flex column allocates available space to the slotted page. -->
      <main
        id="main-content"
        tabindex="-1"
        v-content-reveal="route.path"
        class="app-main flex min-w-0 flex-1 flex-col outline-none"
        :class="mainClass"
      >
        <!--
          Actions align with the bottom of the heading block and wrap when horizontal
          space is insufficient.
        -->
        <div
          v-if="pageTitle && !hidePageHeading"
          class="page-heading mb-4 flex flex-shrink-0 flex-wrap items-end justify-between gap-3"
        >
          <div>
            <h1 class="page-title">{{ pageTitle }}</h1>
            <p v-if="pageDescription" class="page-description">{{ pageDescription }}</p>
            <!--
              Optional metadata appears below the description, such as dashboard live
              status.
            -->
            <div v-if="$slots['page-heading-meta']" class="mt-2">
              <slot name="page-heading-meta" />
            </div>
          </div>
          <div v-if="$slots['page-heading-actions']" class="shrink-0">
            <slot name="page-heading-actions" />
          </div>
        </div>
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { vContentReveal } from '@/directives/contentReveal'

import '@/styles/onboarding.css'
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/onboarding/useOnboardingTour'
import { usePageMeta } from '@/composables/usePageMeta'
import { useTheme } from '@/composables/useTheme'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

// Topography loads asynchronously so console chrome paints immediately without waiting for WebGL shaders.
const Topography = defineAsyncComponent(() => import('@/components/common/Topography.vue'))

// AppLayout.vue allocates space for the global header, optional sidebar, page heading,
// and slotted content. Its viewport modes choose whether the shell follows document
// height or supplies a fixed height through the flex chain. Full viewport mode also
// removes main padding and locks document scrolling during the mounted component
// lifetime.
//
// The scroll lock saves the document element and body inline overflow values separately.
// Unmount restores each saved value, including an empty string, so the next page can
// resume its preceding scrolling rules. Height selection remains reactive to props,
// while document locking runs at mount and cleanup runs at unmount.

// @project-doc docs/architecture/frontend_ui_conventions.md#layout_spacing

// Props selects viewport sizing and the shared page heading visibility.
interface Props {
  // fullViewport fixes the workspace to the dynamic viewport and locks document
  // scrolling when mounted with this flag enabled.
  fullViewport?: boolean
  // fitViewport keeps heading and padding; true locks height at lg, while all supplies
  // viewport height at every screen size.
  fitViewport?: boolean | 'all'
  // hidePageHeading suppresses the layout heading when the slotted page supplies its own
  // title and description.
  hidePageHeading?: boolean
}

// props defaults to document-flow layout with the shared heading visible, leaving
// viewport locking as a choice for each consuming page.
const props = withDefaults(defineProps<Props>(), {
  fullViewport: false,
  fitViewport: false,
  hidePageHeading: false,
})

// Application, authentication, and route state determine sidebar spacing, page metadata,
// and which onboarding guide is selected during component setup.
const appStore = useAppStore()
const authStore = useAuthStore()
const route = useRoute()
const { isDark: isDarkTheme, themePalette } = useTheme()
// topographyColor picks muted cartographic ink tones without neon saturation.
const topographyColor = computed(() => {
  const palette = themePalette.value
  if (isDarkTheme.value) {
    if (palette === 'indigo') return '#7682a4'
    return '#827e74'
  }
  if (palette === 'indigo') return '#64748b'
  return '#87867f'
})
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
// hideSidebar reads route metadata so workspace pages can hide navigation and remove the
// corresponding main-column offset.
const hideSidebar = computed(() => route.meta.hideSidebar === true)
const fullViewport = computed(() => props.fullViewport)

// shellClass supplies an explicit height in locked modes so descendant percentage
// heights and internal scrolling can resolve through the flex chain.
const shellClass = computed(() => {
  if (fullViewport.value) return 'fixed inset-0 h-[100dvh] overflow-hidden'
  if (props.fitViewport === 'all') return 'h-[100dvh] min-h-0 overflow-hidden'
  if (props.fitViewport) return 'min-h-screen lg:h-[100dvh] lg:min-h-0 lg:overflow-hidden'
  return 'min-h-screen'
})
// columnClass carries the shell height into the content column, retaining document-flow
// sizing below lg when desktop-only fitting is selected.
const columnClass = computed(() => {
  if (fullViewport.value || props.fitViewport === 'all') return 'h-full min-h-0'
  if (props.fitViewport) return 'min-h-screen lg:h-full lg:min-h-0'
  return 'min-h-screen'
})
// mainClass preserves standard page padding for fitted modes and removes it for full
// workspaces; minimum height lets child scroll areas shrink.
const mainClass = computed(() => {
  if (fullViewport.value) return 'min-h-0 p-0'
  const padding = 'px-4 pb-4 pt-4 md:px-6 md:pb-6 lg:px-8 lg:pb-8'
  if (props.fitViewport === 'all') return `${padding} min-h-0`
  return props.fitViewport ? `${padding} lg:min-h-0` : padding
})
// isAdmin selects the onboarding storage key at setup, while saved overflow values below
// belong to this mounted layout instance.
const isAdmin = computed(() => authStore.user?.role === 'admin')

// previousHtmlOverflowY and previousBodyOverflowY hold separate pre-lock inline styles.
// Null means no saved value needs restoration; an empty string remains a meaningful
// saved style.
let previousHtmlOverflowY: string | null = null
let previousBodyOverflowY: string | null = null

// lockDocumentScroll saves root scrolling styles and hides document overflow. Call it
// once at mount when fullViewport is enabled; restoration depends on the saved values,
// so repeating the call would replace them with locked styles.
function lockDocumentScroll(): void {
  if (!fullViewport.value || typeof document === 'undefined') return
  previousHtmlOverflowY = document.documentElement.style.overflowY
  previousBodyOverflowY = document.body.style.overflowY
  document.documentElement.style.overflowY = 'hidden'
  document.body.style.overflowY = 'hidden'
}

// restoreDocumentScroll restores the inline overflow values captured by mounting. Each
// saved value is cleared after restoration, including saved empty strings, so repeated
// cleanup leaves document styles untouched after the first restore.
function restoreDocumentScroll(): void {
  if (typeof document === 'undefined') return
  // Restore each saved inline value before clearing its marker so an empty string can
  // correctly return control to stylesheet rules.
  if (previousHtmlOverflowY !== null) {
    document.documentElement.style.overflowY = previousHtmlOverflowY
    previousHtmlOverflowY = null
  }
  if (previousBodyOverflowY !== null) {
    document.body.style.overflowY = previousBodyOverflowY
    previousBodyOverflowY = null
  }
}

// Onboarding setup selects the role-specific storage key and enables automatic start;
// the store receives replay callbacks after the layout has mounted.
const { replayTour, startTeamTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true,
})

// The onboarding store publishes replay actions, while page metadata supplies the title
// and description rendered above the slotted page content.
const onboardingStore = useOnboardingStore()
const { pageTitle, pageDescription } = usePageMeta()

// Mount acquires the optional document scroll lock before publishing guide callbacks;
// unmount releases the captured document styles for subsequent pages.
onMounted(() => {
  lockDocumentScroll()
  onboardingStore.setReplayCallback(replayTour)
  onboardingStore.setTeamGuideCallback(startTeamTour)
})

onBeforeUnmount(() => {
  restoreDocumentScroll()
})

defineExpose({ replayTour })
</script>

<!--
  The flex chain allocates shell height to app-main, then the natural-height heading
  and slotted page. Pages that fill the remaining space provide flex-1 and min-h-0 on
  their own content container.
-->
