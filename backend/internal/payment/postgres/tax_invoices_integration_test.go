//go:build integration

package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	paymentprovider "github.com/TokenFlux/TokenRouter/internal/payment/provider"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// tax_invoices_integration_test.go exercises invoice reservation and refund
// exclusion in a dedicated PostgreSQL schema. An explicit loopback test DSN
// selects a disposable server; each test creates a unique schema and removes
// only that schema after closing its scoped connection pool.

// TestTaxInvoicePostgresRecoveryAndRefunds applies the new migration twice and
// exercises real row locks. Immutable application identity, approval evidence,
// competing refunds, and stale lease publication must survive concurrent callers.
func TestTaxInvoicePostgresRecoveryAndRefunds(t *testing.T) {
	db := taxInvoicePostgres(t)
	store := NewTaxInvoiceStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	seedTaxInvoiceOrder(t, db, 1)
	application := taxInvoiceFixture(1)
	first, err := store.Reserve(ctx, application, approveTaxInvoiceFixture)
	require.NoError(t, err)
	application.ID = uuid.NewString()
	application.RequestID = uuid.NewString()
	again, err := store.Reserve(ctx, application, approveTaxInvoiceFixture)
	require.NoError(t, err)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.RequestID, again.RequestID)
	application.Title = "Another buyer"
	_, err = store.Reserve(ctx, application, approveTaxInvoiceFixture)
	require.Equal(t, "TAX_INVOICE_CONFLICT", apperror.Reason(err))

	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoice_eligibility SET reference='later decision',approved_by='another operator',revoked_at=now() WHERE order_id=1`)
	require.NoError(t, err)
	var reference string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT approval_snapshot->>'reference' FROM payment_tax_invoices WHERE order_id=1`).Scan(&reference))
	require.Equal(t, "confirmed-sale-1", reference)
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET title='changed' WHERE order_id=1`)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `UPDATE payment_orders SET status='REFUNDING' WHERE id=1`)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `UPDATE payment_orders SET status='PARTIALLY_REFUNDED' WHERE id=1`)
	require.Error(t, err)

	claimed, ok, err := store.Claim(ctx, first.ID, time.Now())
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = store.Claim(ctx, first.ID, time.Now())
	require.NoError(t, err)
	require.False(t, ok)
	// Expire the first claim directly to model a stopped worker. A replacement
	// receives a different database lease before the older observation can return.
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET lease_until='epoch' WHERE id=$1`, first.ID)
	require.NoError(t, err)
	replacement, ok, err := store.Claim(ctx, first.ID, time.Now().Add(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	claimed.Status = "PROCESSING"
	claimed.UpdatedAt = time.Now()
	claimed.NextSyncAt = time.Now().Add(30 * time.Second)
	require.Equal(t, "TAX_INVOICE_CONFLICT", apperror.Reason(store.Save(ctx, claimed)))
	replacement.Status = "ISSUED"
	replacement.RemoteID = uuid.NewString()
	replacement.InvoiceNo = "12345678901234567890"
	replacement.UpdatedAt = time.Now()
	replacement.NextSyncAt = time.Now().Add(30 * time.Second)
	require.NoError(t, store.Save(ctx, replacement))
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET status='UNKNOWN',invoice_no='' WHERE id=$1`, first.ID)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET status='REVERSED' WHERE id=$1`, first.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE payment_orders SET status='REFUNDING' WHERE id=1`)
	require.NoError(t, err)

	t.Run("revoked approval and wrong money", func(t *testing.T) {
		seedTaxInvoiceOrder(t, db, 4)
		_, err := db.ExecContext(ctx, `UPDATE payment_tax_invoice_eligibility SET revoked_at=now() WHERE order_id=4`)
		require.NoError(t, err)
		_, err = store.Reserve(ctx, taxInvoiceFixture(4), func(*payment.TaxInvoiceOrder) error { return nil })
		require.Error(t, err)
		seedTaxInvoiceOrder(t, db, 5)
		wrongAmount := taxInvoiceFixture(5)
		wrongAmount.AmountCents = 100
		_, err = store.Reserve(ctx, wrongAmount, approveTaxInvoiceFixture)
		require.Error(t, err)
		_, err = store.Reserve(ctx, taxInvoiceFixture(5), approveTaxInvoiceFixture)
		require.NoError(t, err)
		binding := payment.TaxInvoiceConfig{Endpoint: "https://invoice.example.test/", MerchantID: "main-1", EpayPID: "1001", ProviderInstanceID: "9"}
		due, err := store.Due(ctx, binding, time.Now().Add(time.Second), 20)
		require.NoError(t, err)
		require.Len(t, due, 1)
		require.Equal(t, int64(5), due[0].OrderID)
	})

	t.Run("refund wins", func(t *testing.T) {
		seedTaxInvoiceOrder(t, db, 2)
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		// Cleanup also handles an assertion before commit; an already completed
		// transaction needs no rollback, but any other rollback failure remains visible.
		defer func() {
			rollbackErr := tx.Rollback()
			if !errors.Is(rollbackErr, sql.ErrTxDone) {
				require.NoError(t, rollbackErr)
			}
		}()
		_, err = tx.ExecContext(ctx, `UPDATE payment_orders SET status='REFUNDING' WHERE id=2`)
		require.NoError(t, err)
		finished := make(chan error, 1)
		go func() {
			_, reserveErr := store.Reserve(ctx, taxInvoiceFixture(2), approveTaxInvoiceFixture)
			finished <- reserveErr
		}()
		select {
		case err = <-finished:
			t.Fatalf("reservation escaped payment lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		require.NoError(t, tx.Commit())
		require.Error(t, <-finished)
	})

	t.Run("invoice wins", func(t *testing.T) {
		seedTaxInvoiceOrder(t, db, 3)
		locked := make(chan struct{})
		release := make(chan struct{})
		reserved := make(chan error, 1)
		go func() {
			_, reserveErr := store.Reserve(ctx, taxInvoiceFixture(3), func(order *payment.TaxInvoiceOrder) error {
				close(locked)
				<-release
				return approveTaxInvoiceFixture(order)
			})
			reserved <- reserveErr
		}()
		<-locked
		refunded := make(chan error, 1)
		go func() {
			_, refundErr := db.ExecContext(ctx, `UPDATE payment_orders SET refund_requested_at=now() WHERE id=3`)
			refunded <- refundErr
		}()
		// Reservation owns the payment row before validation returns. The refund
		// must wait, then inspect the committed invoice before changing payment state.
		select {
		case err = <-refunded:
			close(release)
			t.Fatalf("refund escaped invoice lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		require.NoError(t, <-reserved)
		require.Error(t, <-refunded)
	})

	t.Run("confirmed failure releases refund", func(t *testing.T) {
		seedTaxInvoiceOrder(t, db, 6)
		invoice, err := store.Reserve(ctx, taxInvoiceFixture(6), approveTaxInvoiceFixture)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE payment_orders SET status='REFUNDING' WHERE id=6`)
		require.Error(t, err)
		_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET status='FAILED',error_code='wechat_issue_failed' WHERE id=$1`, invoice.ID)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE payment_orders SET status='REFUNDING' WHERE id=6`)
		require.NoError(t, err)
		seedTaxInvoiceOrder(t, db, 7)
		invoice, err = store.Reserve(ctx, taxInvoiceFixture(7), approveTaxInvoiceFixture)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET status='FAILED',error_code='unknown_provider_failure' WHERE id=$1`, invoice.ID)
		require.Error(t, err)
		_, err = db.ExecContext(ctx, `UPDATE payment_orders SET refund_requested_at=now() WHERE id=7`)
		require.Error(t, err)
	})
}

// TestTaxInvoicePostgresDueUsesCurrentBinding queues older applications under a
// previous endpoint before the current merchant's application. A bounded page
// must admit the current binding instead of repeatedly returning suspended work.
func TestTaxInvoicePostgresDueUsesCurrentBinding(t *testing.T) {
	db := taxInvoicePostgres(t)
	store := NewTaxInvoiceStore(db)
	ctx := context.Background()
	for orderID := int64(1); orderID <= 21; orderID++ {
		seedTaxInvoiceOrder(t, db, orderID)
		invoice := taxInvoiceFixture(orderID)
		if orderID <= 20 {
			invoice.Endpoint = "https://previous.example.test/"
		}
		_, err := store.Reserve(ctx, invoice, approveTaxInvoiceFixture)
		require.NoError(t, err)
	}
	binding := payment.TaxInvoiceConfig{Endpoint: "https://invoice.example.test/", MerchantID: "main-1", EpayPID: "1001", ProviderInstanceID: "9"}
	due, err := store.Due(ctx, binding, time.Now().Add(time.Second), 20)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, int64(21), due[0].OrderID)
	for _, field := range []string{"merchant", "pid", "instance"} {
		other := binding
		switch field {
		case "merchant":
			other.MerchantID = "other"
		case "pid":
			other.EpayPID = "other"
		case "instance":
			other.ProviderInstanceID = "other"
		}
		due, err = store.Due(ctx, other, time.Now().Add(time.Second), 20)
		require.NoError(t, err)
		require.Empty(t, due, field)
	}
}

// TestTaxInvoicePostgresSignedLifecycle combines the real store, domain service,
// and TLS transport. An accepted request with a lost outcome must recover its
// original identity, and only the owning buyer can read or download the invoice.
func TestTaxInvoicePostgresSignedLifecycle(t *testing.T) {
	db := taxInvoicePostgres(t)
	seedTaxInvoiceOrder(t, db, 41)
	store := NewTaxInvoiceStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const secret = "synthetic-invoice-lifecycle-secret"
	const remoteID = "00000000-0000-4000-8000-000000000041"
	const queryRemoteID = "00000000-0000-4000-8000-000000000042"
	const downloadURL = "https://invoice.example.test/file?token=temporary-fixture"
	var createCalls, queryCalls, fileCalls atomic.Int32
	var requestMu sync.Mutex
	var firstBody []byte
	var remote payment.TaxInvoiceRemote
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(r.Header.Get("x-lightpay-timestamp") + "\n"))
		_, _ = mac.Write(body)
		assert.Equal(t, "sha256="+hex.EncodeToString(mac.Sum(nil)), r.Header.Get("x-lightpay-signature"))
		assert.Equal(t, "api-1", r.Header.Get("x-lightpay-api-id"))
		assert.Equal(t, http.MethodPost, r.Method)
		var command struct {
			Operation   string            `json:"operation"`   // signed operation purpose
			MerchantID  string            `json:"merchantId"`  // main-site merchant identity
			Application map[string]string `json:"application"` // original immutable buyer request
			InvoiceID   string            `json:"invoiceId"`   // saved upstream record identity
		}
		if !assert.NoError(t, json.Unmarshal(body, &command)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "main-1", command.MerchantID)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tax/applications":
			call := createCalls.Add(1)
			assert.Equal(t, "starpay.invoice.create.v1", command.Operation)
			var count int
			var savedID, savedTitle, savedStatus string
			var savedAmount int64
			err = db.QueryRowContext(r.Context(), `SELECT count(*) FROM payment_tax_invoices WHERE order_id=41`).Scan(&count)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
			err = db.QueryRowContext(r.Context(), `SELECT request_id,title,status,amount_cents FROM payment_tax_invoices WHERE order_id=41`).Scan(&savedID, &savedTitle, &savedStatus, &savedAmount)
			assert.NoError(t, err)
			assert.Equal(t, command.Application["requestId"], savedID)
			assert.Equal(t, command.Application["title"], savedTitle)
			assert.Equal(t, "UNKNOWN", savedStatus)
			assert.Equal(t, int64(1000), savedAmount)
			assert.Equal(t, "paid-trade-41", command.Application["tradeNo"])
			assert.Equal(t, "ORGANIZATION", command.Application["buyerType"])
			assert.Equal(t, "91310000987654321B", command.Application["taxId"])
			assert.Equal(t, "invoice@example.test", command.Application["email"])
			// The provider records the first accepted request before returning 503.
			// Recovery must preserve every original signed buyer field and request ID.
			requestMu.Lock()
			if call == 1 {
				firstBody = append([]byte(nil), body...)
				remote = payment.TaxInvoiceRemote{ID: remoteID, RequestID: savedID, TradeNo: command.Application["tradeNo"], Title: savedTitle, AmountCents: savedAmount, Status: "ISSUED", InvoiceNo: "12345678901234567890"}
			} else {
				assert.Equal(t, firstBody, body)
			}
			observation := remote
			requestMu.Unlock()
			if call == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"ok":false,"code":"invoice_unavailable"}`)
				return
			}
			err = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"merchantId": "main-1", "invoice": observation}})
			assert.NoError(t, err)
		case "/tax/query":
			queryCalls.Add(1)
			assert.Equal(t, "starpay.invoice.query.v1", command.Operation)
			assert.Equal(t, queryRemoteID, command.InvoiceID)
			requestMu.Lock()
			observation := remote
			requestMu.Unlock()
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"merchantId": "main-1", "invoice": observation}}))
		case "/tax/files":
			fileCalls.Add(1)
			assert.Equal(t, "starpay.invoice.files.v1", command.Operation)
			assert.Equal(t, remoteID, command.InvoiceID)
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"merchantId": "main-1", "url": downloadURL}}))
		default:
			t.Errorf("unexpected invoice endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	gateway, err := paymentprovider.NewPerPayInvoices(server.URL+"/tax/", "main-1", "api-1", secret, server.Client().Transport)
	require.NoError(t, err)
	service, err := payment.NewTaxInvoices(payment.TaxInvoiceConfig{Enabled: true, Endpoint: server.URL + "/tax/", MerchantID: "main-1", EpayPID: "1001", ProviderInstanceID: "9"}, store, gateway)
	require.NoError(t, err)
	buyer := payment.TaxInvoiceBuyer{BuyerType: "ORGANIZATION", Title: "Synthetic buyer company", TaxID: "91310000987654321B", Email: "invoice@example.test"}
	view, err := service.Apply(ctx, 41, 7, buyer)
	require.NoError(t, err)
	require.NotNil(t, view.Invoice)
	require.Equal(t, "UNKNOWN", view.Invoice.Status)
	require.Equal(t, "invoice_response_unavailable", view.Invoice.ErrorCode)
	require.Equal(t, int32(1), createCalls.Load())
	applicationID, requestID := view.Invoice.ID, view.Invoice.RequestID
	_, err = service.File(ctx, 41, 7)
	require.Equal(t, "TAX_INVOICE_NOT_ISSUED", apperror.Reason(err))

	// Advance only the persisted schedule, avoiding a real thirty-second wait.
	// The worker still acquires its normal database lease and uses unchanged IDs.
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET next_sync_at='epoch' WHERE order_id=41`)
	require.NoError(t, err)
	require.NoError(t, service.Sync(ctx))
	view, err = service.View(ctx, 41, 7)
	require.NoError(t, err)
	require.Equal(t, "ISSUED", view.Invoice.Status)
	require.Equal(t, applicationID, view.Invoice.ID)
	require.Equal(t, requestID, view.Invoice.RequestID)
	require.Equal(t, "12345678901234567890", view.Invoice.InvoiceNo)
	require.Equal(t, int64(1000), view.Invoice.AmountCents)
	require.Equal(t, int32(2), createCalls.Load())

	// A later unresolved remote record exercises the query branch without
	// issuing again; the immutable application and upstream record remain identical.
	seedTaxInvoiceOrder(t, db, 42)
	second := taxInvoiceFixture(42)
	second.Endpoint = server.URL + "/tax/"
	second.Title = buyer.Title
	second.BuyerType = buyer.BuyerType
	second.TaxID = buyer.TaxID
	saved, err := store.Reserve(ctx, second, approveTaxInvoiceFixture)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE payment_tax_invoices SET status='PROCESSING',remote_id=$1 WHERE order_id=42`, queryRemoteID)
	require.NoError(t, err)
	requestMu.Lock()
	remote = payment.TaxInvoiceRemote{ID: queryRemoteID, RequestID: saved.RequestID, TradeNo: saved.TradeNo, Title: saved.Title, AmountCents: saved.AmountCents, Status: "ISSUED", InvoiceNo: "12345678901234567891"}
	requestMu.Unlock()
	require.NoError(t, service.Sync(ctx))
	secondView, err := service.View(ctx, 42, 7)
	require.NoError(t, err)
	require.Equal(t, "ISSUED", secondView.Invoice.Status)
	require.Equal(t, int32(1), queryCalls.Load())
	require.Equal(t, int32(2), createCalls.Load())
	link, err := service.File(ctx, 41, 7)
	require.NoError(t, err)
	require.Equal(t, downloadURL, link)
	require.Equal(t, int32(1), fileCalls.Load())
	_, err = service.View(ctx, 41, 8)
	require.Equal(t, "NOT_FOUND", apperror.Reason(err))
	require.Equal(t, int32(http.StatusNotFound), apperror.FromError(err).Code)
	_, err = service.File(ctx, 41, 8)
	require.Equal(t, "NOT_FOUND", apperror.Reason(err))
	require.Equal(t, int32(http.StatusNotFound), apperror.FromError(err).Code)
	require.Equal(t, int32(1), fileCalls.Load())
	var storedCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM payment_tax_invoices WHERE order_id=41`).Scan(&storedCount))
	require.Equal(t, 1, storedCount)
}

// taxInvoicePostgres creates a unique schema on an explicitly selected local
// test server. Callers never reuse production schema names, and cleanup drops
// only the randomly generated schema after its scoped pool has closed.
func taxInvoicePostgres(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TAX_INVOICE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TAX_INVOICE_TEST_POSTGRES_DSN is required for the isolated PostgreSQL test")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsed.Hostname())
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := "invoice_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close())
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE payment_orders (
		id bigint PRIMARY KEY,user_id bigint NOT NULL,pay_amount numeric(20,2) NOT NULL,
		payment_type text NOT NULL,payment_trade_no text NOT NULL,provider_instance_id text,
		provider_key text,provider_snapshot jsonb,status text NOT NULL,refund_amount numeric(20,2) NOT NULL DEFAULT 0,
		refund_at timestamptz,refund_requested_at timestamptz,paid_at timestamptz)`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("319_payment_tax_invoices.sql")
	require.NoError(t, err)
	// Replaying a transaction migration must preserve existing tables and rebuild
	// the same trigger definitions without creating duplicate objects or clearing data.
	for range 2 {
		_, err = db.Exec(string(migration))
		require.NoError(t, err)
	}
	return db
}

// seedTaxInvoiceOrder creates a paid fixture and its explicit financial approval.
// The fixture uses separate order numbers while keeping the same tested merchant
// binding, and it grants no global approval to other orders.
func seedTaxInvoiceOrder(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO payment_orders(id,user_id,pay_amount,payment_type,payment_trade_no,
		provider_instance_id,provider_key,provider_snapshot,status,paid_at)
		VALUES($1,7,10,'wxpay',$2,'9','easypay','{"currency":"CNY","merchant_id":"1001","provider_key":"easypay","provider_instance_id":"9"}','COMPLETED',now())`, id, fmt.Sprintf("paid-trade-%d", id))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO payment_tax_invoice_eligibility(order_id,reference,approved_by) VALUES($1,$2,'finance-fixture')`, id, fmt.Sprintf("confirmed-sale-%d", id))
	require.NoError(t, err)
}

// taxInvoiceFixture returns synthetic buyer facts with a new stable application
// identity for one fixture order. Repeated reservations must return the first
// saved identity instead of adopting the candidate's newly generated values.
func taxInvoiceFixture(orderID int64) *payment.TaxInvoice {
	return &payment.TaxInvoice{
		ID: uuid.NewString(), OrderID: orderID, UserID: 7, RequestID: uuid.NewString(),
		TradeNo: fmt.Sprintf("paid-trade-%d", orderID), AmountCents: 1000, BuyerType: "INDIVIDUAL", Title: "Test buyer",
		Email: "invoice@example.test", Endpoint: "https://invoice.example.test/", MerchantID: "main-1", EpayPID: "1001",
		ProviderInstanceID: "9", Status: "UNKNOWN", UpdatedAt: time.Now(), NextSyncAt: time.Now(),
	}
}

// approveTaxInvoiceFixture checks payment state and trusted fixture financial approval.
// It rejects a refunded or unapproved record while the reservation owns the
// payment lock, without making any external provider request.
func approveTaxInvoiceFixture(order *payment.TaxInvoiceOrder) error {
	if !order.Approved || order.Order.Status != "COMPLETED" {
		return fmt.Errorf("order is not an approved completed sale")
	}
	return nil
}
