package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// starpay.go implements StarPAY merchant balance checkout, signed notifications,
// and full refunds using the balance SDK's HMAC protocol. The receiving merchant
// uses its EPay PID and signing key, while the buyer authorizes spending inside
// StarPAY. Platform order IDs occupy the payment runtime's transaction reference;
// the separate business order number remains the durable local lookup key.
// Responses must identify the configured receiving merchant before they can
// supply any payment observation to the runtime.
//
// Notification authentication covers the original JSON bytes and timestamp.
// A fresh platform query must agree with the event before local order checks
// and idempotent fulfillment run. Creation retries reuse the identical payload.
// Refund submission uses an already persisted operation ID, and uncertain
// outcomes retain local preparation until a query confirms the full refund.

const (
	// Transport bounds limit remote work and memory, while the signature window
	// follows the supplied StarPAY balance protocol's five-minute replay tolerance.
	starPayTimeout        = 15 * time.Second // individual HTTP request budget
	starPaySignatureAge   = 300              // accepted clock difference seconds
	starPayMaxBody        = 1 << 20          // maximum JSON response bytes
	starPayMaxSafeInteger = 9007199254740991 // JavaScript exact integer ceiling
	starPayCreateAttempts = 2                // identical creation submission attempts
	starPayMaxCallbackID  = 256              // maximum delivery identifier bytes
)

var (
	// Protocol patterns reject ambiguous money, merchant numbers, and signature
	// encodings before external calls or cryptographic comparisons can affect orders.
	starPayMerchantOrder = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)
	starPayMoney         = regexp.MustCompile(`^(0|[1-9][0-9]{0,13})\.[0-9]{2}$`)
	starPayTimestamp     = regexp.MustCompile(`^[0-9]{10,11}$`)
	starPaySignature     = regexp.MustCompile(`^sha256=[a-f0-9]{64}$`)
	starPayPID           = regexp.MustCompile(`^[1-9][0-9]*$`)

	// Capability assertions keep refund recovery and merchant snapshot validation
	// available when the payment runtime's adapter contracts change during development.
	_ payment.RefundQueryProvider      = (*StarPay)(nil)
	_ payment.MerchantIdentityProvider = (*StarPay)(nil)
)

// StarPay connects one receiving merchant to the hosted balance checkout.
// @project-doc docs/domains/payments_and_entitlements.md#starpay_balance
type StarPay struct {
	baseURL    string           // validated HTTPS service prefix
	pid        int64            // receiving merchant numeric identity
	secret     string           // server-only EPay signing key
	notifyURL  string           // configured HTTPS notification destination
	httpClient *http.Client     // bounded redirect-rejecting HTTP client
	clock      func() time.Time // signature validation time source
}

// starPayOrder contains authoritative identity, money, and progress from StarPAY.
type starPayOrder struct {
	ID                 string `json:"id"`                 // platform payment order identity
	MerchantOrderID    string `json:"merchantOrderId"`    // receiving site's business number
	ReceiverMerchantID int64  `json:"receiverMerchantId"` // receiving merchant numeric identity
	AmountCents        int64  `json:"amountCents"`        // CNY cents
	Status             string `json:"status"`             // pending/paid/refunded/expired
}

// starPayReply carries the SDK's success envelope and optional checkout location.
type starPayReply struct {
	OK   bool `json:"ok"` // platform operation success flag
	Data struct {
		Order       starPayOrder `json:"order"`       // authoritative platform order observation
		CheckoutURL string       `json:"checkoutUrl"` // hosted payment authorization destination
	} `json:"data"` // successful operation response payload
}

// NewStarPay validates receiving credentials and constructs a bounded HTTPS adapter.
// Only CNY is supported. Callback URLs require HTTPS, and each creation request
// additionally checks that its return destination shares the notification origin.
func NewStarPay(_ string, config map[string]string) (*StarPay, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config["apiBase"]), "/")
	if baseURL == "" {
		baseURL = "https://star-web.top/pay"
	}
	base, err := starPayHTTPSURL(baseURL)
	if err != nil || base.RawQuery != "" || base.ForceQuery {
		return nil, fmt.Errorf("starpay apiBase must be an HTTPS URL without query parameters")
	}
	notify, err := starPayHTTPSURL(strings.TrimSpace(config["notifyUrl"]))
	if err != nil {
		return nil, fmt.Errorf("starpay notifyUrl must be an absolute HTTPS URL")
	}
	pidText := strings.TrimSpace(config["pid"])
	pid, err := strconv.ParseInt(pidText, 10, 64)
	if err != nil || !starPayPID.MatchString(pidText) || pid > starPayMaxSafeInteger || strings.TrimSpace(config["pkey"]) == "" {
		return nil, fmt.Errorf("starpay requires a positive EPay PID and signing key")
	}
	if currency := strings.ToUpper(strings.TrimSpace(config["currency"])); currency != "" && currency != "CNY" {
		return nil, fmt.Errorf("starpay merchant balance payments require CNY")
	}
	return &StarPay{
		baseURL: base.String(), pid: pid, secret: config["pkey"], notifyURL: notify.String(), clock: time.Now,
		httpClient: &http.Client{
			Timeout:       starPayTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Name returns the receiving balance adapter's administrator-facing product name.
func (s *StarPay) Name() string { return "StarPAY merchant balance" }

// ProviderKey returns the independent balance protocol identity for dispatch.
func (s *StarPay) ProviderKey() string { return payment.TypeStarPay }

// SupportedTypes returns the sole method exposed by this balance adapter.
func (s *StarPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStarPay}
}

// MerchantIdentityMetadata exposes receiving identity and currency for saved-order checks.
func (s *StarPay) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"pid": strconv.FormatInt(s.pid, 10), "currency": "CNY"}
}

// CreatePayment creates an idempotent StarPAY order and returns its hosted checkout.
// Callers supply a persisted business number and canonical two-decimal CNY amount.
// They must save the returned platform ID before exposing the checkout URL.
//
// A failed remote attempt can already have created an order. The bounded retry
// reuses all creation fields, including callback and return URLs, so the platform
// can recover the same order. An unconfirmed reply preserves the local draft;
// a later signed event can bind its verified platform ID during payment recovery.
func (s *StarPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	cents, err := starPayAmountCents(req.Amount)
	if err != nil || !starPayMerchantOrder.MatchString(req.OrderID) || strings.TrimSpace(req.Subject) == "" {
		return nil, fmt.Errorf("starpay business order, amount, or subject is invalid")
	}
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = s.notifyURL
	}
	notify, err := starPayHTTPSURL(notifyURL)
	if err != nil {
		return nil, fmt.Errorf("starpay notification URL is invalid")
	}
	if req.ReturnURL != "" {
		returnURL, parseErr := starPayHTTPSURL(req.ReturnURL)
		if parseErr != nil || !strings.EqualFold(returnURL.Host, notify.Host) {
			return nil, fmt.Errorf("starpay return URL must share the notification origin")
		}
	}
	payload := map[string]any{
		"operation": "starpay.balance.create.v1",
		"order": map[string]string{
			"merchantOrderId": req.OrderID, "amount": req.Amount, "subject": req.Subject,
			"notifyUrl": notifyURL, "returnUrl": req.ReturnURL,
		},
	}
	var reply *starPayReply
	// The gateway deduplicates an identical business order and creation payload.
	// Retrying cannot change its price or destinations after an ambiguous transport failure.
	for range starPayCreateAttempts {
		reply, err = s.request(ctx, "orders", payload)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return nil, apperror.ServiceUnavailable(payment.CodePaymentCreateUnconfirmed, "StarPAY order creation could not be confirmed")
	}
	order := reply.Data.Order
	checkout, parseErr := starPayHTTPSURL(reply.Data.CheckoutURL)
	base, _ := url.Parse(s.baseURL)
	if order.MerchantOrderID != req.OrderID || order.AmountCents != cents ||
		parseErr != nil || !strings.EqualFold(checkout.Host, base.Host) || order.Status != payment.ProviderStatusPending {
		return nil, apperror.ServiceUnavailable(payment.CodePaymentCreateUnconfirmed, "StarPAY checkout details could not be verified")
	}
	return &payment.CreatePaymentResponse{TradeNo: order.ID, PayURL: checkout.String(), Currency: "CNY"}, nil
}

// QueryOrder reads current progress using only a stored platform order ID.
// It rejects a response identifying another platform order. Returned metadata
// lets fulfillment compare the business number, receiving merchant, and exact cents.
func (s *StarPay) QueryOrder(ctx context.Context, orderID string) (*payment.QueryOrderResponse, error) {
	order, err := s.query(ctx, orderID)
	if err != nil {
		return nil, err
	}
	status := order.Status
	if status == "expired" {
		status = payment.ProviderStatusFailed
	}
	// Convert verified integer cents only at the shared runtime boundary, while
	// metadata retains the exact integer for comparison with the saved charge.
	return &payment.QueryOrderResponse{
		TradeNo: order.ID, Status: status, Amount: float64(order.AmountCents) / 100,
		Metadata: s.orderMetadata(order),
	}, nil
}

// VerifyNotification authenticates raw JSON and confirms current platform payment.
// The callback must have a fresh timestamp, canonical signature, and delivery ID.
// Only paid balance events can produce a fulfillment observation.
//
// The event identifies the platform query, whose receiving merchant, business
// number, amount, and current paid status must agree before returning. Local
// fulfillment subsequently checks the saved platform binding and exact charge.
// A late paid notification after a refund therefore remains unaccepted, and
// repeated valid deliveries rely on the runtime's persistent fulfillment deduplication.
func (s *StarPay) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	timestamp := headers["x-lightpay-timestamp"]
	signature := headers["x-lightpay-signature"]
	callbackID := headers["x-idempotency-key"]
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || !starPayTimestamp.MatchString(timestamp) || !starPaySignature.MatchString(signature) ||
		strings.TrimSpace(callbackID) == "" || len(callbackID) > starPayMaxCallbackID {
		return nil, fmt.Errorf("starpay callback signature headers are invalid")
	}
	// StarPAY permits five minutes of clock skew; older deliveries need a fresh
	// signed attempt so intercepted historical bodies cannot authorize new observations.
	age := s.clock().Unix() - seconds
	if age < -starPaySignatureAge || age > starPaySignatureAge {
		return nil, fmt.Errorf("starpay callback timestamp is outside the accepted window")
	}
	digest, _ := hex.DecodeString(signature[len("sha256="):])
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + rawBody))
	if !hmac.Equal(digest, mac.Sum(nil)) {
		return nil, fmt.Errorf("starpay callback signature mismatch")
	}
	var event struct {
		Event           string `json:"event"`           // signed event category
		Type            string `json:"type"`            // signed payment method
		OrderID         string `json:"orderId"`         // platform payment order identity
		MerchantOrderID string `json:"merchantOrderId"` // receiving site's business number
		Amount          string `json:"amount"`          // CNY decimal text
	}
	if err := json.Unmarshal([]byte(rawBody), &event); err != nil || event.Event != "payment.paid" || event.Type != "balance" {
		return nil, fmt.Errorf("starpay callback is not a paid balance event")
	}
	cents, err := starPayAmountCents(event.Amount)
	if err != nil {
		return nil, err
	}
	order, err := s.query(ctx, event.OrderID)
	if err != nil {
		return nil, err
	}
	if order.Status != payment.ProviderStatusPaid || order.MerchantOrderID != event.MerchantOrderID || order.AmountCents != cents {
		return nil, fmt.Errorf("starpay callback differs from the current paid order")
	}
	metadata := s.orderMetadata(order)
	metadata["callback_id"] = callbackID
	// Keep exact cents in metadata while adapting the verified amount to the
	// runtime's monetary field; local snapshot validation rejects any cent mismatch.
	return &payment.PaymentNotification{
		TradeNo: order.ID, OrderID: order.MerchantOrderID, Amount: float64(cents) / 100,
		Status: payment.NotificationStatusPaid, RawData: rawBody, Metadata: metadata,
	}, nil
}

// Refund returns the entire charge to the original paying merchant balance.
// RequestID must already identify a committed local refund preparation. A fresh
// query checks business identity and full amount before any refund is submitted.
//
// Once submission begins, every unverified outcome wraps ErrRefundOutcomeUnknown
// and retains its request ID for recovery. This prevents the workflow from
// restoring local rights when the remote transfer may already have completed.
// The SDK supplies no definitive rejection catalogue, so response errors remain
// uncertain until QueryRefund observes a matching refunded order.
func (s *StarPay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		return nil, fmt.Errorf("starpay refund requires a persisted request ID")
	}
	cents, err := starPayAmountCents(req.Amount)
	if err != nil {
		return nil, err
	}
	order, err := s.query(ctx, req.TradeNo)
	if err != nil {
		return nil, err
	}
	if order.MerchantOrderID != req.OrderID || order.AmountCents != cents {
		return nil, fmt.Errorf("starpay refunds require the original order and full amount")
	}
	if order.Status == payment.ProviderStatusRefunded {
		return &payment.RefundResponse{RefundID: req.RequestID, Status: payment.ProviderStatusSuccess}, nil
	}
	if order.Status != payment.ProviderStatusPaid {
		return nil, fmt.Errorf("starpay order is not currently paid")
	}
	reply, err := s.request(ctx, "refund", map[string]string{
		"operation": "starpay.balance.refund.v1", "orderId": req.TradeNo, "requestId": req.RequestID,
	})
	response := &payment.RefundResponse{RefundID: req.RequestID}
	if err != nil || reply.Data.Order.ID != req.TradeNo || reply.Data.Order.MerchantOrderID != req.OrderID ||
		reply.Data.Order.AmountCents != cents || reply.Data.Order.Status != payment.ProviderStatusRefunded {
		return response, fmt.Errorf("%w: StarPAY refund requires a status query", payment.ErrRefundOutcomeUnknown)
	}
	response.Status = payment.ProviderStatusSuccess
	return response, nil
}

// QueryRefund confirms a full refund through the original platform payment order.
// A paid or pending observation cannot prove the earlier refund attempt failed,
// so it preserves recovery instead of releasing the prepared local entitlement.
func (s *StarPay) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	cents, err := starPayAmountCents(req.Amount)
	if err != nil || req.Currency != "CNY" {
		return nil, fmt.Errorf("starpay refund query amount or currency is invalid")
	}
	order, err := s.query(ctx, req.TradeNo)
	if err != nil {
		return nil, err
	}
	if order.MerchantOrderID != req.OrderID || order.AmountCents != cents || order.Status != payment.ProviderStatusRefunded {
		return nil, fmt.Errorf("%w: StarPAY full refund is not confirmed", payment.ErrRefundOutcomeUnknown)
	}
	return &payment.RefundResponse{RefundID: req.RefundID, Status: payment.ProviderStatusSuccess}, nil
}

// query binds the platform response to the requested payment identity.
// Empty IDs fail locally; callers retain uncertainty when transport or identity
// validation fails because no replacement merchant-order query is available.
func (s *StarPay) query(ctx context.Context, orderID string) (*starPayOrder, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("starpay platform order ID is missing")
	}
	reply, err := s.request(ctx, "query", map[string]string{"operation": "starpay.balance.query.v1", "orderId": orderID})
	if err != nil {
		return nil, err
	}
	if reply.Data.Order.ID != orderID {
		return nil, fmt.Errorf("starpay query returned a different platform order")
	}
	return &reply.Data.Order, nil
}

// orderMetadata preserves authoritative integer money and both merchant identities.
func (s *StarPay) orderMetadata(order *starPayOrder) map[string]string {
	metadata := s.MerchantIdentityMetadata()
	metadata["merchant_order_id"] = order.MerchantOrderID
	metadata["amount_cents"] = strconv.FormatInt(order.AmountCents, 10)
	return metadata
}

// request signs and sends the exact JSON bytes for one balance operation.
// Redirects remain unaccepted, response bodies are bounded, and upstream errors
// omit response text and credentials. Successful envelopes must contain a valid
// order owned by the configured receiving merchant before callers use its fields.
//
// The bytes submitted to HMAC are the same bytes sent on the connection, so JSON
// escaping and property order cannot separate signing from transmission. The
// operation name stays inside that signed body and distinguishes creation,
// payment lookup, and refund authorization at the gateway.
func (s *StarPay) request(ctx context.Context, path string, payload any) (*starPayReply, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("starpay request could not be encoded")
	}
	timestamp := strconv.FormatInt(s.clock().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write(append([]byte(timestamp+"\n"), body...))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/epay/balance/"+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("starpay request could not be constructed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-starpay-pid", strconv.FormatInt(s.pid, 10))
	req.Header.Set("x-starpay-timestamp", timestamp)
	req.Header.Set("x-starpay-signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("starpay %s transport failed", path)
	}
	// Release every bounded response body, including rejected HTTP responses, so
	// repeated queries do not retain transport resources after their operation ends.
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("starpay %s failed (HTTP %d)", path, response.StatusCode)
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, starPayMaxBody+1))
	if err != nil || len(encoded) > starPayMaxBody {
		return nil, fmt.Errorf("starpay response could not be read within its size limit")
	}
	var reply starPayReply
	if err := json.Unmarshal(encoded, &reply); err != nil || !reply.OK {
		return nil, fmt.Errorf("starpay response did not confirm success")
	}
	order := reply.Data.Order
	validOrderID := order.ID != "" && strings.TrimSpace(order.ID) == order.ID && !strings.ContainsAny(order.ID, "\x00\r\n\t")
	validAmount := order.AmountCents > 0 && order.AmountCents <= starPayMaxSafeInteger
	if !validOrderID || !validAmount || order.ReceiverMerchantID != s.pid || !starPayMerchantOrder.MatchString(order.MerchantOrderID) {
		return nil, fmt.Errorf("starpay response order identity or amount is invalid")
	}
	switch order.Status {
	case payment.ProviderStatusPending, payment.ProviderStatusPaid, payment.ProviderStatusRefunded, "expired":
		return &reply, nil
	default:
		return nil, fmt.Errorf("starpay response order status is unsupported")
	}
}

// starPayAmountCents accepts canonical positive CNY money within JavaScript's integer range.
// Exponents, signs, fractional cents, and surrounding whitespace are rejected so
// callback text and persisted payment cents have a single unambiguous representation.
func starPayAmountCents(amount string) (int64, error) {
	if !starPayMoney.MatchString(amount) {
		return 0, fmt.Errorf("starpay amount must use two decimal places")
	}
	cents, err := payment.AmountToMinorUnit(amount, "CNY")
	if err != nil || cents <= 0 || cents > starPayMaxSafeInteger {
		return 0, fmt.Errorf("starpay amount is outside the supported range")
	}
	return cents, nil
}

// starPayHTTPSURL validates an absolute HTTPS destination without embedded credentials.
// Fragments and opaque URLs are rejected; query parameters remain available for
// signed browser-return references and hosted checkout tokens supplied by StarPAY.
// An explicit default port is removed so equivalent HTTPS origins compare equally.
func starPayHTTPSURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("starpay destination must be an absolute HTTPS URL")
	}
	// Browsers treat explicit HTTPS port 443 as the default origin. Match that
	// representation before comparing return and notification destinations for the same site.
	if parsed.Port() == "443" {
		parsed.Host = strings.TrimSuffix(parsed.Host, ":443")
	}
	return parsed, nil
}
