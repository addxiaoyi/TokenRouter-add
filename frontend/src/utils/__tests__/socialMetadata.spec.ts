import { describe, expect, it } from 'vitest'
import { renderSocialMetadata } from '../../../socialMetadata'

// socialMetadata.spec.ts checks development metadata without running Vite or a
// backend. The fixtures cover attribute escaping, branding fallbacks, configured
// destinations, and missing covers, preserving crawler-visible behavior in local HTML.

describe('social metadata', () => {
  // Homepage sharing copy contains HTML-sensitive text and a cover resource path.
  // Both protocols receive escaped copy and absolute URLs, while model pages retain their own titles.
  it('renders escaped sharing cards and configured destinations', () => {
    const settings = {
      site_share_title: 'Share "<title>',
      site_share_description: 'A & B',
      site_share_image_url: '/api/v1/settings/share-image/abc',
      site_share_image_type: 'image/png',
      site_share_image_width: 1200,
      site_share_image_height: 630,
      frontend_url: 'https://public.test',
    }
    const tags = renderSocialMetadata(settings, 'http://localhost:3000/?secret=value')
    expect(tags).toContain('summary_large_image')
    expect(tags).toContain('property="og:title" content="Share &quot;&lt;title&gt;"')
    expect(tags).toContain('name="twitter:title" content="Share &quot;&lt;title&gt;"')
    expect(tags).toContain('property="og:description" content="A &amp; B"')
    expect(tags).toContain('rel="canonical" href="https://public.test/"')
    expect(tags).toContain('https://public.test/api/v1/settings/share-image/abc')
    expect(tags).not.toContain('secret')

    const models = renderSocialMetadata(settings, 'http://localhost:3000/models?secret=value')
    expect(models).toContain('property="og:title" content="Models &amp; pricing | TokenRouter"')
    expect(models).toContain('name="twitter:title" content="Models &amp; pricing | TokenRouter"')
    expect(models).toContain('rel="canonical" href="https://public.test/models"')
    expect(models).toContain('https://public.test/api/v1/settings/share-image/abc')
    expect(models).not.toContain('Share &quot;&lt;title&gt;')
    expect(models).not.toContain('secret')
  })

  // Older public settings contain only site branding and no cover. Development
  // HTML still advertises summary metadata with the homepage title and subtitle.
  it('uses branding defaults without an image', () => {
    const tags = renderSocialMetadata(
      { site_name: 'Site', site_title: 'Home', site_subtitle: 'About' },
      'http://localhost:3000/',
    )
    expect(tags).toContain('name="twitter:card" content="summary"')
    expect(tags).toContain('property="og:title" content="Home"')
    expect(tags).toContain('property="og:description" content="About"')
    expect(tags).not.toContain('og:image')
  })
})
