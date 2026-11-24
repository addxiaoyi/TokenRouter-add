package httpapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
	"github.com/TokenFlux/TokenRouter/internal/support"
)

// handler.go exposes support conversations through authenticated panel routes.
// User identity always comes from middleware, and staff scope comes from the
// registrar's administrator-only wrapper. JSON bodies cannot choose an owner
// or elevate permissions. Private responses disable browser and shared caching.

// Handler binds conversation use cases and administrator delivery diagnostics.
type Handler struct {
	service *support.Service // validated support use cases
	outbox  support.Outbox   // notification diagnostic and retry operations
}

// RecipientsInput carries staff email addresses without exposing SMTP credentials.
type RecipientsInput struct {
	Recipients []string `json:"recipients"`
} // validated notification address list

// NewHandler binds support operations without accessing persistent ticket contents.
func NewHandler(service *support.Service, outbox support.Outbox) *Handler {
	return &Handler{service: service, outbox: outbox}
}

// actor reads authenticated scope and disables caching for private ticket responses.
// Staff is supplied only by route registration, never request parameters. Missing
// identity stops the handler before any user-owned records can be queried.
func actor(c *gin.Context, staff bool) (support.Actor, bool) {
	c.Header("Cache-Control", "private, no-store")
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		httpx.Unauthorized(c, "User not authenticated")
		return support.Actor{}, false
	}
	return support.Actor{UserID: subject.UserID, Staff: staff}, true
}

// identifier parses a positive route identifier before accessing support storage.
func identifier(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.BadRequest(c, "Invalid ticket identifier")
		return 0, false
	}
	return id, true
}

// list returns paginated support tickets within the registrar's authorized scope.
// Omitted pagination uses service defaults, while malformed integer parameters
// return a validation response before any database query starts.
func (h *Handler) list(c *gin.Context, staff bool) {
	caller, ok := actor(c, staff)
	if !ok {
		return
	}
	page, e1 := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, e2 := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if e1 != nil || e2 != nil {
		httpx.BadRequest(c, "Invalid ticket pagination")
		return
	}
	rows, err := h.service.List(c.Request.Context(), caller, support.Filter{Page: page, PageSize: size, Status: c.Query("status"), Search: c.Query("search")})
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, rows)
}

// get returns one owned or staff-accessible ticket and its next message page.
// Missing and inaccessible ticket identifiers share the same not-found response,
// and invalid message cursors are rejected before conversation retrieval.
func (h *Handler) get(c *gin.Context, staff bool) {
	caller, ok := actor(c, staff)
	if !ok {
		return
	}
	id, ok := identifier(c)
	if !ok {
		return
	}
	after, err := strconv.ParseInt(c.DefaultQuery("after_id", "0"), 10, 64)
	if err != nil {
		httpx.BadRequest(c, "Invalid message cursor")
		return
	}
	thread, err := h.service.Get(c.Request.Context(), caller, id, after)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, thread)
}

// Create submits an owned ticket using a stable client retry identity.
// The service validates all text before transaction creation. The response is
// emitted only after the ticket and its initial message have committed.
func (h *Handler) Create(c *gin.Context) {
	caller, ok := actor(c, false)
	if !ok {
		return
	}
	var input support.CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "Invalid ticket request")
		return
	}
	thread, err := h.service.Create(c.Request.Context(), caller, input)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, thread)
}

// reply appends a validated message in the registrar's authorized ticket scope.
// Idempotent retries preserve one reply, while closed or concurrently changed
// state returns the domain error without replacing previous messages.
func (h *Handler) reply(c *gin.Context, staff bool) {
	caller, ok := actor(c, staff)
	if !ok {
		return
	}
	id, ok := identifier(c)
	if !ok {
		return
	}
	var input support.ReplyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "Invalid ticket reply")
		return
	}
	if err := h.service.Reply(c.Request.Context(), caller, id, input); err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"ok": true})
}

// status applies a revision-bound workflow update within the caller's scope.
// Staff states are enforced by the service, and a stale displayed version
// returns conflict so the operator can reload the current conversation.
func (h *Handler) status(c *gin.Context, staff bool) {
	caller, ok := actor(c, staff)
	if !ok {
		return
	}
	id, ok := identifier(c)
	if !ok {
		return
	}
	var input support.StatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "Invalid ticket status request")
		return
	}
	if err := h.service.SetStatus(c.Request.Context(), caller, id, input); err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"ok": true})
}

// Settings returns support notification recipients and delivery counts to staff.
// The protected registrar owns administrator authorization; failures stop the
// response rather than presenting an empty configuration as a successful read.
func (h *Handler) Settings(c *gin.Context) {
	if _, ok := actor(c, true); !ok {
		return
	}
	recipients, err := h.service.Recipients(c.Request.Context())
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	state, err := h.outbox.DeliveryState(c.Request.Context())
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"recipients": recipients, "delivery": state})
}

// SaveSettings replaces the validated recipient list for future ticket notifications.
// Queued messages retain their original destinations. Invalid input leaves the
// existing configuration intact and returns a descriptive validation response.
func (h *Handler) SaveSettings(c *gin.Context) {
	if _, ok := actor(c, true); !ok {
		return
	}
	var input RecipientsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "Invalid support settings")
		return
	}
	recipients, err := h.service.SetRecipients(c.Request.Context(), input.Recipients)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"recipients": recipients})
}

// Retry resets exhausted notification attempts after staff repairs mail configuration.
// Delivered notices remain untouched, and the background worker performs later
// SMTP delivery independently of this administrator request.
func (h *Handler) Retry(c *gin.Context) {
	if _, ok := actor(c, true); !ok {
		return
	}
	if err := h.outbox.RetryFailed(c.Request.Context()); err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"ok": true})
}
