// taxInvoices.types.ts defines the buyer-facing Chinese tax invoice contract.
// Applications contain buyer details only; the backend owns payment eligibility,
// invoice amounts, seller authorization, and the stable identity used for recovery.

// TaxInvoiceStatus distinguishes uncertain processing, issued documents, and
// reversal outcomes reported by the backend for one application. UNKNOWN and
// PROCESSING need status reads, ISSUED needs a number before download, FAILED
// needs support, and REVERSING or REVERSED keep the original buyer record locked.
export type TaxInvoiceStatus =
  'UNKNOWN' | 'PROCESSING' | 'ISSUED' | 'FAILED' | 'REVERSING' | 'REVERSED'

// TaxInvoiceBuyer contains the immutable customer details submitted for one order.
export interface TaxInvoiceBuyer {
  buyer_type: 'INDIVIDUAL' | 'ORGANIZATION' // Customer legal identity category.
  title: string // Customer invoice title.
  tax_id: string // Organization tax registration identifier; empty for individuals.
  email: string // Customer document delivery email.
}

// TaxInvoice contains the persisted application and server-confirmed issuance status.
export interface TaxInvoice extends TaxInvoiceBuyer {
  id: string // Stored application identifier.
  status: TaxInvoiceStatus // Current application lifecycle state.
  invoice_no: string // Issued document number; empty until confirmed.
  amount_cents: number // Collected amount in CNY cents.
  error_code: string // Safe backend failure code.
  updated_at: string // Last stored update timestamp.
}

// TaxInvoiceState combines creation eligibility with any previously stored application.
export interface TaxInvoiceState {
  enabled: boolean // New application feature availability.
  eligible: boolean // Backend-verified order creation eligibility.
  reason?: string // Safe eligibility reason code.
  // invoice remains visible when creation is disabled, allowing the customer to
  // inspect an existing application without treating feature availability as deletion.
  invoice?: TaxInvoice | null
}
