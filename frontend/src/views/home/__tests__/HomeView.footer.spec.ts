import { enableAutoUnmount, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createI18n } from 'vue-i18n'

import type { PublicSettings } from '@/api/settings.types'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import HomeView from '@/views/HomeView.vue'

// HomeView.footer.spec.ts checks public footer destinations with the shipped
// language catalogs. Store and artwork substitutes isolate navigation from API
// requests and WebGL, while configured groups, legal summaries, and registration
// switches exercise the same rendering rules used by the homepage.

// state supplies published settings and the account state selected before mounting.
const state = vi.hoisted(() => ({
  settings: {} as Partial<PublicSettings>, // Public settings fixture.
  authenticated: false, // Account authentication fixture.
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: state.settings,
    publicSettingsLoaded: true,
    siteName: 'Test gateway',
    siteLogo: '',
    docUrl: '',
  }),
  useAuthStore: () => ({
    isAuthenticated: state.authenticated,
    isAdmin: false,
    checkAuth: vi.fn(),
  }),
}))
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ path: '/home' }),
}))
vi.mock('@/composables/useTheme', () => ({
  useTheme: () => ({ isDark: ref(false), themePalette: ref('warm') }),
}))
vi.mock('@/composables/useLocaleRefresh', () => ({ useLocaleRefresh: vi.fn() }))
vi.mock('@/directives/contentReveal', () => ({ vContentReveal: {} }))
vi.mock('@/components/layout/AppHeader.vue', () => ({ default: { template: '<header />' } }))
vi.mock('@/components/common/DitherVeil.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/FallingText.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/MicroSlats.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/marketplace', () => ({
  getMarketplaceModels: vi.fn().mockResolvedValue([]),
  getMarketplaceStats: vi.fn().mockResolvedValue({}),
}))

enableAutoUnmount(afterEach)
beforeEach(() => {
  state.settings = {}
  state.authenticated = false
})

// mountHome renders the real footer with one shipped catalog and stubbed artwork.
// Automatic unmount releases the homepage's icon timer after each navigation fixture.
function mountHome(locale: 'en' | 'zh' = 'en') {
  return shallowMount(HomeView, {
    global: {
      plugins: [createI18n({ legacy: false, locale, messages: { en, zh } })],
      renderStubDefaultSlot: true,
      stubs: {
        RouterLink: true,
        DitherVeil: true,
        FallingText: true,
        MicroSlats: true,
      },
    },
  })
}

describe('Homepage footer', () => {
  // A guest opens the homepage before optional footer settings are configured.
  // Both catalogs must expose working public destinations and only built-in legal document IDs.
  it.each(['en', 'zh'] as const)('renders localized default navigation in %s', (locale) => {
    const footer = mountHome(locale).get('footer')
    const paths = footer.findAll('router-link-stub').map((link) => link.attributes('to'))

    expect(paths).toEqual([
      '/home',
      '/models',
      '/key-usage',
      '/login',
      '/register',
      '/legal/terms',
      '/legal/usage-policy',
      '/legal/supported-regions',
      '/legal/service-specific-terms',
    ])
    expect(footer.text()).toContain(locale === 'en' ? 'Models & pricing' : '模型与定价')
    expect(footer.text()).not.toContain('home.footer.')
    expect(footer.text()).not.toContain('Powered by')
  })

  // An operator disables account registration without closing the public homepage.
  // Guests retain sign-in access, and the footer must stop advertising an unavailable signup flow.
  it('hides signup when public registration is disabled', () => {
    state.settings = { registration_enabled: false }
    const footer = mountHome().get('footer')

    expect(footer.find('[to="/register"]').exists()).toBe(false)
    expect(footer.find('[to="/login"]').exists()).toBe(true)
  })

  // A signed-in account already has access to its dashboard and API keys.
  // Account navigation must use those destinations instead of repeating guest authentication actions.
  it('shows account destinations for authenticated visitors', () => {
    state.authenticated = true
    const footer = mountHome().get('footer')

    expect(footer.find('[to="/dashboard"]').exists()).toBe(true)
    expect(footer.find('[to="/keys"]').exists()).toBe(true)
    expect(footer.find('[to="/register"]').exists()).toBe(false)
    expect(footer.find('[to="/login"]').exists()).toBe(false)
  })

  // Published legal documents replace built-in summaries, including a custom privacy policy.
  // The footer must preserve their titles and avoid linking documents absent from that published list.
  it('links only the published legal documents', () => {
    state.settings = {
      login_agreement_documents: [{ id: 'privacy', title: 'Privacy policy', content_md: '' }],
    }
    const footer = mountHome().get('footer')

    expect(footer.get('[to="/legal/privacy"]').text()).toBe('Privacy policy')
    expect(footer.find('[to="/legal/terms"]').exists()).toBe(false)
  })

  // An administrator supplies custom navigation groups and additional footer text.
  // Those groups replace every default column while preserving external-link isolation and the configured text.
  it('keeps configured navigation and footer text ahead of defaults', () => {
    state.settings = {
      footer_links: [
        {
          title: 'Resources',
          links: [{ label: 'Integration guide', url: 'https://docs.example.test' }],
        },
      ],
      footer_text: 'Operator details\n\nRegistration information',
    }
    const footer = mountHome().get('footer')
    const guide = footer.get('a[href="https://docs.example.test"]')

    expect(guide.attributes('target')).toBe('_blank')
    expect(guide.attributes('rel')).toBe('noopener noreferrer')
    expect(footer.find('[to="/models"]').exists()).toBe(false)
    expect(footer.text()).toContain('Operator details')
    expect(footer.text()).toContain('Registration information')
  })

  // A deployment advertises its own API documentation through public settings.
  // Default exploration links retain that URL without adding an unconfigured documentation destination.
  it('includes the configured documentation destination', () => {
    state.settings = { doc_url: 'https://docs.example.test' }
    const footer = mountHome().get('footer')

    expect(footer.get('a[href="https://docs.example.test/"]').text()).toBe('Docs')
  })
})
