import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import postcss from 'postcss'
import ts from 'typescript'
import { describe, expect, it } from 'vitest'
import tailwindConfig from '../../tailwind.config.js'
import {
  applyTokens,
  cssRules,
  declarations,
  nodes,
  parseScript,
  readSource,
} from './helpers/sourceSyntax'

// fontTheme.spec.js checks heading-only use of the supplied font and its loading
// contract. Body and code retain their normal text roles, while heading selectors
// opt into the bundled face. Preload and CSS reference the same local font asset.

// fontFamily and fontCss expose the configured text roles and their owning font declarations for these checks.
const fontFamily = tailwindConfig.theme.extend.fontFamily
const fontCss = postcss.parse(readSource('styles/fonts.css'))

describe('font theme', () => {
  // Headings explicitly select the supplied face while interface and code text keep their normal roles.
  // This prevents a root token change from applying decorative glyphs to every label.
  it('separates the supplied heading font from normal text roles', () => {
    expect(fontFamily.sans).toEqual(['var(--font-app)'])
    expect(fontFamily.display).toEqual(['var(--font-heading)'])
    expect(fontFamily.mono).toContain('ui-monospace')
    expect(fontFamily.serif).toContain('ui-serif')
    const roots = cssRules(fontCss, ':root')
    expect(roots.flatMap((rule) => declarations(rule, '--font-heading'))).toEqual([
      "'Chang Ban Dian Song', monospace",
    ])
    expect(roots.flatMap((rule) => declarations(rule, '--font-app')).join()).toContain('system-ui')
    expect(roots.flatMap((rule) => declarations(rule, '--font-app')).join()).not.toContain(
      'Chang Ban Dian Song',
    )
    for (const role of ['sans', 'mono', 'serif']) {
      expect(fontFamily[role].join()).not.toMatch(/Chang Ban Dian Song|--font-heading/)
    }
  })

  // Semantic headings and shared title classes opt into the decorative face without changing their containers.
  // Paragraphs, navigation, controls, and experience statistics must retain the document font.
  it('limits the decorative font utility to headings and shared title classes', () => {
    const selectors = []
    postcss.parse(readSource('style.css')).walkRules((rule) => {
      if (applyTokens(rule).includes('font-display')) selectors.push(...rule.selectors)
    })
    expect(selectors.sort()).toEqual(
      [
        'h1',
        'h2',
        'h3',
        'h4',
        'h5',
        'h6',
        "[role='heading']",
        '.page-title',
        '.modal-title',
        '.empty-state-title',
      ].sort(),
    )
    const experience = postcss.parse(readSource('styles/experience.css'))
    expect(
      cssRules(experience, "[data-tr-plugin='helstare.exp'] .exp-page").flatMap((rule) =>
        declarations(rule, 'font-family'),
      ),
    ).toEqual(['var(--font-app)'])
  })

  // The stylesheet must point to the actual supplied WOFF2 file with its regular weight.
  // Swap keeps text visible while the font downloads.
  it('serves the supplied font without changing its font face', () => {
    const faces = []
    fontCss.walkAtRules('font-face', (rule) => faces.push(rule))
    expect(faces).toHaveLength(1)
    const face = faces[0]
    expect(declarations(face, 'font-family')).toEqual(["'Chang Ban Dian Song'"])
    expect(declarations(face, 'font-weight')).toEqual(['400'])
    expect(declarations(face, 'font-style')).toEqual(['normal'])
    expect(declarations(face, 'font-display')).toEqual(['swap'])
    const source = declarations(face, 'src')[0].match(/url\('([^']+)'\)/)[1]
    const bytes = readFileSync(
      resolve(dirname(fileURLToPath(import.meta.url)), '../styles', source),
    )
    expect(bytes.subarray(0, 4).toString()).toBe('wOF2')
  })

  // A local preload and the font-face source must resolve to one asset.
  // Remote font stylesheets would add a competing font download during startup.
  it('preloads the local font without requesting remote font stylesheets', () => {
    const document = new DOMParser().parseFromString(readSource('../index.html'), 'text/html')
    const preload = document.querySelector('link[rel="preload"][as="font"]')
    expect(preload?.getAttribute('href')).toBe(
      '/src/assets/fonts/chang-ban-dian-song-monospaced-12px-square.woff2',
    )
    expect(preload?.getAttribute('type')).toBe('font/woff2')
    expect(preload?.hasAttribute('crossorigin')).toBe(true)
    expect(document.querySelector('link[href*="fonts.googleapis.com"]')).toBeNull()
  })

  // Font rules load before the global styles apply their shared token.
  // Legacy font packages must not remain in the application's startup imports.
  it('loads the shared face before the global stylesheet', () => {
    const imports = nodes(parseScript(readSource('main.ts')), ts.isImportDeclaration).map(
      (node) => node.moduleSpecifier.text,
    )
    expect(imports.indexOf('./styles/fonts.css')).toBeGreaterThanOrEqual(0)
    expect(imports.indexOf('./styles/fonts.css')).toBeLessThan(imports.indexOf('./style.css'))
    expect(imports.some((path) => path.startsWith('@fontsource'))).toBe(false)
  })

  // The root document supplies the shared font to normal content and onboarding.
  // Third-party tour styles must inherit that font instead of replacing it.
  it('inherits the shared theme in the document and onboarding overlay', () => {
    expect(cssRules(postcss.parse(readSource('style.css')), 'html').flatMap(applyTokens)).toContain(
      'font-sans',
    )
    const onboarding = postcss.parse(readSource('styles/onboarding.css'))
    const fontDeclarations = []
    onboarding.walkDecls('font-family', (node) => fontDeclarations.push(node))
    expect(fontDeclarations.some((node) => node.value === 'inherit' && node.important)).toBe(true)
    expect(fontDeclarations.some((node) => node.value.startsWith('ui-sans-serif'))).toBe(false)
  })
})
