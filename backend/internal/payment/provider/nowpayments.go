package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// nowpayments.go creates NOWPayments cryptocurrency payments through its REST
// API and authenticates its IPN callbacks before reporting payment progress.
// A created payment fixes one settlement asset, and the buyer transfers the
// quoted asset amount to the returned deposit address, so checkout exposes
// that address as the scannable payload and keeps the numeric payment
// identifier as the stored transaction reference for later queries.
//
// NOWPayments signs each IPN body with HMAC-SHA512 over a key-sorted JSON
// rendering, which the adapter reproduces from the received bytes before it
// trusts any field. A finished notification still has to agree with an
// independently queried payment state and fiat amount before fulfillment can
// run, following the same dual-confirmation rule as the other hosted
// adapters. Creation failures stay unconfirmed when the service may already
// have accepted the payment, because a repeated create call could quote a
// second deposit address for one local order.

// NOWPayments transport bounds and protocol defaults govern one adapter
// operation, its decoded response size, and the fallback API and asset codes.
const (
	nowPaymentsHTTPTimeout        = 15 * time.Second                // individual HTTP request budget
	nowPaymentsMaxBody            = 1 << 20                         // maximum response body bytes
	nowPaymentsDefaultAPIBase     = "https://api.nowpayments.io/v1" // production API prefix
	nowPaymentsDefaultPayCurrency = "usdttrc20"                     // default settlement asset and network
)

// NOWPayments payment lifecycle labels are the exact status strings returned
// by the payment API and IPN payloads; only finished settles an order.
const (
	nowPaymentsStatusWaiting       = "waiting"        // created, awaiting the buyer's transfer
	nowPaymentsStatusConfirming    = "confirming"     // transfer seen, awaiting network confirmations
	nowPaymentsStatusConfirmed     = "confirmed"      // transfer confirmed on chain
	nowPaymentsStatusSending       = "sending"        // settlement is being sent to the merchant wallet
	nowPaymentsStatusPartiallyPaid = "partially_paid" // received less than the quoted asset amount
	nowPaymentsStatusFinished      = "finished"       // settled to the merchant wallet
	nowPaymentsStatusFailed        = "failed"         // payment failed at the provider
	nowPaymentsStatusRefunded      = "refunded"       // payment was refunded to the buyer
	nowPaymentsStatusExpired       = "expired"        // payment window elapsed without a transfer
)

// nowPaymentsPayCurrencyPattern accepts the lowercase asset codes NOWPayments
// assigns to settlement currencies, such as usdttrc20 or btc.
var nowPaymentsPayCurrencyPattern = regexp.MustCompile(`^[a-z0-9]{2,32}$`)

// Adapter assertions keep checkout and merchant identity capabilities checked
// whenever the common payment interfaces change during application development.
var (
	_ payment.Provider                 = (*NOWPayments)(nil)
	_ payment.MerchantIdentityProvider = (*NOWPayments)(nil)
)

// NOWPayments implements payment creation, status queries, and IPN
// verification for one configured merchant account.
type NOWPayments struct {
	baseURL     string       // validated HTTPS API prefix
	apiKey      string       // account API key sent as a request header
	ipnSecret   string       // IPN signing secret for callback verification
	currency    string       // fiat price currency for created payments
	payCurrency string       // settlement asset code buyers transfer
	notifyURL   string       // configured callback destination fallback
	httpClient  *http.Client // shared bounded transport client
}

// nowPaymentsPayment decodes the payment object returned by creation, status
// queries, and IPN callbacks. Amounts keep their received number literals so
// later decimal comparisons do not inherit binary rounding.
type nowPaymentsPayment struct {
	PaymentID     int64       `json:"payment_id"`     // provider payment identity
	PaymentStatus string      `json:"payment_status"` // provider lifecycle label
	PayAddress    string      `json:"pay_address"`    // deposit address for the buyer transfer
	PriceAmount   json.Number `json:"price_amount"`   // fiat amount quoted at creation
	PriceCurrency string      `json:"price_currency"` // fiat currency of the quote
	PayAmount     json.Number `json:"pay_amount"`     // asset amount the buyer must transfer
	PayCurrency   string      `json:"pay_currency"`   // settlement asset code
	OrderID       string      `json:"order_id"`       // merchant order number echoed by the provider
}

// NewNOWPayments constructs an account adapter with a bounded HTTPS client.
// The API base defaults to the production prefix and must exclude user
// information, query parameters, and fragments. The fiat currency defaults
// through currency normalization and is limited to USD or CNY, matching the
// order currencies this deployment sells; the settlement asset defaults to
// USDT on TRON. Both the API key and the IPN secret are required because
// checkout creation and callback verification each fail closed without one.
func NewNOWPayments(_ string, config map[string]string) (*NOWPayments, error) {
	rawBase := strings.TrimRight(strings.TrimSpace(config["apiBase"]), "/")
	if rawBase == "" {
		rawBase = nowPaymentsDefaultAPIBase
	}
	base, err := url.Parse(rawBase)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("nowpayments apiBase must be an absolute HTTPS URL")
	}
	currency, err := payment.NormalizePaymentCurrency(config["currency"])
	if err != nil || (currency != "USD" && currency != "CNY") {
		return nil, fmt.Errorf("nowpayments currency must be USD or CNY")
	}
	payCurrency := strings.ToLower(strings.TrimSpace(config["payCurrency"]))
	if payCurrency == "" {
		payCurrency = nowPaymentsDefaultPayCurrency
	}
	if !nowPaymentsPayCurrencyPattern.MatchString(payCurrency) {
		return nil, fmt.Errorf("nowpayments payCurrency is not a recognized asset code")
	}
	notifyURL := strings.TrimSpace(config["notifyUrl"])
	if notifyURL != "" {
		parsedNotify, notifyErr := url.Parse(notifyURL)
		if notifyErr != nil || parsedNotify.Scheme != "https" || parsedNotify.Hostname() == "" ||
			parsedNotify.User != nil || parsedNotify.Fragment != "" {
			return nil, fmt.Errorf("nowpayments notifyUrl must be an absolute HTTPS URL")
		}
		notifyURL = parsedNotify.String()
	}
	apiKey := strings.TrimSpace(config["apiKey"])
	ipnSecret := strings.TrimSpace(config["ipnSecret"])
	if apiKey == "" || ipnSecret == "" {
		return nil, fmt.Errorf("nowpayments API key and IPN secret are required")
	}
	return &NOWPayments{
		baseURL:     strings.TrimRight(base.String(), "/"),
		apiKey:      apiKey,
		ipnSecret:   ipnSecret,
		currency:    currency,
		payCurrency: payCurrency,
		notifyURL:   notifyURL,
		httpClient:  &http.Client{Timeout: nowPaymentsHTTPTimeout},
	}, nil
}

// Name returns the payment adapter's administrator-facing product display name.
func (n *NOWPayments) Name() string { return "NOWPayments" }

// ProviderKey returns the persisted identity used for NOWPayments dispatch.
func (n *NOWPayments) ProviderKey() string { return payment.TypeNOWPayments }

// SupportedTypes returns a fresh collection containing the cryptocurrency checkout method.
func (n *NOWPayments) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeNOWPayments}
}

// MerchantIdentityMetadata returns price currency and settlement asset attributes for order validation.
func (n *NOWPayments) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"currency": n.currency, "pay_currency": n.payCurrency}
}

// CreatePayment quotes one fixed-asset payment and returns its deposit address.
// The request amount is the local order's fiat charge in the configured price
// currency, and the provider echoes it in later observations. The returned
// transaction reference is the provider payment identifier, which status
// queries and IPN callbacks reuse for the life of the order.
//
// A transport failure or a server-side error stays unconfirmed because the
// provider may already have stored the payment under this order number, and
// repeating creation could quote a second deposit address. Client errors are
// explicit rejections issued before any payment exists, so they return
// ordinary errors and let checkout release the local order.
func (n *NOWPayments) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	amount, err := easyPayPositiveMoney(req.Amount)
	if err != nil || req.OrderID == "" || len(req.OrderID) > 128 {
		return nil, fmt.Errorf("nowpayments checkout order or amount is invalid")
	}
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = n.notifyURL
	}
	body := map[string]any{
		"price_amount":   json.Number(amount.String()),
		"price_currency": strings.ToLower(n.currency),
		"pay_currency":   n.payCurrency,
		"order_id":       req.OrderID,
	}
	if req.Subject != "" {
		body["order_description"] = req.Subject
	}
	if notifyURL != "" {
		body["ipn_callback_url"] = notifyURL
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("nowpayments payment request could not be encoded")
	}
	status, data, err := n.doJSON(ctx, http.MethodPost, "/payment", encoded)
	if err != nil {
		// The create request may already have been accepted. Preserve the local
		// order for IPN recovery and keep transport text out of the caller's error.
		return nil, apperror.ServiceUnavailable(payment.CodePaymentCreateUnconfirmed, "NOWPayments payment creation could not be confirmed")
	}
	if status >= 500 {
		// A provider-side failure cannot prove the payment was not stored under
		// this order number. Keep the order recoverable instead of releasing it.
		return nil, apperror.ServiceUnavailable(payment.CodePaymentCreateUnconfirmed, "NOWPayments payment creation could not be confirmed")
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("nowpayments payment creation was rejected (HTTP %d)", status)
	}
	created, err := nowPaymentsDecodePayment(data)
	if err != nil || created.PaymentID <= 0 || created.PayAddress == "" {
		// A success envelope without a usable payment identity cannot be tracked
		// or queried later. Treat the creation as unconfirmed so the order stays
		// available for a signed IPN under the same order number.
		return nil, apperror.ServiceUnavailable(payment.CodePaymentCreateUnconfirmed, "NOWPayments payment creation could not be confirmed")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:  strconv.FormatInt(created.PaymentID, 10),
		QRCode:   created.PayAddress,
		Currency: n.currency,
	}, nil
}

// QueryOrder reads one payment by its stored numeric identifier. The provider
// repeats the fiat quote from creation, and that quote is the amount the
// runtime compares with the local order. Unrecognized lifecycle labels remain
// errors so an uncertain observation cannot be classified as unpaid.
func (n *NOWPayments) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	if !nowPaymentsPaymentReference(tradeNo) {
		return nil, fmt.Errorf("nowpayments payment reference is unavailable")
	}
	status, data, err := n.doJSON(ctx, http.MethodGet, "/payment/"+url.PathEscape(tradeNo), nil)
	if err != nil {
		return nil, fmt.Errorf("nowpayments payment query could not be completed")
	}
	if status == http.StatusNotFound {
		return nil, payment.ErrProviderOrderNotFound
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("nowpayments payment query was rejected (HTTP %d)", status)
	}
	observed, err := nowPaymentsDecodePayment(data)
	if err != nil || strconv.FormatInt(observed.PaymentID, 10) != tradeNo {
		return nil, fmt.Errorf("nowpayments payment query response could not be confirmed")
	}
	normalized, err := nowPaymentsNormalizeStatus(observed.PaymentStatus)
	if err != nil {
		return nil, err
	}
	amount, err := nowPaymentsPositiveAmount(observed.PriceAmount)
	if err != nil {
		return nil, fmt.Errorf("nowpayments payment amount is invalid")
	}
	amountFloat, _ := amount.Float64()
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Amount:   amountFloat,
		Status:   normalized,
		Metadata: n.MerchantIdentityMetadata(),
	}, nil
}

// VerifyNotification authenticates one IPN callback and reports its progress.
// The signature covers a key-sorted rendering of the received JSON object, so
// verification re-renders the parsed body in that canonical form and compares
// HMAC-SHA512 digests in constant time before any field is trusted. Payload
// currency and asset codes must also match this instance, which keeps a
// callback signed for a differently configured account from being accepted
// here when two instances share a dashboard.
//
// Only the finished label settles an order, and it still requires agreement
// with an independent status query: the queried payment must report the same
// finished state and the same fiat amount as the signed payload. Earlier
// lifecycle labels return pending or failed observations without that query,
// so routine transfer progress cannot trigger fulfillment on its own.
// @project-doc docs/domains/payments_and_entitlements.md#nowpayments_payments
func (n *NOWPayments) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	signature := strings.TrimSpace(headers["x-nowpayments-sig"])
	if signature == "" || !n.nowPaymentsValidSignature(rawBody, signature) {
		return nil, fmt.Errorf("nowpayments IPN signature is invalid")
	}
	payload, err := nowPaymentsDecodePayment([]byte(rawBody))
	if err != nil || payload.PaymentID <= 0 || payload.OrderID == "" ||
		!strings.EqualFold(payload.PriceCurrency, n.currency) ||
		!strings.EqualFold(payload.PayCurrency, n.payCurrency) {
		return nil, fmt.Errorf("nowpayments IPN payment fields are invalid")
	}
	amount, err := nowPaymentsPositiveAmount(payload.PriceAmount)
	if err != nil {
		return nil, fmt.Errorf("nowpayments IPN payment amount is invalid")
	}
	normalized, err := nowPaymentsNormalizeStatus(payload.PaymentStatus)
	if err != nil {
		return nil, err
	}
	tradeNo := strconv.FormatInt(payload.PaymentID, 10)
	if normalized == payment.ProviderStatusSuccess {
		queried, queryErr := n.QueryOrder(ctx, tradeNo)
		if queryErr != nil || queried.Status != payment.ProviderStatusSuccess ||
			!amount.Equal(decimal.NewFromFloat(queried.Amount)) {
			return nil, fmt.Errorf("nowpayments IPN does not match a finished payment")
		}
	}
	amountFloat, _ := amount.Float64()
	return &payment.PaymentNotification{
		OrderID: payload.OrderID, TradeNo: tradeNo, Amount: amountFloat,
		Status: normalized, RawData: rawBody,
		Metadata: n.MerchantIdentityMetadata(),
	}, nil
}

// Refund rejects automatic transfers because the NOWPayments API exposes no
// merchant refund operation for these payments. Administrators reconcile
// refunds from the provider dashboard; configuration prevents enabling
// automatic or buyer-initiated refunds for NOWPayments instances.
func (n *NOWPayments) Refund(_ context.Context, _ payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("nowpayments automatic refunds are not supported")
}

// doJSON performs one bounded API call with the account key header. It
// returns the HTTP status and response bytes for the caller's own success and
// rejection classification. Transport failures return an error without the
// request details, and redirects are followed by the shared client because
// the API documents none for these endpoints.
func (n *NOWPayments) doJSON(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, n.baseURL+path, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("nowpayments request could not be constructed")
	}
	request.Header.Set("x-api-key", n.apiKey)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := n.httpClient.Do(request)
	if err != nil {
		// Request headers carry the account key. Keep transport details out of
		// caller-visible errors while leaving the payment state unresolved.
		return 0, nil, fmt.Errorf("nowpayments request could not be completed")
	}
	// Release the response on every status and parsing path. A close failure
	// cannot establish payment and does not replace the observed result.
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, nowPaymentsMaxBody+1))
	if err != nil || len(data) > nowPaymentsMaxBody {
		return 0, nil, fmt.Errorf("nowpayments response could not be read")
	}
	return response.StatusCode, data, nil
}

// nowPaymentsValidSignature reports whether the supplied hex digest matches
// the HMAC-SHA512 of the canonical body under the configured IPN secret. A
// body that cannot be canonicalized and a digest that is not valid hex both
// fail verification instead of falling back to the raw bytes.
func (n *NOWPayments) nowPaymentsValidSignature(rawBody, signature string) bool {
	canonical, err := nowPaymentsCanonicalJSON(rawBody)
	if err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha512.New, []byte(n.ipnSecret))
	mac.Write(canonical)
	return hmac.Equal(provided, mac.Sum(nil))
}

// nowPaymentsCanonicalJSON renders one received JSON document with object
// keys sorted at every depth. NOWPayments signs that rendering of the payload
// rather than the transmitted byte order, so verification must reproduce it
// exactly. Numbers keep a normalized decimal form and strings use the same
// minimal escaping as the provider's serializer; arrays keep their order.
func nowPaymentsCanonicalJSON(raw string) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	// A second value after the first document means the body is not the single
	// signed object. Reject it instead of signing a prefix of the received text.
	if decoder.More() {
		return nil, fmt.Errorf("nowpayments IPN body has trailing data")
	}
	var out bytes.Buffer
	if err := nowPaymentsWriteCanonical(&out, value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// nowPaymentsWriteCanonical writes one decoded JSON value in canonical form.
// Object members are emitted in sorted key order so the result is independent
// of the sender's member order; scalar values use their JSON spellings.
func nowPaymentsWriteCanonical(out *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			nowPaymentsWriteJSONString(out, key)
			out.WriteByte(':')
			if err := nowPaymentsWriteCanonical(out, typed[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
		return nil
	case []any:
		out.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := nowPaymentsWriteCanonical(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
		return nil
	case string:
		nowPaymentsWriteJSONString(out, typed)
		return nil
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return fmt.Errorf("nowpayments IPN number is invalid")
		}
		// The provider's serializer prints amounts in shortest plain decimal
		// form. Reformatting the parsed value reproduces that spelling instead
		// of preserving insignificant zeros from the received literal.
		out.WriteString(strconv.FormatFloat(parsed, 'f', -1, 64))
		return nil
	case bool:
		if typed {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
		return nil
	case nil:
		out.WriteString("null")
		return nil
	default:
		return fmt.Errorf("nowpayments IPN value type is unsupported")
	}
}

// nowPaymentsWriteJSONString writes one JSON string with HTML escaping
// disabled, matching the provider serializer's treatment of callback URLs and
// descriptions that can contain angle brackets or ampersands.
func nowPaymentsWriteJSONString(out *bytes.Buffer, value string) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	// Encoding a string cannot fail; the trailing newline the encoder appends
	// is not part of the JSON value and is dropped before writing.
	_ = encoder.Encode(value)
	out.Write(bytes.TrimRight(encoded.Bytes(), "\n"))
}

// nowPaymentsDecodePayment parses one payment object while preserving amount
// literals. Malformed JSON returns an error for the caller's fail-closed path.
func nowPaymentsDecodePayment(data []byte) (*nowPaymentsPayment, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var parsed nowPaymentsPayment
	if err := decoder.Decode(&parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// nowPaymentsNormalizeStatus maps one provider lifecycle label to the shared
// provider status vocabulary. Only the finished label reports success, while
// in-flight and partially paid labels stay pending so amount checks keep
// their authority over fulfillment.
func nowPaymentsNormalizeStatus(status string) (string, error) {
	switch status {
	case nowPaymentsStatusFinished:
		return payment.ProviderStatusSuccess, nil
	case nowPaymentsStatusWaiting, nowPaymentsStatusConfirming,
		nowPaymentsStatusConfirmed, nowPaymentsStatusSending,
		nowPaymentsStatusPartiallyPaid:
		return payment.ProviderStatusPending, nil
	case nowPaymentsStatusFailed, nowPaymentsStatusExpired:
		return payment.ProviderStatusFailed, nil
	case nowPaymentsStatusRefunded:
		return payment.ProviderStatusRefunded, nil
	default:
		return "", fmt.Errorf("nowpayments payment status is unsupported")
	}
}

// nowPaymentsPositiveAmount parses one received amount literal as a finite
// positive decimal. Empty, malformed, and nonpositive values are rejected
// before any comparison with a local order can run.
func nowPaymentsPositiveAmount(literal json.Number) (decimal.Decimal, error) {
	if literal == "" {
		return decimal.Zero, fmt.Errorf("nowpayments amount is missing")
	}
	parsed, err := decimal.NewFromString(literal.String())
	if err != nil || !parsed.IsPositive() {
		return decimal.Zero, fmt.Errorf("nowpayments amount is invalid")
	}
	return parsed, nil
}

// nowPaymentsPaymentReference validates one stored payment identifier before
// it is placed in a query path. Identifiers are decimal integers issued by
// the provider, so any other text is rejected locally.
func nowPaymentsPaymentReference(value string) bool {
	if value == "" || len(value) > 20 {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
