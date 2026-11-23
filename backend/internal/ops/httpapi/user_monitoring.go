package httpapi

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// user_monitoring.go exposes aggregate service health to authenticated users.
// The application reader derives permitted groups from current grants, and this
// endpoint supplies only authenticated identity and a bounded window. Private
// response caching is disabled to avoid retaining revoked group access.

// UserMonitoringReader supplies permission-scoped observations without administrative details.
type UserMonitoringReader interface {
	// Read resolves the user's current groups and returns a bounded observation window.
	Read(context.Context, int64, int) (ops.UserMonitoring, error)
}

// UserMonitoringHandler exposes one permission-aware service monitoring query.
type UserMonitoringHandler struct{ reader UserMonitoringReader } // authorized aggregate reader

// NewUserMonitoringHandler binds authorized aggregate reads without fetching private data.
func NewUserMonitoringHandler(reader UserMonitoringReader) *UserMonitoringHandler {
	return &UserMonitoringHandler{reader: reader}
}

// Read returns service summaries for the current user over 1, 24, 72, or 168 hours.
// Unknown windows and missing identity are rejected before any query, and the
// supplied request context cancels both permission and observation reads.
func (h *UserMonitoringHandler) Read(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		httpx.Unauthorized(c, "User not authenticated")
		return
	}
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if err != nil || (hours != 1 && hours != 24 && hours != 72 && hours != 168) {
		httpx.BadRequest(c, "Monitoring window must be 1, 24, 72, or 168 hours")
		return
	}
	view, err := h.reader.Read(c.Request.Context(), subject.UserID, hours)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, view)
}

// RegisterUserMonitoring mounts aggregate reads behind authentication and heavy-query limits.
func RegisterUserMonitoring(group *gin.RouterGroup, h *UserMonitoringHandler, heavy gin.HandlerFunc) {
	group.GET("/monitoring", heavy, h.Read)
}
