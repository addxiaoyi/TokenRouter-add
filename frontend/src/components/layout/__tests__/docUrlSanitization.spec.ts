import ts from 'typescript'
import { describe, expect, it } from 'vitest'
import { sanitizeUrl } from '@/utils/url'
import { attribute, nodes, parseVue, variableInitializer } from '@/__tests__/helpers/sourceSyntax'

// docUrlSanitization.spec.ts checks document-link sanitization at each public
// entry point and verifies the URL policy with representative protocols. Entries
// that delegate documentation navigation identify their rendered owner explicitly.
// Script assertions inspect that owner's computed callback, so an unrelated logo
// sanitizer cannot satisfy the document-link contract.

// entryOwners pairs each public entry with the component that sanitizes its
// documentation link, retaining coverage when navigation moves into a shared header.
const entryOwners = [
  ['components/layout/AppHeader.vue', 'components/layout/AppHeader.vue'],
  ['views/HomeView.vue', 'views/HomeView.vue'],
  ['views/KeyUsageView.vue', 'views/KeyUsageView.vue'],
  ['views/ModelMarketplaceView.vue', 'components/layout/AppHeader.vue'],
]

describe('doc_url sanitization', () => {
  // Public entries either own their documentation link or render the shared public header.
  // The actual owner must return the sanitizer result directly before binding that URL.
  it.each(entryOwners)('%s uses sanitized documentation from %s', (entryPath, ownerPath) => {
    const entry = parseVue(entryPath)
    if (entryPath !== ownerPath) {
      const header = entry.elements.find((element) => element.tag === 'AppHeader')
      expect(header).toBeDefined()
      if (!header) throw new Error('Missing shared documentation header')
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
    const imports = nodes(script, ts.isImportDeclaration)
    expect(
      imports.some(
        (node) =>
          ts.isStringLiteral(node.moduleSpecifier) &&
          node.moduleSpecifier.text === '@/utils/url' &&
          nodes(node, ts.isImportSpecifier).some((binding) => binding.name.text === 'sanitizeUrl'),
      ),
    ).toBe(true)
    const computed = variableInitializer(script, 'docUrl')
    expect(ts.isCallExpression(computed)).toBe(true)
    if (!ts.isCallExpression(computed)) throw new Error('docUrl must call computed')
    expect(computed.expression.getText(script)).toBe('computed')
    const getter = computed.arguments[0]
    expect(ts.isArrowFunction(getter)).toBe(true)
    if (!ts.isArrowFunction(getter) || !ts.isCallExpression(getter.body))
      throw new Error('Missing sanitizing getter')
    expect(getter.body.expression.getText(script)).toBe('sanitizeUrl')
    expect(getter.body.arguments).toHaveLength(1)
    expect(
      nodes(getter.body.arguments[0], ts.isPropertyAccessExpression).map((node) => node.name.text),
    ).toContain('docUrl')
  })

  // Configured document links can use executable or opaque protocols. The URL
  // policy rejects these inputs before a component can bind an anchor href.
  it('rejects executable and non-web protocols', () => {
    expect(sanitizeUrl('javascript:alert(1)')).toBe('')
    expect(sanitizeUrl('data:text/html,<script>alert(1)</script>')).toBe('')
    expect(sanitizeUrl('//evil.example/path')).toBe('')
  })

  // Administrators may configure either HTTP or HTTPS documentation hosts. Both
  // supported protocols remain usable, including URL normalization of host-only inputs.
  it('keeps valid HTTP and HTTPS document links', () => {
    expect(sanitizeUrl('https://docs.example.com/guide')).toBe('https://docs.example.com/guide')
    expect(sanitizeUrl('http://docs.example.com')).toBe('http://docs.example.com/')
  })
})
