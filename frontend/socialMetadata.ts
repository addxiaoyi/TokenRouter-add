import english from './src/i18n/locales/en/seo.ts'
import chinese from './src/i18n/locales/zh/seo.ts'

// socialMetadata.ts resolves public page identities and renders development HTML
// metadata from public settings. Browser navigation uses the same resolver so
// canonical links and sharing URLs identify the displayed destination. Account,
// callback, and plugin routes receive a noindex policy and carry no public graph.
//
// Published legal document identifiers determine which policy pages are public.
// Root and home paths share the root identity; pricing resolves to models.
// Configured frontend origins take precedence over request origins, and query
// strings stay outside canonical identities. Cover bytes remain on the backend
// image endpoint; only validated resource paths appear in sharing metadata.

/** SocialMetadata contains the optional public fields accepted from older and current servers. */
export interface SocialMetadata {
  locale?: string // Resolved content language.
  site_name?: string // Resolved site name.
  site_title?: string // Resolved homepage title.
  site_subtitle?: string // Resolved homepage subtitle.
  site_share_title?: string // Resolved sharing title.
  site_share_description?: string // Resolved sharing description.
  site_share_image_url?: string // Digest-addressed cover path.
  site_share_image_type?: string // Validated cover MIME type.
  site_share_image_width?: number // Cover width in pixels.
  site_share_image_height?: number // Cover height in pixels.
  frontend_url?: string // Configured frontend base URL.
  backend_mode_enabled?: boolean // Restricted public navigation switch.
  login_agreement_documents?: {
    id: string // Published document identifier.
    title: string // Localized document title.
  }[] // Published public policy summaries.
}

/**
 * PageMetadata describes one route's public identity and crawler visibility policy.
 * Private destinations keep canonical and image empty and schema null. The
 * indexable field identifies published paths independently of origin validity.
 */
export interface PageMetadata {
  title: string // Branded browser title.
  // shareTitle honors configured homepage sharing text while other pages keep their
  // own titles, preventing homepage copy from replacing a different public identity.
  shareTitle: string // Branded social card title.
  description: string // Public page summary.
  canonical: string // Public canonical URL.
  image: string // Absolute sharing image.
  indexable: boolean // Public indexing eligibility.
  language: string // Resolved document language.
  schema: Record<string, unknown> | null // Public structured-data graph.
}

/**
 * resolvePageMetadata selects a public page from its path and published settings.
 * requestURL must be absolute, and locale selects the supplied Chinese or English
 * summaries. Published legal identifiers determine policy visibility; root and
 * pricing aliases share their existing destination's canonical identity. Unknown
 * destinations retain an empty public identity. Invalid configured frontend
 * origins omit absolute URLs while preserving the route's title, summary, and
 * visibility. Query strings and fragments never participate in public identity.
 */
export function resolvePageMetadata(
  settings: SocialMetadata,
  requestURL: string,
  locale = settings.locale || 'en',
): PageMetadata {
  const copy = locale.toLowerCase().startsWith('zh') ? chinese : english
  const siteName = settings.site_name?.trim() || 'TokenRouter'
  const request = new URL(requestURL)
  const path = request.pathname.replace(/\/+$/, '') || '/'
  let canonicalPath = ''
  let pageTitle = ''
  let description = ''
  let schemaType = 'WebPage'
  const homeDescription =
    settings.site_share_description?.trim() ||
    settings.site_subtitle?.trim() ||
    copy.homeDescription

  if ((path === '/' || path === '/home') && !settings.backend_mode_enabled) {
    canonicalPath = '/'
    const configuredTitle = settings.site_title?.trim()
    // Branding alone does not describe the homepage, so supply the same localized gateway purpose used by initial server HTML.
    pageTitle =
      configuredTitle && configuredTitle.toLowerCase() !== siteName.toLowerCase()
        ? configuredTitle
        : `${siteName} | ${copy.homeTitle}`
    description = homeDescription
  } else if (path === '/models' || path === '/pricing') {
    canonicalPath = '/models'
    pageTitle = copy.modelsTitle
    description = copy.modelsDescription
    schemaType = 'CollectionPage'
  } else if (path === '/docs') {
    canonicalPath = '/docs'
    pageTitle = copy.docsTitle
    description = copy.docsDescription
  } else if (path === '/model-quality' && !settings.backend_mode_enabled) {
    canonicalPath = '/model-quality'
    pageTitle = copy.qualityTitle
    description = copy.qualityDescription
    schemaType = 'CollectionPage'
  } else {
    const legalDocument = settings.login_agreement_documents?.find(
      (entry) =>
        entry.id.trim() === entry.id &&
        entry.id !== '.' &&
        entry.id !== '..' &&
        !/[\\/?#]/.test(entry.id) &&
        path === `/legal/${encodeURIComponent(entry.id)}` &&
        entry.title.trim(),
    )
    if (legalDocument) {
      canonicalPath = `/legal/${encodeURIComponent(legalDocument.id)}`
      pageTitle = legalDocument.title.trim()
      description = copy.legalDescription
        .replace('{document}', pageTitle)
        .replace('{site}', siteName)
    }
  }

  let canonical = ''
  let image = ''
  let homeURL = ''
  try {
    const base = new URL(settings.frontend_url?.trim() || request.origin)
    if (['http:', 'https:'].includes(base.protocol) && !base.username && !base.password) {
      if (canonicalPath) {
        canonical = new URL(canonicalPath, base).href
        homeURL = new URL('/', base).href
        if (settings.site_share_image_url?.startsWith('/api/v1/settings/share-image/')) {
          image = new URL(settings.site_share_image_url, base).href
        }
      }
    }
  } catch {
    // Invalid configured origins leave text metadata usable while omitting URLs that would identify an unrelated page or host.
  }

  const title =
    canonicalPath === '/' ? pageTitle : pageTitle ? `${pageTitle} | ${siteName}` : siteName
  const configuredShareTitle = settings.site_share_title?.trim()
  const shareTitle =
    canonicalPath === '/' &&
    configuredShareTitle &&
    configuredShareTitle.toLowerCase() !== siteName.toLowerCase()
      ? configuredShareTitle
      : title
  return {
    title,
    shareTitle,
    description,
    canonical,
    image,
    indexable: Boolean(canonicalPath),
    language: locale,
    schema: canonical
      ? {
          '@context': 'https://schema.org',
          '@graph': [
            {
              '@type': 'WebSite',
              '@id': `${homeURL}#website`,
              url: homeURL,
              name: siteName,
              description: homeDescription,
              inLanguage: locale,
            },
            {
              '@type': schemaType,
              '@id': `${canonical}#webpage`,
              url: canonical,
              name: title,
              description,
              inLanguage: locale,
              isPartOf: { '@id': `${homeURL}#website` },
            },
          ],
        }
      : null,
  }
}

/** escapeAttribute encodes text and URL values for quoted HTML attributes. */
function escapeAttribute(value: string): string {
  return value.replace(
    /[&<>"']/g,
    (character) =>
      ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
      })[character] || character,
  )
}

/**
 * renderSocialMetadata builds crawler and sharing tags for one development page.
 * The caller replaces existing managed tags before inserting this block. Private
 * destinations receive only noindex metadata, with public sharing and graph fields omitted.
 */
export function renderSocialMetadata(settings: SocialMetadata, requestURL: string): string {
  const { shareTitle, description, canonical, image, indexable, schema } = resolvePageMetadata(
    settings,
    requestURL,
  )
  const tags: string[] = []
  /** add appends a nonempty field after escaping its attribute value. */
  function add(attribute: string, name: string, value: string) {
    if (value) tags.push(`<meta ${attribute}="${name}" content="${escapeAttribute(value)}" />`)
  }
  add('name', 'robots', indexable ? 'index, follow' : 'noindex, nofollow')
  if (!indexable) return tags.join('\n')
  add('name', 'description', description)
  if (canonical) tags.push(`<link rel="canonical" href="${escapeAttribute(canonical)}" />`)
  add('name', 'twitter:card', image ? 'summary_large_image' : 'summary')
  add('name', 'twitter:title', shareTitle)
  add('name', 'twitter:description', description)
  add('name', 'twitter:image', image)
  add('property', 'og:type', 'website')
  add('property', 'og:title', shareTitle)
  add('property', 'og:description', description)
  add('property', 'og:site_name', settings.site_name || '')
  add('property', 'og:url', canonical)
  add('property', 'og:image', image)
  if (image) {
    add('property', 'og:image:type', settings.site_share_image_type || '')
    if (settings.site_share_image_width)
      add('property', 'og:image:width', String(settings.site_share_image_width))
    if (settings.site_share_image_height)
      add('property', 'og:image:height', String(settings.site_share_image_height))
  }
  if (schema) {
    // Escape the HTML opening delimiter so administrator text cannot terminate the structured-data script in the containing document.
    tags.push(
      `<script id="site-structured-data" type="application/ld+json">${JSON.stringify(schema).replace(/</g, '\\u003c')}</script>`,
    )
  }
  return tags.join('\n')
}
