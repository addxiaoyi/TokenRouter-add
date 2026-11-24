package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/payment"
)

// tax_invoices.go signs merchant-scoped commands for the PerPay tax invoice API.
// The main-site HMAC credentials are independent of EasyPay payment credentials.
// Each request carries a signed operation and merchant identity so a payment
// signature cannot be reused as permission to issue a tax invoice. Requests use
// HTTPS, a fixed timeout, no redirects and bounded JSON messages.
//
// The adapter makes one HTTP request per call and never generates idempotency
// keys. The payment service persists the original application before calling it
// and retains uncertainty after every transport or protocol failure. Returned
// merchant identity is checked here; order, buyer and amount correspondence are
// checked by the payment service before saving the observation or presenting it.

// These message limits match the merchant automation contract and bound parsing
// before an upstream response is allowed to influence invoice recovery.
const (
	taxInvoiceRequestLimit  = 8192
	taxInvoiceResponseLimit = 65536
)

// PerPayInvoices retains one fixed merchant and its server-only authentication secret.
type PerPayInvoices struct {
	base       *url.URL // verified HTTPS endpoint prefix
	merchantID string   // bound main-site merchant identity
	apiID      string   // merchant HMAC API identifier
	secret     string   // server-only HMAC signing secret
	// client owns request deadlines and rejects redirects before a second
	// destination can receive the original merchant authentication headers.
	client *http.Client // bounded no-redirect HTTP client
}

// perPayInvoiceReply carries the merchant identity and operation-specific response value.
type perPayInvoiceReply struct {
	// MerchantID must match the configured account before either invoice state
	// or a document link can leave the authenticated transport boundary.
	MerchantID string `json:"merchantId"` // authenticated account response association
	// Invoice supplies create and query observations. Files responses omit it;
	// callers still validate a returned observation against the persisted application.
	Invoice *payment.TaxInvoiceRemote `json:"invoice"` // original invoice record observation
	// URL is populated by the files operation and validated by the payment
	// service before customer navigation; create and query responses may omit it.
	URL string `json:"url"` // short-lived HTTPS document link
}

// NewPerPayInvoices binds validated HTTPS configuration and independent HMAC credentials.
// The optional transport supports explicit certificate trust in tests; production
// callers pass nil and retain Go's certificate verification and proxy behavior.
func NewPerPayInvoices(endpoint, merchantID, apiID, secret string, transport http.RoundTripper) (*PerPayInvoices, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !strings.HasSuffix(base.Path, "/") {
		return nil, errors.New("invalid PerPay invoice endpoint")
	}
	if merchantID == "" || apiID == "" || secret == "" || len(secret) > 4096 || strings.ContainsAny(apiID, "\r\n") {
		return nil, errors.New("PerPay invoice HMAC credentials are required")
	}
	return &PerPayInvoices{base: base, merchantID: merchantID, apiID: apiID, secret: secret, client: &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Create submits exactly the persisted buyer and platform order identity.
// It does not accept amount or seller overrides in the signed application.
// Recovery calls reuse the caller's original request ID and immutable buyer fields.
func (p *PerPayInvoices) Create(ctx context.Context, invoice *payment.TaxInvoice) (*payment.TaxInvoiceRemote, error) {
	// The merchant API uses camelCase buyer keys while the local buyer API uses
	// snake_case; project only persisted fields so amount and seller remain server-owned.
	application := struct {
		RequestID string `json:"requestId"` // persistent upstream request identity
		TradeNo   string `json:"tradeNo"`   // confirmed platform payment identity
		BuyerType string `json:"buyerType"` // immutable buyer legal classification
		Title     string `json:"title"`     // immutable customer invoice title
		TaxID     string `json:"taxId"`     // immutable organization tax identifier
		Email     string `json:"email"`     // immutable invoice delivery address
	}{invoice.RequestID, invoice.TradeNo, invoice.BuyerType, invoice.Title, invoice.TaxID, invoice.Email}
	reply, err := p.post(ctx, "applications", map[string]any{"operation": "starpay.invoice.create.v1", "merchantId": p.merchantID, "application": application})
	return reply.Invoice, err
}

// Query retrieves the original remote invoice record without initiating issuance.
// The payment service subsequently compares its stored identities and amount.
// A missing or malformed response remains unresolved and does not permit replacement.
func (p *PerPayInvoices) Query(ctx context.Context, id string) (*payment.TaxInvoiceRemote, error) {
	reply, err := p.post(ctx, "query", map[string]any{"operation": "starpay.invoice.query.v1", "merchantId": p.merchantID, "invoiceId": id})
	return reply.Invoice, err
}

// Files requests a fresh download link for the original remote invoice.
// The upstream checks issuance again, while the payment service validates HTTPS
// before publishing that temporary URL to the authenticated owning customer.
func (p *PerPayInvoices) Files(ctx context.Context, id string) (string, error) {
	reply, err := p.post(ctx, "files", map[string]any{"operation": "starpay.invoice.files.v1", "merchantId": p.merchantID, "invoiceId": id})
	return reply.URL, err
}

// post authenticates exact JSON bytes and validates the response account boundary.
// Only HTTP 200 or an unresolved creation HTTP 202 is accepted. Other responses
// return a safe fixed error without customer details or transport credentials.
//
// Redirects are disabled before signing so authentication headers cannot reach
// another destination. UTF-8 and body limits apply before decoding. The transport
// does not retry an uncertain request; the owning service uses its durable
// application identity to recover through the next scheduled observation.
func (p *PerPayInvoices) post(ctx context.Context, endpoint string, command any) (reply perPayInvoiceReply, err error) {
	unavailable := errors.New("PerPay invoice response unavailable")
	body, err := json.Marshal(command)
	if err != nil || len(body) > taxInvoiceRequestLimit {
		return perPayInvoiceReply{}, unavailable
	}
	// Sign Unix seconds, a newline and the exact transmitted JSON bytes because
	// re-encoding the body or changing timestamp units invalidates the merchant signature.
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(p.secret))
	_, _ = mac.Write([]byte(stamp + "\n"))
	_, _ = mac.Write(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base.ResolveReference(&url.URL{Path: endpoint}).String(), bytes.NewReader(body))
	if err != nil {
		return perPayInvoiceReply{}, unavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-lightpay-api-id", p.apiID)
	request.Header.Set("x-lightpay-timestamp", stamp)
	request.Header.Set("x-lightpay-signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response, err := p.client.Do(request)
	if err != nil {
		return perPayInvoiceReply{}, unavailable
	}
	// Close every response after validation, including rejected HTML or redirects.
	// A close failure keeps the outcome unresolved instead of publishing successful issuance.
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			reply = perPayInvoiceReply{}
			err = unavailable
		}
	}()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return perPayInvoiceReply{}, unavailable
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, taxInvoiceResponseLimit+1))
	if err != nil || len(content) > taxInvoiceResponseLimit || !utf8.Valid(content) {
		return perPayInvoiceReply{}, unavailable
	}
	var envelope struct {
		OK   bool               `json:"ok"`   // upstream operation acceptance flag
		Data perPayInvoiceReply `json:"data"` // merchant-bound operation response payload
	}
	if json.Unmarshal(content, &envelope) != nil || !envelope.OK || (response.StatusCode != 200 && response.StatusCode != 202) || envelope.Data.MerchantID != p.merchantID {
		return perPayInvoiceReply{}, unavailable
	}
	if response.StatusCode == 202 && (endpoint != "applications" || envelope.Data.Invoice == nil || (envelope.Data.Invoice.Status != "UNKNOWN" && envelope.Data.Invoice.Status != "PROCESSING")) {
		return perPayInvoiceReply{}, unavailable
	}
	return envelope.Data, nil
}
