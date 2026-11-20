<script setup lang="ts">
import { RouterView, useRouter, useRoute } from 'vue-router'
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import Toast from '@/components/common/Toast.vue'
import NavigationProgress from '@/components/common/NavigationProgress.vue'
import { resolveRouteDocumentTitle } from '@/router/title'
import { defineLazyComponent } from '@/components/common/asyncComponent'
import { globalSearchOpen } from '@/components/layout/search/globalSearchState'
import {
  useAppStore,
  useAuthStore,
  useSubscriptionStore,
  useAnnouncementStore,
  useAdminSettingsStore,
} from '@/stores'
import { getSetupStatus } from '@/api/setup'
import { sanitizeUrl } from '@/utils/url'
import { updateFavicon } from '@/utils/branding'
import { usePluginsStore } from '@/stores/plugins'

// App.vue renders the routed page and shared progress, toast, announcement, and global
// search surfaces. Search downloads on its first opening and remains mounted so a
// later opening retains its query. Announcement code waits for an actual queued popup;
// the loaded instance remains available through closing transitions and queue changes.
// It updates branding and document titles from reactive configuration, and refreshes
// shared translated data when the locale changes. Announcement popups stay hidden on the
// root destination and Home page.
//
// Authentication changes start subscription polling and announcement reads, while logout
// clears their local state and removes the visibility listener. A restored session reads
// announcements immediately; a fresh login schedules a delayed forced read. The delay is
// cancelled on logout and unmount. Mounting checks installation status before loading
// public settings, and unmounting removes the window and document listeners installed by
// this shell.

// The root shell shares router and store instances across page changes so global
// branding, notifications, and session-driven refreshes follow application state.
const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()
const announcementStore = useAnnouncementStore()
const adminSettingsStore = useAdminSettingsStore()
const pluginsStore = usePluginsStore()

// Optional surfaces share loading feedback and a recoverable chunk-failure notice.
// Vue starts their imports only when their first-use gates mount the wrappers.
const GlobalSearchPalette = defineLazyComponent(
  () => import('@/components/layout/search/GlobalSearchPalette.vue'),
  () => globalSearchOpen.value,
)
const AnnouncementPopup = defineLazyComponent(
  () => import('@/components/announcements/AnnouncementPopup.vue'),
  () => shouldShowAnnouncementPopup.value && !!announcementStore.currentPopup,
)

// First-use flags retain component state and leave-transition ownership after loading.
// Delayed announcement reads belong to this shell and are cancelled before logout.
const searchActivated = ref(false)
const popupActivated = ref(false)
let announcementTimer: ReturnType<typeof setTimeout> | null = null

watch(
  globalSearchOpen,
  (open) => {
    if (open) searchActivated.value = true
  },
  { immediate: true },
)
watch(
  () => announcementStore.currentPopup,
  (popup) => {
    if (popup) popupActivated.value = true
  },
  { immediate: true },
)

// shouldShowAnnouncementPopup gates the global popup surface to console destinations
// while leaving the root and named Home page without popup rendering.
const shouldShowAnnouncementPopup = computed(() => route.path !== '/' && route.name !== 'Home')

// onLocaleChanged refreshes public settings after a language-change event and then
// refreshes authenticated shared data. Announcement and subscription requests settle
// independently before the document title is recalculated; component drafts remain owned
// by their mounted pages.
async function onLocaleChanged() {
  await appStore.fetchPublicSettings(true)
  if (authStore.isAuthenticated) {
    await Promise.allSettled([
      announcementStore.fetchAnnouncements(true),
      subscriptionStore.fetchActiveSubscriptions(true),
    ])
  }
  updateDocumentTitle()
}

// updateDocumentTitle applies route, site, custom-menu, and accepted plugin page titles
// to the browser tab. Discovery updates preserve the selected audience when descriptions
// arrive after navigation or another branding refresh completes.
function updateDocumentTitle() {
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  document.title = resolveRouteDocumentTitle(
    route,
    appStore.siteName,
    customMenuItems,
    pluginsStore.plugins,
  )
}

watch(
  () => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }),
  (newLogo) => {
    if (newLogo) {
      updateFavicon(newLogo)
    }
  },
  { immediate: true },
)

watch(
  [
    () => route.fullPath,
    () => route.meta.title,
    () => route.meta.titleKey,
    () => appStore.siteName,
    () => appStore.cachedPublicSettings?.custom_menu_items,
    () => authStore.isAdmin,
    () => adminSettingsStore.customMenuItems,
    () => pluginsStore.plugins,
  ],
  updateDocumentTitle,
  { deep: true },
)

// onVisibilityChange requests announcements when the authenticated document becomes
// visible again. The announcement store controls throttling so repeated visibility
// events reuse its existing fetch-admission window instead of forcing reads.
function onVisibilityChange() {
  if (document.visibilityState === 'visible' && authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
}

watch(
  () => authStore.isAuthenticated,
  (isAuthenticated, oldValue) => {
    if (announcementTimer !== null) {
      clearTimeout(announcementTimer)
      announcementTimer = null
    }
    if (isAuthenticated) {
      // Authentication starts subscription loading and polling; a failed initial read is
      // logged while the periodic poller can perform later refreshes.
      subscriptionStore.fetchActiveSubscriptions().catch((error) => {
        console.error('Failed to preload subscriptions:', error)
      })
      subscriptionStore.startPolling()
      if (oldValue === false) {
        // A fresh sign-in delays forced announcement loading by three seconds; restored
        // sessions use the ordinary fetch path immediately during shell setup.
        announcementTimer = setTimeout(() => {
          announcementTimer = null
          announcementStore.fetchAnnouncements(true)
        }, 3000)
      } else {
        announcementStore.fetchAnnouncements()
      }
      document.addEventListener('visibilitychange', onVisibilityChange)
    } else {
      // Logout invalidates subscription publication and clears announcements before
      // removing the visibility listener, ending the shell's authenticated refresh
      // triggers.
      subscriptionStore.clear()
      announcementStore.reset()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  },
  { immediate: true },
)

// removeNavigationListener releases this shell's announcement refresh callback on unmount.
const removeNavigationListener = router.afterEach(() => {
  if (authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
})

onBeforeUnmount(() => {
  // Unmount releases the shell-owned locale and visibility listeners so later browser
  // events cannot invoke callbacks attached by this component instance.
  window.removeEventListener('locale-changed', onLocaleChanged)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  removeNavigationListener?.()
  if (announcementTimer !== null) clearTimeout(announcementTimer)
})

onMounted(async () => {
  window.addEventListener('locale-changed', onLocaleChanged)
  try {
    const status = await getSetupStatus()
    if (status.needs_setup && route.path !== '/setup') {
      router.replace('/setup')
      return
    }
  } catch {
    // Setup status is unknown after a failed request; continue loading public
    // settings so the normal shell remains usable during that endpoint failure.
  }
  await appStore.fetchPublicSettings()
  updateDocumentTitle()
})
</script>

<template>
  <NavigationProgress />
  <RouterView />
  <GlobalSearchPalette v-if="searchActivated" />
  <Toast />
  <AnnouncementPopup v-if="shouldShowAnnouncementPopup && popupActivated" />
</template>
