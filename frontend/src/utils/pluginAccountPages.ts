import type { PluginUI } from '@/api/plugins.types'

// pluginAccountPages.ts groups related personal plugin contributions into account pages.
// Each panel retains its installed plugin and exact path, while navigation points
// to one account destination. Discovery still controls which panels the user can
// open, so a missing or disabled plugin never gains visibility through grouping.

// AccountPanel identifies an existing user contribution within a combined page.
export interface AccountPanel {
  id: string // Stable account tab identifier.
  pluginId: string // Installed plugin identifier.
  subpath: string // Exact user contribution path.
  titleKey: string // Host-owned translated tab label.
}

// AccountPage defines one account destination and its supported plugin panels.
export interface AccountPage {
  path: string // Canonical account page route.
  titleKey: string // Host-owned navigation label.
  panels: readonly AccountPanel[] // Ordered supported user contributions.
}

// accountPages maps only these existing user contributions into combined destinations;
// administrator menus and unrelated pages retain their accepted manifest routes.
export const accountPages: readonly AccountPage[] = [
  {
    path: '/account/verification',
    titleKey: 'plugins.verification',
    panels: [
      {
        id: 'identity',
        pluginId: 'helstare.partner',
        subpath: 'verify/identity',
        titleKey: 'plugins.personalVerification',
      },
      {
        id: 'enterprise',
        pluginId: 'helstare.partner',
        subpath: 'verify/enterprise',
        titleKey: 'plugins.enterpriseVerification',
      },
    ],
  },
  {
    path: '/account/bindings',
    titleKey: 'plugins.bindings',
    panels: [
      { id: 'qq', pluginId: 'helstare.qqbot', subpath: '', titleKey: 'plugins.qqBinding' },
      { id: 'tg', pluginId: 'helstare.tgbot', subpath: '', titleKey: 'plugins.tgBinding' },
    ],
  },
]

// accountPageForRoute resolves legacy user links to their account page and tab.
// Unrelated contribution routes return undefined, allowing the ordinary plugin
// host to continue handling their original path and audience without redirection.
export function accountPageForRoute(
  route: string,
): { page: AccountPage; panel: AccountPanel } | undefined {
  for (const page of accountPages) {
    const panel = page.panels.find(
      (entry) => route === `/ext/${entry.pluginId}${entry.subpath ? `/${entry.subpath}` : ''}`,
    )
    if (panel) return { page, panel }
  }
  return undefined
}

// availableAccountPanels returns only exact contributions accepted for the current user.
// Menu declarations do not grant page access; installed pages must match both
// the user audience and the requested path before a panel becomes mountable.
export function availableAccountPanels(
  page: AccountPage,
  plugins: readonly PluginUI[],
): readonly AccountPanel[] {
  return page.panels.filter((panel) =>
    plugins.some(
      (plugin) =>
        plugin.id === panel.pluginId &&
        plugin.pages.some((entry) => entry.audience === 'user' && entry.path === panel.subpath),
    ),
  )
}
