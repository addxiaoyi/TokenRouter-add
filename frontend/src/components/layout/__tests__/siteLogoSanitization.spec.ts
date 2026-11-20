import ts from 'typescript'
import { describe, expect, it } from 'vitest'
import { attribute, nodes, parseVue, variableInitializer } from '@/__tests__/helpers/sourceSyntax'
import { sanitizeUrl } from '@/utils/url'

// siteLogoSanitization.spec.ts checks sanitizer inputs and options for site
// branding and favicon consumers, including shared branding owners. Syntax
// assertions attach relative and image-data permissions to the actual logo
// sanitizer call; URL probes preserve rejection of executable or non-image inputs.

// entries maps branding consumers to their actual owner and its cached-settings
// fallback requirement, keeping shared-header pages covered after local logos disappear.
const entries = [
  ['components/layout/AppHeader.vue', 'components/layout/AppHeader.vue', false],
  ['views/HomeView.vue', 'views/HomeView.vue', true],
  ['views/KeyUsageView.vue', 'views/KeyUsageView.vue', true],
  ['views/ModelMarketplaceView.vue', 'components/layout/AppHeader.vue', false],
  ['App.vue', 'App.vue', false],
] as const

describe('site_logo sanitization', () => {
  // Branding accepts local assets and data images while sanitizing configured
  // URLs. Each actual branding owner must pass these permissions on its sanitizer call.
  it.each(entries)('%s uses sanitized branding from %s', (entryPath, ownerPath, cached) => {
    const entry = parseVue(entryPath)
    if (entryPath !== ownerPath) {
      const header = entry.elements.find((element) => element.tag === 'AppHeader')
      expect(header).toBeDefined()
      if (!header) throw new Error('Missing shared branding header')
      expect(attribute(header, 'public-page')).toBe('')
      expect(
        nodes(entry.script, ts.isImportDeclaration).some(
          (node) =>
            ts.isStringLiteral(node.moduleSpecifier) &&
            node.moduleSpecifier.text === `@/${ownerPath}` &&
            node.importClause?.name?.text === 'AppHeader',
        ),
      ).toBe(true)
    }
    const script = parseVue(ownerPath).script
    const scope = ownerPath === 'App.vue' ? script : variableInitializer(script, 'siteLogo')
    const calls = nodes(scope, ts.isCallExpression).filter(
      (node) => ts.isIdentifier(node.expression) && node.expression.text === 'sanitizeUrl',
    )
    const call = calls.find(
      (node) =>
        node.arguments[0] &&
        nodes(node.arguments[0], ts.isPropertyAccessExpression).some(
          (property) => property.name.text === 'siteLogo',
        ),
    )
    expect(call).toBeDefined()
    if (!call) throw new Error('Missing logo sanitizer call')
    const input = call.arguments[0]
    expect(ts.isBinaryExpression(input)).toBe(true)
    if (!ts.isBinaryExpression(input)) throw new Error('Missing empty logo fallback')
    expect(input.operatorToken.kind).toBe(ts.SyntaxKind.BarBarToken)
    expect(ts.isStringLiteral(input.right) && input.right.text).toBe('')
    let storeLogo = input.left
    if (cached) {
      expect(ts.isBinaryExpression(storeLogo)).toBe(true)
      if (!ts.isBinaryExpression(storeLogo)) throw new Error('Missing cached logo precedence')
      expect(storeLogo.operatorToken.kind).toBe(ts.SyntaxKind.BarBarToken)
      const cachedLogo = storeLogo.left
      expect(ts.isPropertyAccessExpression(cachedLogo)).toBe(true)
      if (!ts.isPropertyAccessExpression(cachedLogo))
        throw new Error('Missing cached logo property')
      expect(cachedLogo.name.text).toBe('site_logo')
      expect(cachedLogo.questionDotToken).toBeDefined()
      expect(
        ts.isPropertyAccessExpression(cachedLogo.expression) && cachedLogo.expression.name.text,
      ).toBe('cachedPublicSettings')
      storeLogo = storeLogo.right
    }
    expect(ts.isPropertyAccessExpression(storeLogo)).toBe(true)
    if (!ts.isPropertyAccessExpression(storeLogo)) throw new Error('Missing store logo property')
    expect(storeLogo.name.text).toBe('siteLogo')
    expect(ts.isIdentifier(storeLogo.expression) && storeLogo.expression.text).toBe('appStore')
    const options = call.arguments[1]
    expect(ts.isObjectLiteralExpression(options)).toBe(true)
    if (!ts.isObjectLiteralExpression(options)) throw new Error('Missing logo sanitizer options')
    const values = Object.fromEntries(
      options.properties
        .filter(ts.isPropertyAssignment)
        .map((node) => [
          ts.isIdentifier(node.name) || ts.isStringLiteral(node.name) ? node.name.text : '',
          node.initializer.kind,
        ]),
    )
    expect(values).toMatchObject({
      allowRelative: ts.SyntaxKind.TrueKeyword,
      allowDataUrl: ts.SyntaxKind.TrueKeyword,
    })
    expect(
      nodes(script, ts.isImportDeclaration).some(
        (node) =>
          ts.isStringLiteral(node.moduleSpecifier) &&
          node.moduleSpecifier.text === '@/utils/url' &&
          nodes(node, ts.isImportSpecifier).some((binding) => binding.name.text === 'sanitizeUrl'),
      ),
    ).toBe(true)
  })

  // Logo permissions admit local assets and image data, never arbitrary executable
  // URLs. Non-image data payloads and protocol-relative hosts remain rejected by that same policy.
  it('rejects executable and non-image logo inputs with branding permissions enabled', () => {
    const options = { allowRelative: true, allowDataUrl: true }
    expect(sanitizeUrl('javascript:alert(1)', options)).toBe('')
    expect(sanitizeUrl('data:text/html,<script>alert(1)</script>', options)).toBe('')
    expect(sanitizeUrl('//evil.example/logo.svg', options)).toBe('')
    expect(sanitizeUrl('/logo.svg', options)).toBe('/logo.svg')
    expect(sanitizeUrl('data:image/png;base64,AA==', options)).toBe('data:image/png;base64,AA==')
  })
})
