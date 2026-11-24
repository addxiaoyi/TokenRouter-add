package httpapi

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/modelquality"
	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// handler.go exposes public model assessment reports and authenticated operations.
// Public responses contain safe report projections only. Administrator routes
// supply existing authentication and audit middleware, while submission identity
// is read from that authenticated context instead of caller-controlled JSON.

// Operations registers assessment management permissions for console and MCP callers.
var Operations = map[string]*adminapi.Operation{
	"targets": adminapi.Define(adminapi.Operation{ID: "model_quality.targets", Module: "model_quality", Title: "List model assessment targets", Method: "GET", Path: "/api/v1/admin/model-quality/targets", ReadOnly: true, BrowserPath: "/admin/model-quality", Input: adminapi.Arguments("/api/v1/admin/model-quality/targets", nil, nil)}),
	"start":   adminapi.Define(adminapi.Operation{ID: "model_quality.start", Module: "model_quality", Title: "Start model assessment", Method: "POST", Path: "/api/v1/admin/model-quality/tests", BrowserPath: "/admin/model-quality", Input: adminapi.Arguments("/api/v1/admin/model-quality/tests", nil, (*modelquality.StartInput)(nil))}),
	"get":     adminapi.Define(adminapi.Operation{ID: "model_quality.get", Module: "model_quality", Title: "Read model assessment progress", Method: "GET", Path: "/api/v1/admin/model-quality/tasks/:id", ReadOnly: true, BrowserPath: "/admin/model-quality", Input: adminapi.Arguments("/api/v1/admin/model-quality/tasks/:id", nil, nil)}),
}

// Handler binds safe history reads and validated assessment commands.
type Handler struct{ service *modelquality.Service } // assessment use case owner

// NewHandler retains the assessment service without issuing storage queries.
func NewHandler(service *modelquality.Service) *Handler { return &Handler{service: service} }

// RegisterAdmin installs catalogued assessment management under existing administrator guards.
func RegisterAdmin(group *gin.RouterGroup, h *Handler) {
	adminapi.Register(group, Operations["targets"], h.Targets)
	adminapi.Register(group, Operations["start"], h.Start)
	adminapi.Register(group, Operations["get"], h.Get)
}

// List returns bounded public reports and their fixed-suite interpretation metadata.
// Storage failures remain explicit, and responses bypass shared caches so queued
// progress and completed history do not inherit stale administrator state.
func (h *Handler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	reports, err := h.service.Store.List(ctx)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"reports": reports, "suite_version": modelquality.SuiteVersion, "probe_count": len(modelquality.Suite()), "max_output_tokens": modelquality.MaxOutputTokens})
}

// Targets returns configured eligible text providers to an authenticated administrator.
func (h *Handler) Targets(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	targets, err := h.service.Backend.Targets(ctx)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	active, err := h.service.Store.Active(ctx)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, gin.H{"targets": targets, "active": active})
}

// Start queues one validated assessment using the authenticated administrator identity.
// Invalid input fails before execution admission. Repeated client identifiers
// return committed progress, while an active different assessment returns conflict.
func (h *Handler) Start(c *gin.Context) {
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok {
		httpx.Unauthorized(c, "User not authenticated")
		return
	}
	var input modelquality.StartInput
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "Invalid assessment request")
		return
	}
	report, err := h.service.Start(c.Request.Context(), subject.UserID, input)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, report)
}

// Get returns current safe progress for a syntactically valid assessment identifier.
func (h *Handler) Get(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	if _, err := uuid.Parse(c.Param("id")); err != nil {
		httpx.BadRequest(c, "Invalid assessment identifier")
		return
	}
	report, err := h.service.Store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	httpx.Success(c, report)
}
