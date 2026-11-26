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

// fontTheme.spec.js checks the supplied font asset and its application-wide loading
// contract. Semantic text roles retain their utility names while every role resolves
// to the same bundled face, and preload and CSS reference the same local asset.

// fontFamily and fontCss expose the configured text roles and their owning font declarations for these checks.
const fontFamily = tailwindConfig.theme.extend.fontFamily
const fontCss = postcss.parse(readSource('styles/fonts.css'))

describe('font theme', () => {
  // Every semantic text role resolves to the supplied face so a component cannot
  // accidentally reintroduce a system stack through a utility alias. This keeps
  // semantic utility names useful without permitting separate font-family values.
  it('uses the supplied font stack for every text role', () => {
    expect(fontFamily.sans).toEqual(['var(--font-app)'])
    expect(fontFamily.display).toEqual(['var(--font-app)'])
    expect(fontFamily.mono).toEqual(['var(--font-app)'])
    expect(fontFamily.serif).toEqual(['var(--font-app)'])
    const roots = cssRules(fontCss, ':root')
    expect(roots.flatMap((rule) => declarations(rule, '--font-app'))).toEqual([
      "'Chang Ban Dian Song', monospace",
    ])
  })

  // Semantic headings keep their display utility, but the shared token means body,
  // controls, charts, and experience statistics resolve to the same family. The test
  // also checks that the experience plugin does not override the application stack.
  it('keeps semantic heading utilities on the shared font token', () => {
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

  // The document must not request a source-only font path that production bundling rewrites.
  // The imported stylesheet owns the hashed asset URL and avoids duplicate font requests.
  it('keeps the document free of source-only font preloads', () => {
    const document = new DOMParser().parseFromString(readSource('../index.html'), 'text/html')
    expect(document.querySelector('link[rel="preload"][as="font"]')).toBeNull()
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
