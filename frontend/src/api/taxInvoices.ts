import { apiClient } from './client'
import type { TaxInvoiceBuyer, TaxInvoiceState } from './taxInvoices.types'

// taxInvoices.ts sends authenticated invoice operations for a single owned order.
// The API receives buyer fields while the backend resolves the payment mapping
// and seller configuration. Download responses are checked before entering browser navigation.

// invoicePath validates the selected payment identity before constructing its API path.
function invoicePath(orderId: number): string {
  if (!Number.isSafeInteger(orderId) || orderId <= 0) {
    throw new Error('Invalid payment order ID')
  }
  return `/payment/orders/${orderId}/tax-invoice`
}

// taxInvoicesAPI groups order-scoped reads, application submission, and temporary
// document retrieval; all operations retain the shared client's authentication behavior.
export const taxInvoicesAPI = {
  // read retrieves eligibility and the original application's current stored status.
  // An optional abort signal belongs to the mounted detail panel and prevents
  // closed panels from continuing their requests.
  async read(orderId: number, signal?: AbortSignal) {
    return apiClient.get<TaxInvoiceState>(invoicePath(orderId), { signal })
  },

  // submit creates or resumes the selected order's immutable buyer application.
  // Callers preserve the original fields after dispatch and recover uncertainty
  // through read; this method does not schedule retries.
  async submit(orderId: number, buyer: TaxInvoiceBuyer, signal?: AbortSignal) {
    const { buyer_type, title, tax_id, email } = buyer
    return apiClient.post<TaxInvoiceState>(
      invoicePath(orderId),
      { buyer_type, title, tax_id, email },
      { signal },
    )
  },

  // file retrieves a fresh HTTPS document address for an issued invoice.
  // The address is temporary and must not be saved as a permanent document
  // location. Invalid protocols, credentials, or fragments reject the response.
  async file(orderId: number, signal?: AbortSignal) {
    const reply = await apiClient.get<{
      url: string // Short-lived HTTPS document address.
    }>(`${invoicePath(orderId)}/file`, { signal })
    const link = new URL(reply.data.url)
    if (link.protocol !== 'https:' || link.username || link.password || link.hash) {
      throw new Error('Invalid invoice document URL')
    }
    return reply
  },
}
