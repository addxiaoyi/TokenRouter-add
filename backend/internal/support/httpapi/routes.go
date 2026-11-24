package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/support"
)

// routes.go separates owner support actions from administrator management.
// Parent groups supply authentication, rate limits, and audit middleware before
// registration. Each wrapper fixes its scope once; clients cannot request staff
// access by adding JSON fields or query parameters to user routes.

// Operations describes support permissions for shared HTTP and MCP administration.
var Operations = map[string]*adminapi.Operation{
	"list":     adminapi.Define(adminapi.Operation{ID: "support.list", Module: "support", Title: "List support tickets", Method: "GET", Path: "/api/v1/admin/support-tickets", ReadOnly: true, BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets", []string{"page", "page_size", "status", "search"}, nil)}),
	"get":      adminapi.Define(adminapi.Operation{ID: "support.get", Module: "support", Title: "Read support conversation", Method: "GET", Path: "/api/v1/admin/support-tickets/:id", ReadOnly: true, BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/:id", []string{"after_id"}, nil)}),
	"reply":    adminapi.Define(adminapi.Operation{ID: "support.reply", Module: "support", Title: "Reply to support ticket", Method: "POST", Path: "/api/v1/admin/support-tickets/:id/replies", BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/:id/replies", nil, (*support.ReplyInput)(nil))}),
	"status":   adminapi.Define(adminapi.Operation{ID: "support.status", Module: "support", Title: "Update support status", Method: "PATCH", Path: "/api/v1/admin/support-tickets/:id/status", BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/:id/status", nil, (*support.StatusInput)(nil))}),
	"settings": adminapi.Define(adminapi.Operation{ID: "support.settings.get", Module: "support", Title: "Read support notifications", Method: "GET", Path: "/api/v1/admin/support-tickets/settings", ReadOnly: true, BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/settings", nil, nil)}),
	"save":     adminapi.Define(adminapi.Operation{ID: "support.settings.update", Module: "support", Title: "Set support notification recipients", Method: "PUT", Path: "/api/v1/admin/support-tickets/settings", BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/settings", nil, (*RecipientsInput)(nil))}),
	"retry":    adminapi.Define(adminapi.Operation{ID: "support.notifications.retry", Module: "support", Title: "Retry support email delivery", Method: "POST", Path: "/api/v1/admin/support-tickets/notifications/retry", BrowserPath: "/admin/support-tickets", Input: adminapi.Arguments("/api/v1/admin/support-tickets/notifications/retry", nil, nil)}),
}

// RegisterUser installs owner-only support endpoints beneath authenticated panel middleware.
func RegisterUser(group *gin.RouterGroup, h *Handler) {
	tickets := group.Group("/support-tickets")
	tickets.GET("", func(c *gin.Context) { h.list(c, false) })
	tickets.POST("", h.Create)
	tickets.GET("/:id", func(c *gin.Context) { h.get(c, false) })
	tickets.POST("/:id/replies", func(c *gin.Context) { h.reply(c, false) })
	tickets.PATCH("/:id/status", func(c *gin.Context) { h.status(c, false) })
}

// RegisterAdmin installs support management beneath existing administrator authentication.
// Every operation enters the shared permission catalog; the route fixes staff
// scope before invoking conversation handlers or notification settings operations.
func RegisterAdmin(group *gin.RouterGroup, h *Handler) {
	adminapi.Register(group, Operations["list"], func(c *gin.Context) { h.list(c, true) })
	adminapi.Register(group, Operations["get"], func(c *gin.Context) { h.get(c, true) })
	adminapi.Register(group, Operations["reply"], func(c *gin.Context) { h.reply(c, true) })
	adminapi.Register(group, Operations["status"], func(c *gin.Context) { h.status(c, true) })
	adminapi.Register(group, Operations["settings"], h.Settings)
	adminapi.Register(group, Operations["save"], h.SaveSettings)
	adminapi.Register(group, Operations["retry"], h.Retry)
}
