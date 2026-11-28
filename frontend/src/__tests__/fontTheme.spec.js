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

// fontTheme.spec.js checks the supplied heading font and the system stacks used
// for body, code, and serif text. Semantic utilities keep their distinct roles,
// while bundled font loading and heading selectors preserve the intended display scope.

// fontFamily and fontCss expose the configured text roles and their owning font declarations for these checks.
const fontFamily = tailwindConfig.theme.extend.fontFamily
const fontCss = postcss.parse(readSource('styles/fonts.css'))

describe('font theme', () => {
  // Headings use the supplied pixel face while other semantic text roles retain
  // system stacks. The display fallback keeps headings readable before the bundled font loads.
  it('separates the supplied heading face from system text stacks', () => {
    expect(fontFamily.sans).toEqual(['var(--font-app)'])
    expect(fontFamily.display).toEqual(['var(--font-display)'])
    expect(fontFamily.mono).toEqual(['var(--font-mono)'])
    expect(fontFamily.serif).toEqual(['var(--font-serif)'])
    const roots = cssRules(fontCss, ':root')
    const bodyFonts = roots.flatMap((rule) => declarations(rule, '--font-app'))
    expect(bodyFonts).toHaveLength(1)
    expect(bodyFonts[0]).toMatch(/^-apple-system,\s*BlinkMacSystemFont,/)
    expect(bodyFonts[0]).toContain('sans-serif')
    expect(bodyFonts[0]).not.toContain('Chang Ban Dian Song')
    expect(roots.flatMap((rule) => declarations(rule, '--font-display'))).toEqual([
      "'Chang Ban Dian Song', var(--font-app)",
    ])
    expect(roots.flatMap((rule) => declarations(rule, '--font-mono'))).toEqual([
      expect.stringMatching(/^ui-monospace,\s*SFMono-Regular,/),
    ])
    expect(roots.flatMap((rule) => declarations(rule, '--font-serif'))).toEqual([
      "ui-serif, Georgia, Cambria, 'Times New Roman', serif",
    ])
  })

  // Semantic headings select the display utility while the experience plugin's
  // ordinary content inherits the body token. The selector list keeps global pixel styling scoped to titles.
  it('scopes display utilities to headings and keeps experience content on the body token', () => {
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
