import { describe, expect, it, vi } from 'vitest'
import type { PluginUI } from '@/api/plugins.types'
import { useNavigationItems } from '../navigationItems'

// navigationItems.spec.ts verifies the account destinations shared by sidebar and search.
// Plugin descriptions represent existing user and administrator contributions so
// grouping tests preserve administration links while removing duplicate personal
// destinations. No installed scripts or authenticated API requests execute here.

// Store doubles expose accepted menu declarations without loading private settings
// or network state; the composable still builds its real role-specific destination lists.
const fixture = vi.hoisted(() => ({ plugins: [] as PluginUI[] }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ cachedPublicSettings: {} }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))
vi.mock('@/stores/plugins', () => ({ usePluginsStore: () => fixture }))

// contribution returns user and administrator links for one installed plugin page.
function contribution(id: string, paths: string[]): PluginUI {
  return {
    id,
    version: '1.0.0',
    name: id,
    script: { url: '/fixture.js', integrity: 'fixture' },
    styles: [],
    pages: [],
    menus: paths.flatMap((path) => [
      {
        page: path || 'home',
        audience: 'user' as const,
        label: path,
        icon_svg: '',
        order: 1,
        route: `/ext/${id}${path ? `/${path}` : ''}`,
      },
      {
        page: path || 'admin',
        audience: 'admin' as const,
        label: path,
        icon_svg: '',
        order: 1,
        route: `/admin/ext/${id}${path ? `/${path}` : ''}`,
      },
    ]),
  }
}

describe('account plugin navigation', () => {
  // Related personal contributions arrive as separate installed plugin menu links.
  // Sidebar and search share one destination per group, retaining every administrator link.
  it('groups personal links while preserving administrator menus', () => {
    fixture.plugins = [
      contribution('helstare.partner', ['verify/identity', 'verify/enterprise']),
      contribution('helstare.qqbot', ['']),
      contribution('helstare.tgbot', ['']),
    ]
    const { accountNavItems, adminNavItems } = useNavigationItems()
    const paths = accountNavItems.value.map((entry) => entry.path)
    expect(paths.filter((path) => path === '/account/verification')).toHaveLength(1)
    expect(paths.filter((path) => path === '/account/bindings')).toHaveLength(1)
    expect(paths).not.toContain('/model-quality')
    expect(paths.some((path) => path.startsWith('/ext/helstare.qqbot'))).toBe(false)
    expect(
      adminNavItems.value.filter((entry) => entry.path.startsWith('/admin/ext/')),
    ).toHaveLength(4)
  })
})
