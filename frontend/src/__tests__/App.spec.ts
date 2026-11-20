import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'

import App from '@/App.vue'
import { closeGlobalSearch, openGlobalSearch } from '@/components/layout/search/globalSearchState'

// App.spec.ts verifies announcement popup visibility in the shared application shell
// across public and console destinations, including first-use loading of search.
// Route and store doubles isolate optional surface mounting
// from authentication requests. The actual shell runs its mount lifecycle before
// assertions compare public landing destinations with the dashboard.

// mocks supplies route and store state before module evaluation; each case selects a
// destination while asynchronous startup requests resolve through controlled spies.
const mocks = vi.hoisted(() => ({
  route: {
    path: '/home',
    fullPath: '/home',
    name: 'Home' as string | undefined,
    meta: {} as Record<string, unknown>,
  },
  router: {
    afterEach: vi.fn(),
    replace: vi.fn(),
  },
  appStore: {
    cachedPublicSettings: null,
    siteLogo: '',
    siteName: 'Test Site',
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: true,
    isAdmin: false,
  },
  subscriptionStore: {
    fetchActiveSubscriptions: vi.fn(),
    startPolling: vi.fn(),
    clear: vi.fn(),
  },
  announcementStore: {
    currentPopup: null as { id: number } | null,
    fetchAnnouncements: vi.fn(),
    reset: vi.fn(),
  },
  adminSettingsStore: {
    customMenuItems: [] as unknown[],
  },
  getSetupStatus: vi.fn(),
}))

// announcementState publishes queue changes to the shell without real account reads.
const announcementState = reactive(mocks.announcementStore)

vi.mock('vue-router', () => ({
  RouterView: { template: '<div data-testid="router-view"></div>' },
  useRoute: () => mocks.route,
  useRouter: () => mocks.router,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => mocks.appStore,
  useAuthStore: () => mocks.authStore,
  useSubscriptionStore: () => mocks.subscriptionStore,
  useAnnouncementStore: () => announcementState,
  useAdminSettingsStore: () => mocks.adminSettingsStore,
}))

vi.mock('@/stores/plugins', () => ({ usePluginsStore: () => ({ plugins: [] }) }))

vi.mock('@/api/setup', () => ({
  getSetupStatus: (...args: unknown[]) => mocks.getSetupStatus(...args),
}))

vi.mock('@/router/title', () => ({
  resolveRouteDocumentTitle: () => 'Test Site',
}))

vi.mock('@/components/announcements/AnnouncementPopup.vue', () => ({
  __esModule: true,
  default: {
    name: 'AnnouncementPopup',
    template: '<div data-testid="announcement-popup"></div>',
  },
}))

// mountApp selects a mocked route and mounts the actual application shell. The global
// search palette is stubbed because these cases cover shell surfaces and install no i18n.
// The caller awaits startup promises and owns unmounting the returned wrapper so shell
// event listeners are released.
function mountApp(path: string, name?: string) {
  mocks.route.path = path
  mocks.route.fullPath = path
  mocks.route.name = name

  return mount(App, {
    global: {
      stubs: {
        NavigationProgress: true,
        Toast: true,
        GlobalSearchPalette: true,
      },
    },
  })
}

describe('App announcement popup visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.authStore.isAuthenticated = true
    announcementState.currentPopup = null
    closeGlobalSearch()
    mocks.getSetupStatus.mockResolvedValue({ needs_setup: false })
    mocks.appStore.fetchPublicSettings.mockResolvedValue(undefined)
    mocks.subscriptionStore.fetchActiveSubscriptions.mockResolvedValue(undefined)
  })

  afterEach(() => {
    document.body.style.overflow = ''
    closeGlobalSearch()
  })

  // An authenticated shell mounts at the root or named Home destination. The global
  // announcement popup stays absent on both public-facing destinations after
  // asynchronous mounting work settles.
  it.each([
    ['/', undefined],
    ['/home', 'Home'],
  ])('已登录用户访问门面路由 %s 时不挂载公告弹窗', async (path, name) => {
    const wrapper = mountApp(path, name)
    await flushPromises()

    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(false)

    wrapper.unmount()
  })

  // An empty console queue leaves announcement code unused. Its first entry mounts
  // the popup, and clearing that entry retains the instance for its closing transition.
  it('loads the first queued popup and retains its closing instance', async () => {
    const wrapper = mountApp('/dashboard', 'Dashboard')
    await flushPromises()

    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(false)
    announcementState.currentPopup = { id: 1 }
    await flushPromises()
    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(true)
    announcementState.currentPopup = null
    await flushPromises()
    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(true)

    wrapper.unmount()
  })

  // Search has no mounted panel until first use. Once opened, its instance survives
  // closing so the actual palette can retain its query between openings.
  it('mounts search on first use and keeps it after closing', async () => {
    const wrapper = mountApp('/home', 'Home')
    await flushPromises()
    expect(wrapper.find('global-search-palette-stub').exists()).toBe(false)

    openGlobalSearch()
    await flushPromises()
    expect(wrapper.find('global-search-palette-stub').exists()).toBe(true)
    closeGlobalSearch()
    await flushPromises()
    expect(wrapper.find('global-search-palette-stub').exists()).toBe(true)

    wrapper.unmount()
  })
})
