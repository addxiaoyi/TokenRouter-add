package payment

import (
	"context"
	"time"
)

// tax_invoice_contract.go defines durable buyer applications and their payment
// association for Chinese digital invoices. A recharge is not automatically an
// invoiceable sale: a trusted financial approval is required for each order.
// Buyer fields, the confirmed platform trade number, collected CNY amount and
// merchant binding are captured before contacting the invoicing platform.
//
// Storage owns the order lock, unique application and worker lease. The gateway
// owns transport authentication and returns observations for the original request.
// UNKNOWN survives communication failures, so workers recover that same request
// rather than inventing another invoice. Browser responses exclude deployment
// credentials, platform identities and internal lease fields while preserving
// the buyer's own saved details for review.

// TaxInvoiceConfig binds one approved payment source to its invoicing merchant.
type TaxInvoiceConfig struct {
	// Enabled permits creation, synchronization and file retrieval. False leaves
	// existing owned applications readable without constructing an authenticated gateway.
	Enabled bool // explicit deployment activation flag
	// Endpoint, MerchantID, EpayPID and ProviderInstanceID bind an invoice merchant
	// to one checkout source. Applications capture all four and recovery requires
	// exact equality with the active deployment binding before contacting upstream.
	Endpoint           string // complete HTTPS API prefix
	MerchantID         string // main-site merchant identity
	EpayPID            string // expected payment merchant identity
	ProviderInstanceID string // expected checkout instance identity
}

// TaxInvoiceBuyer contains the immutable customer supplied invoice delivery details.
type TaxInvoiceBuyer struct {
	BuyerType string `json:"buyer_type"` // individual or organization classification
	Title     string `json:"title"`      // buyer legal invoice title
	// TaxID must be present for organization buyers and empty for individuals;
	// validation preserves the supplied canonical value for later immutable comparisons.
	TaxID string `json:"tax_id"` // organization tax registration number
	Email string `json:"email"`  // buyer invoice delivery address
}

// TaxInvoice retains one immutable request and its latest local observation.
type TaxInvoice struct {
	// ID, OrderID, UserID, RequestID, TradeNo and AmountCents are reserved together
	// before network dispatch. Recovery preserves this association so a lost
	// response cannot attach a new request or payment to the original buyer.
	ID          string `json:"id"`           // local application UUID identity
	OrderID     int64  `json:"-"`            // owned business order identity
	UserID      int64  `json:"-"`            // authenticated purchasing account identity
	RequestID   string `json:"-"`            // persistent upstream idempotency identity
	TradeNo     string `json:"-"`            // confirmed platform payment number
	AmountCents int64  `json:"amount_cents"` // CNY minor units
	// BuyerType, Title, TaxID and Email retain the customer's first accepted
	// submission. Apply rejects changed values instead of replacing buyer details
	// on an application that may already have reached the issuer.
	BuyerType string `json:"buyer_type"` // individual or organization classification
	Title     string `json:"title"`      // immutable buyer invoice title
	TaxID     string `json:"tax_id"`     // immutable organization tax identifier
	Email     string `json:"email"`      // immutable invoice delivery address
	// Endpoint, MerchantID, EpayPID and ProviderInstanceID capture the deployment
	// association at reservation. A later configuration change suspends recovery
	// unless all four still match, protecting the original seller account.
	Endpoint           string `json:"-"` // captured upstream endpoint prefix
	MerchantID         string `json:"-"` // captured main-site merchant identity
	EpayPID            string `json:"-"` // captured payment merchant identity
	ProviderInstanceID string `json:"-"` // captured payment instance identity
	// Status starts as UNKNOWN. UNKNOWN, PROCESSING and REVERSING are synchronized;
	// ISSUED, FAILED and REVERSED stop automatic observation in the business worker.
	Status string `json:"status"` // latest confirmed workflow status
	// RemoteID remains empty until a matching upstream observation arrives.
	// Recovery then changes from replaying the original request to querying this ID.
	RemoteID string `json:"-"` // upstream invoice record identity
	// InvoiceNo is required before an ISSUED record can offer document retrieval;
	// reversal records may retain the number of the original issued invoice.
	InvoiceNo string `json:"invoice_no"` // issued tax invoice number
	// ErrorCode contains a validated upstream identifier or a fixed transport
	// failure label, so browser responses never need raw provider error messages.
	ErrorCode string    `json:"error_code"` // safe machine-readable failure code
	UpdatedAt time.Time `json:"updated_at"` // latest persisted observation time
	// NextSyncAt delays each later recovery attempt after the previous observation;
	// the store also checks status and the worker lease before admission.
	NextSyncAt time.Time `json:"-"` // earliest scheduled recovery time
	// LeaseUntil identifies the exact worker claim accepted by Save. A stale
	// worker cannot publish after another worker obtains a replacement lease.
	LeaseUntil time.Time `json:"-"`
}

// TaxInvoiceOrder combines payment facts with a trusted financial eligibility decision.
type TaxInvoiceOrder struct {
	Order *Order // current persisted payment observation
	// Approved comes from a trusted sale approval, not payment completion alone;
	// new applications require this decision alongside confirmed payment and refund checks.
	Approved bool // active trusted financial approval
}

// TaxInvoiceView exposes eligibility and the buyer's existing application together.
type TaxInvoiceView struct {
	// Enabled, Eligible and Reason describe current admission separately from
	// application history. Eligible requires an empty Reason; Enabled also controls
	// file retrieval even when a saved issued record is present.
	Enabled  bool   `json:"enabled"`          // deployment creation capability flag
	Eligible bool   `json:"eligible"`         // current new-application eligibility flag
	Reason   string `json:"reason,omitempty"` // safe eligibility refusal code
	// Invoice is omitted when no application exists. Disabled deployments retain
	// an owned saved record so the customer can still inspect its history.
	Invoice *TaxInvoice `json:"invoice,omitempty"` // existing owned invoice application
}

// TaxInvoiceRemote represents an observation returned by the signed invoice API.
type TaxInvoiceRemote struct {
	// ID, RequestID, TradeNo, Title and AmountCents identify the returned application.
	// The service compares these values with its saved record before accepting a
	// status update, and a previously recorded remote ID cannot be replaced.
	ID          string `json:"id"`          // upstream local record UUID
	RequestID   string `json:"requestId"`   // original idempotency request identity
	TradeNo     string `json:"tradeNo"`     // original platform payment number
	Title       string `json:"title"`       // original buyer invoice title
	AmountCents int64  `json:"amountCents"` // CNY minor units
	Status      string `json:"status"`      // observed upstream invoice status
	// InvoiceNo must be present for ISSUED and absent for UNKNOWN, PROCESSING
	// and FAILED; reversal observations may preserve the original document number.
	InvoiceNo string `json:"invoiceNo"` // issued tax invoice number
	// ErrorCode is validated before storage. A FAILED observation also needs a
	// recognized definite-failure code before the service accepts that terminal outcome.
	ErrorCode string `json:"errorCode"` // safe upstream failure identifier
}

// TaxInvoiceStore persists applications before dispatch and serializes their recovery.
type TaxInvoiceStore interface {
	// Read returns an owned order and its optional invoice application. Missing
	// or foreign orders must have the same not-found response to prevent disclosure.
	// A nil application means the owned order has no saved invoice request.
	Read(context.Context, int64, int64) (*TaxInvoiceOrder, *TaxInvoice, error)
	// Reserve locks the payment order and validates its latest eligibility before
	// inserting one application. Repeated identical requests return the original
	// record; changed buyer or payment associations must be rejected atomically.
	Reserve(context.Context, *TaxInvoice, func(*TaxInvoiceOrder) error) (*TaxInvoice, error)
	// Claim reserves a due nonterminal record and returns its persisted lease
	// identity. The boolean is false when another worker owns recovery or the
	// record no longer needs it; callers then leave the observation unchanged.
	Claim(context.Context, string, time.Time) (*TaxInvoice, bool, error)
	// Save publishes an observation only for its matching current worker lease.
	// It retains every immutable field and clears the lease after persistence;
	// expired claims cannot overwrite a newer worker's observation.
	Save(context.Context, *TaxInvoice) error
	// Due returns a bounded page of scheduled nonterminal applications matching
	// the deployment's endpoint, merchant, payment pid and provider instance. The
	// binding predicate must precede pagination so suspended accounts cannot occupy
	// every slot; workers still call Claim before any gateway request.
	Due(context.Context, TaxInvoiceConfig, time.Time, int) ([]*TaxInvoice, error)
}

// TaxInvoiceGateway authenticates commands to one configured upstream merchant account.
type TaxInvoiceGateway interface {
	// Create submits the exact persisted application once and returns its
	// original upstream record. Repeating the same identity must never create a
	// second invoice, including recovery after an unknown transport outcome.
	Create(context.Context, *TaxInvoice) (*TaxInvoiceRemote, error)
	// Query reads the original upstream record without issuing another invoice.
	// The caller verifies payment and application identity before persisting its
	// observation; transport failures leave the previous outcome unresolved.
	Query(context.Context, string) (*TaxInvoiceRemote, error)
	// Files requests a fresh HTTPS download link after the upstream rechecks
	// issuance. Callers establish buyer ownership and must not persist the
	// temporary link as the permanent location of the invoice.
	Files(context.Context, string) (string, error)
}
