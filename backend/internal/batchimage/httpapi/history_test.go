package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/batchimage"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
)

// history_test.go verifies panel history permissions and query validation before
// storage. The reader captures the scope passed by each route, and fixture
// results show whether the handler discloses submitter identity to the client.

// historyReaderProbe records the validated filter and returns one visible job.
type historyReaderProbe struct {
	filters []batchimage.HistoryFilter // validated filters passed to storage
}

// List records the effective scope and returns a metadata-only fixture.
func (p *historyReaderProbe) List(_ context.Context, filter batchimage.HistoryFilter) (batchimage.HistoryPage, error) {
	p.filters = append(p.filters, filter)
	return batchimage.HistoryPage{
		Items: []batchimage.HistoryEntry{{ID: "imgbatch_fixture", UserID: 42, Model: "image-public"}},
	}, nil
}

// TestHistoryHandlerScopes protects the submitting user's session scope and
// makes submitter identity available only on the guarded administrator route.
// Invalid account or pagination filters are rejected before querying storage.
func TestHistoryHandlerScopes(t *testing.T) {
	reader := &historyReaderProbe{}
	handler := NewHistoryHandler(reader)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	user := v1.Group("", func(c *gin.Context) {
		if c.GetHeader("Authorization") == "Bearer fixture-user" {
			c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 42})
		}
	})
	admin := v1.Group("/admin", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer fixture-admin" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	pass := func(c *gin.Context) { c.Next() }
	RegisterUserHistory(user, handler, pass)
	RegisterAdminHistory(admin, handler, pass)
	cases := []struct {
		name       string // subtest description
		path       string // route and query parameters
		credential string // fixture authentication header value
		status     int    // expected HTTP response status
		wantUser   bool   // expected submitter field presence
	}{
		{"anonymous", "/api/v1/task-history", "", http.StatusUnauthorized, false},
		{"no cross-account filter", "/api/v1/task-history?user_id=7", "Bearer fixture-user", http.StatusBadRequest, false},
		{"invalid cursor", "/api/v1/task-history?cursor=0", "Bearer fixture-user", http.StatusBadRequest, false},
		{"invalid status", "/api/v1/task-history?status=unknown", "Bearer fixture-user", http.StatusBadRequest, false},
		{"session scope", "/api/v1/task-history?status=running&cursor=5", "Bearer fixture-user", http.StatusOK, false},
		{"user cannot use admin route", "/api/v1/admin/task-history", "Bearer fixture-user", http.StatusForbidden, false},
		{"invalid administrator filter", "/api/v1/admin/task-history?user_id=0", "Bearer fixture-admin", http.StatusBadRequest, false},
		{"cross-account administrator", "/api/v1/admin/task-history?user_id=42&batch_id=imgbatch_fixture", "Bearer fixture-admin", http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", tc.credential)
			engine.ServeHTTP(writer, req)
			require.Equal(t, tc.status, writer.Code, writer.Body.String())
			require.Equal(t, tc.wantUser, strings.Contains(writer.Body.String(), `"user_id":42`))
			if tc.status == http.StatusOK {
				require.Equal(t, "private, no-store", writer.Header().Get("Cache-Control"))
			}
		})
	}
	require.Equal(t, []batchimage.HistoryFilter{
		{UserID: 42, BeforeID: 5, Status: "running"},
		{UserID: 42, BatchID: "imgbatch_fixture"},
	}, reader.filters)
}
