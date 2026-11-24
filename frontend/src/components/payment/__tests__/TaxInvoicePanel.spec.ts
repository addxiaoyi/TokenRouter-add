import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import TaxInvoicePanel from '../TaxInvoicePanel.vue'

// TaxInvoicePanel.spec.ts exercises buyer application, recovery, and document
// visibility using controlled backend responses. The fixtures never contact an
// invoice provider and keep uncertain issuance distinct from a confirmed invoice.

// API doubles expose every read, submission, and file request made by the mounted
// panel so tests can verify that background recovery never submits another invoice.
const { read, submit, file } = vi.hoisted(() => ({
  read: vi.fn(),
  submit: vi.fn(),
  file: vi.fn(),
}))
vi.mock('@/api/taxInvoices', () => ({ taxInvoicesAPI: { read, submit, file } }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))

// The stored buyer and application fixtures model an immutable application whose
// CNY cents are assigned by the backend rather than entered by the customer.
const buyer = {
  buyer_type: 'ORGANIZATION',
  title: 'Fixture Buyer',
  tax_id: '91310000987654321B',
  email: 'invoice@example.test',
}
const application = {
  id: 'invoice-42',
  ...buyer,
  status: 'UNKNOWN',
  invoice_no: '',
  amount_cents: 1000,
  error_code: '',
  updated_at: '2026-11-21T00:00:00Z',
}

enableAutoUnmount(afterEach)

// mountPanel supplies the selected order while retaining the real form behavior.
function mountPanel() {
  return mount(TaxInvoicePanel, { props: { orderId: 42 } })
}

describe('tax invoice application', () => {
  beforeEach(() => {
    read.mockReset().mockResolvedValue({ data: { enabled: true, eligible: true } })
    submit.mockReset().mockResolvedValue({
      data: { enabled: true, eligible: false, invoice: application },
    })
    file.mockReset().mockResolvedValue({ data: { url: 'https://example.test/invoice.pdf' } })
  })

  afterEach(() => vi.useRealTimers())

  // An unconfigured backend has no invoice history for this order. The customer
  // must not see a submission form or an enabled invoice action.
  it('hides new applications when the feature is disabled', async () => {
    read.mockResolvedValue({ data: { enabled: false, eligible: false } })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('[data-testid="tax-invoice"]').exists()).toBe(false)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(submit).not.toHaveBeenCalled()
  })

  // A customer completes organization buyer fields on an eligible order. Only
  // those fields are submitted, and an unknown result immediately locks editing.
  it('submits buyer fields once and preserves an uncertain application', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('input[value="ORGANIZATION"]').setValue()
    await wrapper.get('[name="invoice-title"]').setValue(buyer.title)
    await wrapper.get('[name="invoice-tax-id"]').setValue(buyer.tax_id)
    await wrapper.get('[name="invoice-email"]').setValue(buyer.email)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(submit).toHaveBeenCalledTimes(1)
    expect(submit).toHaveBeenCalledWith(42, buyer, expect.any(AbortSignal))
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.text()).toContain('status.UNKNOWN')
    expect(wrapper.find('[data-testid="invoice-file"]').exists()).toBe(false)
  })

  // Whitespace titles and malformed organization identifiers cannot form valid
  // buyer details. Rejected input stays editable and never reaches the submission API.
  it('rejects invalid buyer fields without freezing the form', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[name="invoice-title"]').setValue('   ')
    await wrapper.get('[name="invoice-email"]').setValue('buyer@example.test')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toBe('invalidBuyer')
    expect(wrapper.find('form').exists()).toBe(true)
    expect(submit).not.toHaveBeenCalled()
    await wrapper.get('input[value="ORGANIZATION"]').setValue()
    await wrapper.get('[name="invoice-title"]').setValue('Buyer')
    await wrapper.get('[name="invoice-tax-id"]').setValue('invalid')
    await wrapper.get('form').trigger('submit')
    expect(submit).not.toHaveBeenCalled()
  })

  // Disabling invoicing preserves historical buyer details and the confirmed
  // invoice number. It must hide document retrieval because the backend disables it.
  it('shows issued history without offering disabled document retrieval', async () => {
    read.mockResolvedValue({
      data: {
        enabled: false,
        eligible: false,
        invoice: { ...application, status: 'ISSUED', invoice_no: '12345678901234567890' },
      },
    })
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.text()).toContain('12345678901234567890')
    expect(wrapper.find('[data-testid="invoice-file"]').exists()).toBe(false)
    expect(submit).not.toHaveBeenCalled()
    expect(file).not.toHaveBeenCalled()
  })

  // A submission disconnects after the server may have accepted it. The panel
  // reads the original application and never posts again during status polling.
  it('recovers submission errors by reading without automatic reissue', async () => {
    vi.useFakeTimers()
    submit.mockRejectedValue(new Error('private upstream error'))
    read.mockResolvedValueOnce({ data: { enabled: true, eligible: true } }).mockResolvedValue({
      data: {
        enabled: true,
        eligible: false,
        invoice: { ...application, buyer_type: 'INDIVIDUAL', tax_id: '' },
      },
    })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[name="invoice-title"]').setValue('Fixture Buyer')
    await wrapper.get('[name="invoice-email"]').setValue(buyer.email)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(submit).toHaveBeenCalledTimes(1)
    expect(read.mock.calls.length).toBeGreaterThanOrEqual(3)
    expect(wrapper.text()).not.toContain('private upstream error')
    expect(wrapper.find('form').exists()).toBe(false)
  })

  // A validated rejection and a successful empty read prove no application was
  // persisted. The customer can correct buyer details without reopening order details.
  it('unlocks rejected buyer fields after confirming no application exists', async () => {
    submit.mockRejectedValueOnce({ status: 400 })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[name="invoice-title"]').setValue('Original Buyer')
    await wrapper.get('[name="invoice-email"]').setValue(buyer.email)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
    await wrapper.get('[name="invoice-title"]').setValue('Corrected Buyer')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(submit).toHaveBeenNthCalledWith(
      2,
      42,
      { buyer_type: 'INDIVIDUAL', title: 'Corrected Buyer', tax_id: '', email: buyer.email },
      expect.any(AbortSignal),
    )
  })

  // A lost response with no stored application does not establish rejection.
  // Explicit retry reuses the locked buyer snapshot while background reads never submit.
  it('offers an explicit unchanged retry when the original dispatch is uncertain', async () => {
    vi.useFakeTimers()
    submit.mockRejectedValueOnce({ status: 0 })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[name="invoice-title"]').setValue('Original Buyer')
    await wrapper.get('[name="invoice-email"]').setValue(buyer.email)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(30_000)
    await flushPromises()
    expect(submit).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-testid="invoice-retry"]').trigger('click')
    await flushPromises()
    expect(submit).toHaveBeenNthCalledWith(
      2,
      42,
      { buyer_type: 'INDIVIDUAL', title: 'Original Buyer', tax_id: '', email: buyer.email },
      expect.any(AbortSignal),
    )
    expect(wrapper.text()).toContain('status.UNKNOWN')
  })

  // A rejected submission is followed by a failed recovery read. A later
  // successful empty read unlocks editing without mistaking the read failure for rejection.
  it('waits for an authoritative empty read before unlocking rejected input', async () => {
    submit.mockRejectedValueOnce({ status: 400 })
    read
      .mockResolvedValueOnce({ data: { enabled: true, eligible: true } })
      .mockRejectedValueOnce({ status: 503 })
      .mockResolvedValue({ data: { enabled: true, eligible: true } })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[name="invoice-title"]').setValue('Original Buyer')
    await wrapper.get('[name="invoice-email"]').setValue(buyer.email)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
    expect(submit).toHaveBeenCalledTimes(1)
  })

  // A stored application remains readable after administrators disable creation.
  // Only a numbered issued record allows the customer to request its document.
  it.each(['UNKNOWN', 'PROCESSING', 'FAILED', 'REVERSING', 'REVERSED', 'ISSUED'])(
    'keeps %s history read-only and hides unconfirmed documents',
    async (status) => {
      read.mockResolvedValue({
        data: { enabled: false, eligible: false, invoice: { ...application, status } },
      })
      const wrapper = mountPanel()
      await flushPromises()
      expect(wrapper.text()).toContain(buyer.title)
      expect(wrapper.find('form').exists()).toBe(false)
      expect(wrapper.find('[data-testid="invoice-file"]').exists()).toBe(false)
      expect(submit).not.toHaveBeenCalled()
    },
  )

  // A numbered issued invoice can obtain a fresh HTTPS document link. The
  // customer follows that explicit link, and no permanent download URL is stored.
  it('requests the document only for a numbered issued invoice', async () => {
    read.mockResolvedValue({
      data: {
        enabled: true,
        eligible: false,
        invoice: { ...application, status: 'ISSUED', invoice_no: '12345678901234567890' },
      },
    })
    const wrapper = mountPanel()
    await flushPromises()
    await wrapper.get('[data-testid="invoice-file"]').trigger('click')
    await flushPromises()
    expect(file).toHaveBeenCalledWith(42, expect.any(AbortSignal))
    expect(wrapper.get('a').attributes('href')).toBe('https://example.test/invoice.pdf')
    expect(wrapper.get('a').attributes('rel')).toBe('noopener noreferrer')
  })

  // A closed order detail must stop status polling and release its requests.
  // Late network completion cannot start additional background reads after unmount.
  it('cancels requests and polling when the panel unmounts', async () => {
    vi.useFakeTimers()
    read.mockResolvedValue({ data: { enabled: true, eligible: false, invoice: application } })
    const wrapper = mountPanel()
    await flushPromises()
    const signal = read.mock.calls[0][1] as AbortSignal
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(signal.aborted).toBe(true)
    expect(read).toHaveBeenCalledTimes(1)
  })
})
