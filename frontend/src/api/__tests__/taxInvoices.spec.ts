import { beforeEach, describe, expect, it, vi } from 'vitest'
import { taxInvoicesAPI } from '../taxInvoices'

// taxInvoices.spec.ts verifies the authenticated order-scoped invoice boundary.
// Client doubles inspect paths and request fields while malformed file links are
// rejected before they can become navigation targets in the customer interface.

// Shared-client doubles retain normal Axios response shapes without issuing real
// requests, allowing the tests to inspect identity binding and cancellation forwarding.
const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { get, post } }))

describe('tax invoice API', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: { enabled: false, eligible: false } })
    post.mockReset().mockResolvedValue({ data: { enabled: true, eligible: false } })
  })

  // A buyer application belongs to the selected numeric payment order. The
  // adapter forwards cancellation without adding provider credentials or monetary fields.
  it('binds operations to the order and exact buyer fields', async () => {
    const signal = new AbortController().signal
    const buyer = {
      buyer_type: 'INDIVIDUAL' as const,
      title: 'Buyer',
      tax_id: '',
      email: 'buyer@example.test',
    }
    await taxInvoicesAPI.read(42, signal)
    await taxInvoicesAPI.submit(42, buyer, signal)
    expect(get).toHaveBeenCalledWith('/payment/orders/42/tax-invoice', { signal })
    expect(post).toHaveBeenCalledWith('/payment/orders/42/tax-invoice', buyer, { signal })
  })

  // Invalid identities must fail before authenticated requests are constructed.
  // File responses must use HTTPS and exclude embedded credentials or fragments.
  it('rejects invalid order IDs and unsafe download links', async () => {
    await expect(taxInvoicesAPI.read(0)).rejects.toThrow()
    expect(get).not.toHaveBeenCalled()
    for (const url of [
      'http://example.test/a.pdf',
      'javascript:alert(1)',
      'https://user:pass@example.test/a.pdf',
      'https://example.test/a.pdf#x',
    ]) {
      get.mockResolvedValue({ data: { url } })
      await expect(taxInvoicesAPI.file(42)).rejects.toThrow()
    }
  })
})
