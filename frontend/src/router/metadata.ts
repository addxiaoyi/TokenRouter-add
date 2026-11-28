import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { CustomMenuItem, PublicSettings } from '@/api/settings.types'
import type { PluginUI } from '@/api/plugins.types'
import { getLocale, i18n } from '@/i18n'
import { resolvePageMetadata, type SocialMetadata } from '../../socialMetadata'
import { resolveRouteDocumentTitle } from './title'

// metadata.ts synchronizes browser head metadata with the displayed route,
// active language, and current public settings. The public page resolver is
// shared with Vite's HTML injection so initial documents and later SPA
// navigation follow the same canonical and indexing rules.
//
// Navigation, branding refreshes, and locale changes all enter through one
// operation. Each call replaces or removes every managed field, including
// server-injected tags, so a private page cannot retain a previous public
// page's canonical URL, sharing image, or structured-data graph. Private tab
// titles continue using custom-menu and accepted plugin labels.

/** MetadataContext supplies current branding and authorized labels for browser metadata. */
interface MetadataContext {
  settings: PublicSettings | null // Current public settings.
  siteName: string // Resolved branding name.
  customMenuItems?: CustomMenuItem[] // Authorized menu labels.
  plugins?: readonly PluginUI[] // Accepted plugin descriptions.
}

/** MetadataRoute contains the destination fields shared by guards and mounted pages. */
type MetadataRoute = Pick<RouteLocationNormalizedLoaded, 'path' | 'name' | 'params' | 'meta'>

/**
 * setMeta replaces one managed metadata value and removes duplicate elements.
 * Empty values remove the corresponding tag, allowing private routes to clear
 * sharing fields injected into the initial public document.
 */
function setMeta(attribute: 'name' | 'property', key: string, content: string): void {
  const matches = document.head.querySelectorAll<HTMLMetaElement>(`meta[${attribute}="${key}"]`)
  const current = matches[0]
  matches.forEach((entry, index) => {
    if (index > 0) entry.remove()
  })
  if (!content) {
    current?.remove()
    return
  }
  const element = current || document.createElement('meta')
  element.setAttribute(attribute, key)
  element.content = content
  if (!current) document.head.append(element)
}

/**
 * applyRouteMetadata updates the complete browser identity for one destination.
 * Call after language initialization with settings available to the caller;
 * later configuration reads can repeat the operation to publish localized copy.
 * Published pages receive canonical URLs and a public graph, while all other
 * routes clear public sharing fields and retain their ordinary translated tab
 * titles. Every managed tag is replaced together so earlier server-rendered
 * destinations cannot survive a route or locale change.
 */
export function applyRouteMetadata(route: MetadataRoute, context: MetadataContext): void {
  const settings: SocialMetadata = context.settings || { site_name: context.siteName }
  const page = resolvePageMetadata(
    settings,
    new URL(route.path, window.location.origin).href,
    getLocale(),
  )
  const routeDescription = route.meta.descriptionKey
    ? i18n.global.t(route.meta.descriptionKey)
    : typeof route.meta.description === 'string'
      ? route.meta.description
      : ''
  document.title = page.indexable
    ? page.title
    : resolveRouteDocumentTitle(route, context.siteName, context.customMenuItems, context.plugins)
  setMeta('name', 'description', page.indexable ? page.description : routeDescription)
  setMeta('name', 'robots', page.indexable ? 'index, follow' : 'noindex, nofollow')
  setMeta('property', 'og:type', page.indexable ? 'website' : '')
  setMeta('property', 'og:title', page.indexable ? page.shareTitle : '')
  setMeta('property', 'og:description', page.description)
  setMeta('property', 'og:site_name', page.indexable ? context.siteName : '')
  setMeta('property', 'og:url', page.canonical)
  setMeta('property', 'og:locale', page.indexable ? page.language.replace(/-/g, '_') : '')
  setMeta('property', 'og:image', page.image)
  setMeta('property', 'og:image:type', page.image ? settings.site_share_image_type || '' : '')
  setMeta(
    'property',
    'og:image:width',
    page.image && settings.site_share_image_width ? String(settings.site_share_image_width) : '',
  )
  setMeta(
    'property',
    'og:image:height',
    page.image && settings.site_share_image_height ? String(settings.site_share_image_height) : '',
  )
  setMeta(
    'name',
    'twitter:card',
    page.indexable ? (page.image ? 'summary_large_image' : 'summary') : '',
  )
  setMeta('name', 'twitter:title', page.indexable ? page.shareTitle : '')
  setMeta('name', 'twitter:description', page.description)
  setMeta('name', 'twitter:image', page.image)

  document.head.querySelectorAll('link[rel="canonical"]').forEach((entry) => entry.remove())
  if (page.canonical) {
    const canonical = document.createElement('link')
    canonical.rel = 'canonical'
    canonical.href = page.canonical
    document.head.append(canonical)
  }

  document.head.querySelectorAll('#site-structured-data').forEach((entry) => entry.remove())
  if (page.schema) {
    const schema = document.createElement('script')
    schema.id = 'site-structured-data'
    schema.type = 'application/ld+json'
    schema.textContent = JSON.stringify(page.schema)
    document.head.append(schema)
  }
}
