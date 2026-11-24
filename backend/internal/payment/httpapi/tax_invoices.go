package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// tax_invoices.go exposes owned digital invoice applications and temporary files.
// Every endpoint derives the buyer from authenticated context and parses the
// addressed order before invoking the payment service. Request bodies contain
// buyer details only; financial eligibility, amounts and merchant identities
// remain server-controlled. Responses disable caching because they include
// invoice buyer details or short-lived document links.

// GetTaxInvoice returns saved invoice state and current eligibility for one order.
// Authentication and ownership failures retain the shared HTTP error policy.
// This read does not contact the provider or initiate a new invoice application.
func (h *PaymentHandler) GetTaxInvoice(c *gin.Context) {
	order, user, ok := h.taxInvoiceIdentity(c)
	if !ok {
		return
	}
	view, err := h.taxInvoices.View(c.Request.Context(), order, user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// CreateTaxInvoice accepts bounded buyer input and submits the original application.
// Unknown fields and trailing JSON are rejected before storage. The service owns
// payment eligibility, immutable fields and idempotent recovery across repeated calls.
func (h *PaymentHandler) CreateTaxInvoice(c *gin.Context) {
	order, user, ok := h.taxInvoiceIdentity(c)
	if !ok {
		return
	}
	var buyer payment.TaxInvoiceBuyer
	// Limit the decoded buyer document before service validation so oversized
	// payloads cannot consume unbounded memory while submitting this authenticated application.
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&buyer); err != nil {
		response.BadRequest(c, "Invalid invoice buyer details")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		response.BadRequest(c, "Invalid invoice buyer details")
		return
	}
	view, err := h.taxInvoices.Apply(c.Request.Context(), order, user, buyer)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// GetTaxInvoiceFile returns a temporary HTTPS link for an owned issued invoice.
// The payment service and upstream recheck availability before publishing it.
// Clients must request another link after expiry instead of retaining it permanently.
func (h *PaymentHandler) GetTaxInvoiceFile(c *gin.Context) {
	order, user, ok := h.taxInvoiceIdentity(c)
	if !ok {
		return
	}
	link, err := h.taxInvoices.File(c.Request.Context(), order, user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"url": link})
}

// taxInvoiceIdentity validates authenticated routing inputs and suppresses caching.
// A missing service is unavailable, while a nonpositive or malformed order ID is a bad
// request. Callers stop when the boolean is false because a response was written.
func (h *PaymentHandler) taxInvoiceIdentity(c *gin.Context) (int64, int64, bool) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	subject, ok := UserRequireAuth(c)
	if !ok {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid order ID")
		return 0, 0, false
	}
	if h.taxInvoices == nil {
		response.ErrorFrom(c, apperror.ServiceUnavailable("TAX_INVOICE_UNAVAILABLE", "Invoice service is unavailable"))
		return 0, 0, false
	}
	return id, subject.UserID, true
}
