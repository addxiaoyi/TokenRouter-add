// taxInvoice.ts supplies English text for order invoice applications and recovery.
// These messages are loaded by the invoice panel and order details without
// changing the shared payment catalog or provider-specific Stripe document labels.

export default {
  title: 'Tax invoice',
  loading: 'Checking invoice availability…',
  unavailable: 'This order is not currently available for invoicing.',
  loadError: 'Could not load the invoice. Refresh to check again.',
  submitError:
    'The submission result is not confirmed. Refresh the original application before continuing.',
  submitRejected: 'This application was not accepted. Check the details and try again.',
  fileError: 'Could not retrieve the invoice document. Try again later.',
  invalidBuyer: 'Check the invoice title, tax registration number, and email.',
  buyerType: 'Buyer type',
  individual: 'Individual',
  organization: 'Organization',
  buyerTitle: 'Invoice title',
  taxId: 'Tax registration number',
  email: 'Email',
  buyerHint: 'Check these details before submitting. They will be saved with this order.',
  submit: 'Submit invoice application',
  submitting: 'Submitting…',
  retrySubmission: 'Retry with original details',
  retryHint: 'Submission is not confirmed. Retrying will use the buyer details shown above.',
  refresh: 'Refresh status',
  amount: 'Invoice amount (CNY)',
  invoiceNumber: 'Invoice number',
  saved: 'Buyer details are saved and cannot be edited here.',
  pending: 'The original application is being checked. You can close this window and return later.',
  unconfirmed: 'Issuance not confirmed',
  requestFile: 'Get invoice document',
  download: 'Download invoice',
  downloadHint: 'This link expires shortly. Request it again if it has expired.',
  status: {
    UNKNOWN: 'Awaiting confirmation',
    PROCESSING: 'Processing',
    ISSUED: 'Issued',
    FAILED: 'Application failed; contact support',
    REVERSING: 'Reversal in progress',
    REVERSED: 'Reversed',
  },
}
