import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { PluginUI } from '@/api/plugins.types'
import PluginAccountView from '../PluginAccountView.vue'
import { accountPages, accountPageForRoute } from '@/utils/pluginAccountPages'

// PluginAccountView.spec.ts verifies combined account tabs and legacy destinations.
// Installed pages are bounded discovery fixtures, and content hosts are editable
// inputs so tests observe retained drafts without submitting identity information
// or contacting binding services. Administrator-only pages remain outside the panels.

// Discovery fixtures control visibility and failure independently of installed bundles;
// route changes update the same mounted account page during tab retention tests.
const fixture = vi.hoisted(() => ({ ensure: vi.fn() }))
const route = reactive({ path: '/account/verification', query: {} as { tab?: string } })
const store = reactive({ plugins: [] as PluginUI[], ensureUi: fixture.ensure })
vi.mock('vue-router', () => ({ useRoute: () => route }))
vi.mock('vue-i18n', async (original) => ({
  ...(await original<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/stores/plugins', () => ({ usePluginsStore: () => store }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 7, role: 'user' } }) }))

// installed describes a plugin with only the supplied user-page contribution paths.
function installed(id: string, paths: string[]): PluginUI {
  return {
    id,
    version: '1.0.0',
    name: id,
    script: { url: '/fixture.js', integrity: 'fixture' },
    styles: [],
    menus: [],
    pages: paths.map((path) => ({ id: path || 'home', path, audience: 'user', title: path })),
  }
}

// accountView replaces only external form content while retaining real tab lifetimes.
function accountView() {
  return mount(PluginAccountView, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        RouterLink: { template: '<a><slot /></a>' },
        PluginPageContent: {
          props: ['subpath'],
          template: '<input :aria-label="subpath || \'binding\'" />',
        },
      },
    },
  })
}

beforeEach(() => {
  vi.resetAllMocks()
  route.path = '/account/verification'
  route.query = {}
  store.plugins = [installed('helstare.partner', ['verify/identity', 'verify/enterprise'])]
  fixture.ensure.mockResolvedValue(store.plugins)
})

describe('combined account pages', () => {
  // A user switches verification type after editing a personal form draft.
  // Both original forms remain mounted and the draft survives returning to its tab.
  it('retains draft inputs when switching verification tabs', async () => {
    const view = accountView()
    await flushPromises()
    await view.get('input[aria-label="verify/identity"]').setValue('local draft')
    route.query = { tab: 'enterprise' }
    await flushPromises()
    expect(view.get('input[aria-label="verify/enterprise"]').isVisible()).toBe(true)
    expect(view.get('input[aria-label="verify/identity"]').isVisible()).toBe(false)
    route.query = { tab: 'identity' }
    await flushPromises()
    expect(
      (view.get('input[aria-label="verify/identity"]').element as HTMLInputElement).value,
    ).toBe('local draft')
    view.unmount()
  })

  // Only one binding plugin is installed and the requested tab references another.
  // The page exposes the available binding without inventing an unavailable TG panel.
  it('filters missing plugins and administrator-only contributions', async () => {
    route.path = '/account/bindings'
    route.query = { tab: 'tg' }
    const admin = installed('helstare.tgbot', [''])
    admin.pages[0]!.audience = 'admin'
    store.plugins = [installed('helstare.qqbot', ['']), admin]
    const view = accountView()
    await flushPromises()
    expect(view.findAll('input')).toHaveLength(1)
    expect(view.text()).toContain('plugins.qqBinding')
    expect(view.text()).not.toContain('plugins.tgBinding')
    view.unmount()
  })

  // Discovery fails before any plugin form becomes available to the page.
  // An explicit retry recovers the authorized panels without leaving a blank screen.
  it('recovers from discovery failure', async () => {
    fixture.ensure.mockRejectedValueOnce(new Error('offline'))
    const view = accountView()
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('plugins.loadFailed')
    await view.get('button').trigger('click')
    await flushPromises()
    expect(view.findAll('input')).toHaveLength(2)
    view.unmount()
  })

  // Saved legacy URLs must select the matching panel in each combined destination.
  // Administrator plugin URLs and unrelated partner pages keep their existing routing.
  it('maps legacy user URLs to their account destinations', () => {
    for (const page of accountPages) {
      for (const panel of page.panels) {
        const legacy = `/ext/${panel.pluginId}${panel.subpath ? `/${panel.subpath}` : ''}`
        expect(accountPageForRoute(legacy)).toEqual({ page, panel })
      }
    }
    expect(accountPageForRoute('/admin/ext/helstare.qqbot')).toBeUndefined()
    expect(accountPageForRoute('/ext/helstare.partner')).toBeUndefined()
  })
})
