package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// tax_invoices.go persists one buyer application per payment order and leases
// recovery work without keeping database transactions open during provider calls.
// Reservation locks the owned payment row before checking current financial
// approval and inserting its immutable snapshot. Concurrent identical submissions
// return that original record, while changed buyer or merchant details fail.
// The migration's order trigger shares this lock with refund transitions.
//
// Recovery claims return the complete current record together with the database
// lease timestamp. Publication checks that lease and every immutable identity
// before releasing the claim, so a delayed worker cannot overwrite a successor.
// Failed publication leaves the durable application available for later recovery;
// no persistence method generates a replacement request or contacts the provider.

const (
	// These column lists keep scan order shared across reads and claim returns,
	// while explicit projections prevent schema additions from changing decoding.
	taxInvoiceColumns = `id,order_id,user_id,request_id,trade_no,amount_cents,buyer_type,title,tax_id,email,
		endpoint,merchant_id,epay_pid,provider_instance_id,status,remote_id,invoice_no,error_code,
		updated_at,next_sync_at,lease_until`
	taxInvoiceOrderSQL = `SELECT p.id,p.user_id,p.pay_amount,p.payment_type,p.payment_trade_no,p.provider_instance_id,
		p.provider_key,p.provider_snapshot,p.status,p.refund_amount,p.refund_at,p.refund_requested_at,p.paid_at,
		EXISTS(SELECT 1 FROM payment_tax_invoice_eligibility e WHERE e.order_id=p.id AND e.revoked_at IS NULL)
		FROM payment_orders p WHERE p.id=$1 AND p.user_id=$2`
)

// TaxInvoiceStore owns SQL application reservations and bounded worker leases.
type TaxInvoiceStore struct {
	// db remains owned by application assembly and must outlive every store call.
	db *sql.DB
}

// taxInvoiceScanner decodes the same projection from single or multiple SQL rows.
type taxInvoiceScanner interface {
	// Scan copies the projected fields into destinations and reports conversion
	// failures. Its implementation retains ownership of the underlying SQL rows.
	Scan(...any) error
}

// NewTaxInvoiceStore retains the caller-owned pool for invoice persistence.
// Assembly must apply the invoice migration before calling any method and close
// the pool only after invoice workers finish their bounded shutdown.
func NewTaxInvoiceStore(db *sql.DB) *TaxInvoiceStore {
	return &TaxInvoiceStore{db: db}
}

// Read returns payment facts and an optional application for the given owner.
// Missing and foreign orders share the same not-found error; invoice details are
// read only after that ownership predicate succeeds, including when issuance is disabled.
func (s *TaxInvoiceStore) Read(ctx context.Context, orderID, userID int64) (*payment.TaxInvoiceOrder, *payment.TaxInvoice, error) {
	order, err := scanTaxInvoiceOrder(s.db.QueryRowContext(ctx, taxInvoiceOrderSQL, orderID, userID))
	if err != nil {
		return nil, nil, err
	}
	invoice, err := scanTaxInvoice(s.db.QueryRowContext(ctx, "SELECT "+taxInvoiceColumns+" FROM payment_tax_invoices WHERE order_id=$1 AND user_id=$2", orderID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return order, nil, nil
	}
	return order, invoice, err
}

// Reserve commits the first immutable application while holding its payment row.
// The validation callback reads current payment and financial approval inside
// that transaction and must not perform network operations or retain the row.
//
// An existing application is compared before new-application validation because
// later approval revocation cannot erase the original request identity. Matching
// submissions return its saved request ID, regardless of the candidate ID; changed
// buyer, order, amount, or merchant snapshots fail without writing. Rollback runs
// on every precommit error, and only a successful commit publishes a new record.
func (s *TaxInvoiceStore) Reserve(ctx context.Context, invoice *payment.TaxInvoice, validate func(*payment.TaxInvoiceOrder) error) (*payment.TaxInvoice, error) {
	if invoice == nil || validate == nil {
		return nil, errors.New("tax invoice reservation requires an application and validator")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tax invoice reservation: %w", err)
	}
	// Keep the order lock through validation and insertion. Any failed step rolls
	// back the unpublished application before a caller can send it to the provider.
	defer func() { _ = tx.Rollback() }()
	order, err := scanTaxInvoiceOrder(tx.QueryRowContext(ctx, taxInvoiceOrderSQL+" FOR UPDATE OF p", invoice.OrderID, invoice.UserID))
	if err != nil {
		return nil, err
	}
	existing, err := scanTaxInvoice(tx.QueryRowContext(ctx, "SELECT "+taxInvoiceColumns+" FROM payment_tax_invoices WHERE order_id=$1", invoice.OrderID))
	if err == nil {
		if !sameTaxInvoiceApplication(existing, invoice) {
			return nil, taxInvoiceConflict()
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read reserved tax invoice: %w", err)
	}
	if err = validate(order); err != nil {
		return nil, err
	}
	// The trigger freezes the approval reference under the same order lock and
	// independently rejects refund or ownership changes before this reservation commits.
	reserved, err := scanTaxInvoice(tx.QueryRowContext(ctx, `INSERT INTO payment_tax_invoices
		(id,order_id,user_id,request_id,trade_no,amount_cents,buyer_type,title,tax_id,email,
		endpoint,merchant_id,epay_pid,provider_instance_id,status,updated_at,next_sync_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'UNKNOWN',$15,$15)
		RETURNING `+taxInvoiceColumns,
		invoice.ID, invoice.OrderID, invoice.UserID, invoice.RequestID, invoice.TradeNo, invoice.AmountCents,
		invoice.BuyerType, invoice.Title, invoice.TaxID, invoice.Email, invoice.Endpoint, invoice.MerchantID,
		invoice.EpayPID, invoice.ProviderInstanceID, invoice.UpdatedAt.UTC().Truncate(time.Microsecond)))
	if err != nil {
		return nil, fmt.Errorf("reserve tax invoice: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tax invoice reservation: %w", err)
	}
	return reserved, nil
}

// Claim atomically leases a due nonterminal record and returns its latest fields.
// False means the row is absent, terminal, not due, or already leased; it does not
// authorize a provider request. The returned lease timestamp must accompany Save.
//
// UPDATE evaluates eligibility against the current row, so a stale Due result
// cannot obtain a second active claim. Returning the full projection also prevents
// the caller from publishing data read before another worker's completed update.
func (s *TaxInvoiceStore) Claim(ctx context.Context, id string, now time.Time) (*payment.TaxInvoice, bool, error) {
	// PostgreSQL timestamps use microseconds; returning the stored value avoids
	// equality failures when the caller's original clock has finer precision.
	stamp := now.UTC().Truncate(time.Microsecond)
	invoice, err := scanTaxInvoice(s.db.QueryRowContext(ctx, `UPDATE payment_tax_invoices
		SET lease_until=$2 + interval '90 seconds'
		WHERE id=$1 AND status IN ('UNKNOWN','PROCESSING','REVERSING')
		AND next_sync_at<=$2 AND lease_until<=$2 RETURNING `+taxInvoiceColumns, id, stamp))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("claim tax invoice: %w", err)
	}
	return invoice, true, nil
}

// Save publishes one observation only while its exact worker lease remains active.
// Callers preserve every immutable application field and supply the next recovery
// time. Database transition checks reject contradictory remote identities and
// terminal-state regressions even when a worker presents a valid lease.
//
// A conditional update compares the whole immutable association as well as the
// lease. Zero affected rows therefore returns conflict, leaving a successor's
// observation intact. Successful publication clears the persisted lease so the
// next due cycle can recover transient failures without replacing the request.
func (s *TaxInvoiceStore) Save(ctx context.Context, invoice *payment.TaxInvoice) error {
	if invoice == nil {
		return errors.New("tax invoice observation is required")
	}
	updated, err := s.db.ExecContext(ctx, `UPDATE payment_tax_invoices SET
		status=$2,remote_id=$3,invoice_no=$4,error_code=$5,updated_at=$6,next_sync_at=$7,lease_until='epoch'
		WHERE id=$1 AND lease_until=$8 AND lease_until>clock_timestamp()
		AND order_id=$9 AND user_id=$10 AND request_id=$11 AND trade_no=$12 AND amount_cents=$13
		AND buyer_type=$14 AND title=$15 AND tax_id=$16 AND email=$17 AND endpoint=$18
		AND merchant_id=$19 AND epay_pid=$20 AND provider_instance_id=$21`,
		invoice.ID, invoice.Status, invoice.RemoteID, invoice.InvoiceNo, invoice.ErrorCode,
		invoice.UpdatedAt.UTC(), invoice.NextSyncAt.UTC(), invoice.LeaseUntil.UTC(),
		invoice.OrderID, invoice.UserID, invoice.RequestID, invoice.TradeNo, invoice.AmountCents,
		invoice.BuyerType, invoice.Title, invoice.TaxID, invoice.Email, invoice.Endpoint,
		invoice.MerchantID, invoice.EpayPID, invoice.ProviderInstanceID)
	if err != nil {
		return fmt.Errorf("save tax invoice observation: %w", err)
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("count tax invoice observation: %w", err)
	}
	if count != 1 {
		return taxInvoiceConflict()
	}
	return nil
}

// Due returns up to one hundred scheduled records for the current merchant binding.
// The query filters before pagination so older suspended bindings cannot starve
// this deployment's recovery queue. Reading does not reserve a lease; callers
// still Claim each record before provider work, and invalid limits fail locally.
func (s *TaxInvoiceStore) Due(ctx context.Context, binding payment.TaxInvoiceConfig, now time.Time, limit int) (invoices []*payment.TaxInvoice, err error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("tax invoice recovery limit must be between 1 and 100")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+taxInvoiceColumns+` FROM payment_tax_invoices
		WHERE status IN ('UNKNOWN','PROCESSING','REVERSING') AND next_sync_at<=$1 AND lease_until<=$1
		AND endpoint=$2 AND merchant_id=$3 AND epay_pid=$4 AND provider_instance_id=$5
		ORDER BY next_sync_at,id LIMIT $6`, now.UTC(), binding.Endpoint, binding.MerchantID, binding.EpayPID, binding.ProviderInstanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("read due tax invoices: %w", err)
	}
	// Report close errors alongside iteration errors so a failed database stream
	// cannot look like a complete recovery page to the worker scheduling it.
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close due tax invoices: %w", closeErr))
		}
	}()
	invoices = make([]*payment.TaxInvoice, 0, limit)
	for rows.Next() {
		invoice, err := scanTaxInvoice(rows)
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, invoice)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due tax invoices: %w", err)
	}
	return invoices, nil
}

// scanTaxInvoice decodes one complete application and its persisted worker state.
// It returns SQL row errors unchanged so callers can distinguish an absent
// application from a failure that must stop dispatch or publication.
func scanTaxInvoice(row taxInvoiceScanner) (*payment.TaxInvoice, error) {
	invoice := &payment.TaxInvoice{}
	err := row.Scan(&invoice.ID, &invoice.OrderID, &invoice.UserID, &invoice.RequestID, &invoice.TradeNo,
		&invoice.AmountCents, &invoice.BuyerType, &invoice.Title, &invoice.TaxID, &invoice.Email,
		&invoice.Endpoint, &invoice.MerchantID, &invoice.EpayPID, &invoice.ProviderInstanceID,
		&invoice.Status, &invoice.RemoteID, &invoice.InvoiceNo, &invoice.ErrorCode,
		&invoice.UpdatedAt, &invoice.NextSyncAt, &invoice.LeaseUntil)
	if err != nil {
		return nil, err
	}
	return invoice, nil
}

// scanTaxInvoiceOrder preserves the provider snapshot while decoding eligibility facts.
// Invalid stored JSON fails the read rather than supplying empty merchant or
// currency attributes. Missing rows share the authenticated order not-found response.
func scanTaxInvoiceOrder(row taxInvoiceScanner) (*payment.TaxInvoiceOrder, error) {
	order := &payment.Order{}
	var snapshot []byte
	var approved bool
	err := row.Scan(&order.ID, &order.UserID, &order.PayAmount, &order.PaymentType, &order.PaymentTradeNo,
		&order.ProviderInstanceID, &order.ProviderKey, &snapshot, &order.Status, &order.RefundAmount,
		&order.RefundAt, &order.RefundRequestedAt, &order.PaidAt, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("NOT_FOUND", "order not found")
	}
	if err != nil {
		return nil, fmt.Errorf("read tax invoice order: %w", err)
	}
	if len(snapshot) > 0 {
		if err = json.Unmarshal(snapshot, &order.ProviderSnapshot); err != nil {
			return nil, errors.New("invalid persisted payment provider snapshot")
		}
	}
	return &payment.TaxInvoiceOrder{Order: order, Approved: approved}, nil
}

// sameTaxInvoiceApplication compares buyer and routing facts for duplicate submissions.
// Candidate local and request IDs are intentionally excluded: a second browser
// submission receives the first persisted identity when all business facts match.
func sameTaxInvoiceApplication(a, b *payment.TaxInvoice) bool {
	return a.OrderID == b.OrderID && a.UserID == b.UserID && a.TradeNo == b.TradeNo &&
		a.AmountCents == b.AmountCents && a.BuyerType == b.BuyerType && a.Title == b.Title &&
		a.TaxID == b.TaxID && a.Email == b.Email && a.Endpoint == b.Endpoint &&
		a.MerchantID == b.MerchantID && a.EpayPID == b.EpayPID && a.ProviderInstanceID == b.ProviderInstanceID
}

// taxInvoiceConflict returns the safe error for changed application or lease identity.
func taxInvoiceConflict() error {
	return apperror.Conflict("TAX_INVOICE_CONFLICT", "invoice application changed; refresh its saved state")
}
