import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { userRoutes } from '../routes/user'

// userRoutes.spec.ts resolves account destinations against the actual user route records.
// Lazy components remain unloaded, while route matching and redirects use the real
// router so combined account links cannot silently fall through to generic plugin
// routes or disappear from the published route inventory.

describe('account destination routing', () => {
  // Combined verification, binding, and monitoring links need concrete user route records.
  // Resolution retains authentication metadata and reaches each intended account component.
  it('registers both account pages and service monitoring', () => {
    const router = createRouter({ history: createMemoryHistory(), routes: userRoutes })
    for (const path of ['/account/verification', '/account/bindings', '/monitoring']) {
      const destination = router.resolve(path)
      expect(destination.matched).toHaveLength(1)
      expect(destination.meta.requiresAuth).toBe(true)
      expect(destination.meta.requiresAdmin).toBe(false)
    }
    expect(router.resolve('/monitoring').name).toBe('ServiceMonitoring')
  })

  // Saved plugin URLs must match dedicated redirects before the generic plugin host.
  // Each redirect selects the corresponding tab without granting administrator access.
  it.each([
    ['/ext/helstare.partner/verify/identity', '/account/verification', 'identity'],
    ['/ext/helstare.partner/verify/enterprise', '/account/verification', 'enterprise'],
    ['/ext/helstare.qqbot', '/account/bindings', 'qq'],
    ['/ext/helstare.tgbot', '/account/bindings', 'tg'],
  ])('redirects %s to its combined account tab', (legacy, path, tab) => {
    const router = createRouter({ history: createMemoryHistory(), routes: userRoutes })
    expect(router.resolve(legacy).matched[0]?.redirect).toEqual({ path, query: { tab } })
    expect(router.resolve('/admin/ext/helstare.qqbot').matched).toHaveLength(0)
  })
})
