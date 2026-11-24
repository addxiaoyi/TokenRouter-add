package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// tax_invoices_test.go checks application ownership and worker publication
// boundaries through controlled SQL replies. These tests observe the queries
// and state returned to the domain service; PostgreSQL integration scenarios
// separately verify row locking and trigger behavior against a real database.

// TestTaxInvoiceReadHidesForeignOrders simulates an order outside the requesting
// account. The store must return not-found without reading any invoice details.
func TestTaxInvoiceReadHidesForeignOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	mock.ExpectQuery("SELECT .* FROM payment_orders").WithArgs(int64(42), int64(7)).WillReturnError(sql.ErrNoRows)
	_, _, err = NewTaxInvoiceStore(db).Read(context.Background(), 42, 7)
	require.Equal(t, "NOT_FOUND", apperror.Reason(err))
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
}

// TestTaxInvoiceSaveRejectsStaleLease models a replacement worker winning its
// claim. A result from the earlier lease must not overwrite the persisted record.
func TestTaxInvoiceSaveRejectsStaleLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	invoice := &payment.TaxInvoice{ID: "00000000-0000-4000-8000-000000000001", LeaseUntil: time.Now().UTC(), Status: "ISSUED", InvoiceNo: "12345678901234567890"}
	mock.ExpectExec("UPDATE payment_tax_invoices").WillReturnResult(sqlmock.NewResult(0, 0))
	require.Equal(t, "TAX_INVOICE_CONFLICT", apperror.Reason(NewTaxInvoiceStore(db).Save(context.Background(), invoice)))
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
}

// TestTaxInvoiceClaimDoesNotInventOwnership simulates an already claimed invoice.
// The caller receives no record and cannot dispatch another upstream request.
func TestTaxInvoiceClaimDoesNotInventOwnership(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	mock.ExpectQuery("UPDATE payment_tax_invoices").WillReturnError(sql.ErrNoRows)
	invoice, claimed, err := NewTaxInvoiceStore(db).Claim(context.Background(), "00000000-0000-4000-8000-000000000001", time.Now())
	require.NoError(t, err)
	require.False(t, claimed)
	require.Nil(t, invoice)
	require.NoError(t, mock.ExpectationsWereMet())
	mock.ExpectClose()
}
