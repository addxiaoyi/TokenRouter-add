package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/batchimage"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// history.go mounts bounded task metadata reads for panel sessions and
// administrators. The authenticated user route derives the submitting account
// from its session, while the administrator route accepts an optional account
// filter. Both read the same narrow projection under the panel's authorization
// and rate limiting. Gateway task results retain their original access policy.

// The history operation makes the existing administrator scope visible in the
// management catalog, and the timeout bounds each query independently of client lifetime.
const (
	adminHistoryPath = "/api/v1/admin/task-history"
	historyTimeout   = 5 * time.Second
)

// adminHistory describes the read-only batch history route for the management catalog.
var adminHistory = adminapi.Define(adminapi.Operation{
	ID:          "task_history.list",
	Module:      "gateway",
	Title:       "List batch task history",
	Description: "Read visible batch image job state and item counts, optionally scoped to the submitting user.",
	Method:      http.MethodGet,
	Path:        adminHistoryPath,
	ReadOnly:    true,
	BrowserPath: "/admin/task-history",
	Input:       adminapi.Arguments(adminHistoryPath, []string{"batch_id", "cursor", "status", "user_id"}, nil),
})

// HistoryReader retrieves projected task metadata for the supplied panel scope.
type HistoryReader interface {
	// List returns one page of visible batch task metadata and a next cursor.
	List(context.Context, batchimage.HistoryFilter) (batchimage.HistoryPage, error)
}

// HistoryHandler supplies read-only history endpoints under two panel guards.
type HistoryHandler struct {
	reader HistoryReader // projected persisted task reader
}

// NewHistoryHandler binds task history to its read-only data source.
func NewHistoryHandler(reader HistoryReader) *HistoryHandler {
	return &HistoryHandler{reader: reader}
}

// RegisterUserHistory mounts the session-scoped read with the heavy panel limit.
// Callers install the authenticated panel group before registration so user
// identity originates from a verified session.
func RegisterUserHistory(group *gin.RouterGroup, h *HistoryHandler, heavy gin.HandlerFunc) {
	group.GET("/task-history", heavy, h.ReadUser)
}

// RegisterAdminHistory mounts the catalogued read using the administrator group.
// Its caller must supply a group already protected by administrator
// authentication and the ordinary administrative request audit.
func RegisterAdminHistory(group *gin.RouterGroup, h *HistoryHandler, heavy gin.HandlerFunc) {
	adminapi.Register(group, adminHistory, heavy, h.ReadAdmin)
}

// ReadUser scopes history to the session's submitting account. Every query is
// filtered by verified user ID, and a supplied user ID fails before storage
// can read any records.
func (h *HistoryHandler) ReadUser(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID < 1 {
		httpx.Unauthorized(c, "User not authenticated")
		return
	}
	if _, supplied := c.GetQuery("user_id"); supplied {
		httpx.BadRequest(c, "User filter is available to administrators only")
		return
	}
	history, ok := h.read(c, subject.UserID)
	if !ok {
		return
	}
	for i := range history.Items {
		history.Items[i].UserID = 0
	}
	httpx.Success(c, history)
}

// ReadAdmin accepts an optional submitting-account filter on an authenticated
// administrator route. Omitted scope includes all accounts, while a malformed
// account filter fails instead of broadening the query.
func (h *HistoryHandler) ReadAdmin(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	var userID int64
	if supplied, present := c.GetQuery("user_id"); present {
		var err error
		userID, err = strconv.ParseInt(supplied, 10, 64)
		if err != nil || userID < 1 {
			httpx.BadRequest(c, "User ID must be a positive integer")
			return
		}
	}
	history, ok := h.read(c, userID)
	if ok {
		httpx.Success(c, history)
	}
}

// read validates common filters and returns a page ready for projection. An expired
// request or database failure goes through the panel error writer instead of
// being misreported as an empty history.
func (h *HistoryHandler) read(c *gin.Context, userID int64) (batchimage.HistoryPage, bool) {
	filter := batchimage.HistoryFilter{
		UserID:  userID,
		Status:  strings.TrimSpace(c.Query("status")),
		BatchID: strings.TrimSpace(c.Query("batch_id")),
	}
	if !batchimage.ValidHistoryStatus(filter.Status) || len(filter.BatchID) > 64 {
		httpx.BadRequest(c, "Invalid task history filters")
		return batchimage.HistoryPage{}, false
	}
	if supplied, present := c.GetQuery("cursor"); present {
		var err error
		filter.BeforeID, err = strconv.ParseInt(supplied, 10, 64)
		if err != nil || filter.BeforeID < 1 {
			httpx.BadRequest(c, "Invalid task history cursor")
			return batchimage.HistoryPage{}, false
		}
	}
	// A separate short deadline limits database work when a panel client
	// keeps the connection open while a query is waiting for resources.
	ctx, cancel := context.WithTimeout(c.Request.Context(), historyTimeout)
	defer cancel()
	history, err := h.reader.List(ctx, filter)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return batchimage.HistoryPage{}, false
	}
	return history, true
}
