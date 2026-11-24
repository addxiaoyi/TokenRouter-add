package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/modelquality"
)

// handler_test.go checks public report projection and unauthenticated submission.
// Storage fixtures supply only safe reports, while handlers must not access
// execution or private target selection for either public reads or rejected writes.

// reportStore supplies one sanitized assessment without private execution fields.
type reportStore struct{ modelquality.Store }

// List returns a fixture report containing only the public assessment contract.
func (reportStore) List(context.Context) ([]modelquality.Report, error) {
	return []modelquality.Report{{ID: "public", Model: "model", Label: "Public label", Status: "incomplete", Probes: []modelquality.ProbeResult{}}}, nil
}

// TestPublicReportsExcludePrivateExecution checks the unauthenticated history projection.
// Only public report fields are serialized; provider identity and credentials
// remain inaccessible even when the response is inspected as arbitrary JSON.
func TestPublicReportsExcludePrivateExecution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewHandler(modelquality.NewService(reportStore{}, nil))
	router.GET("/reports", h.List)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/reports", nil))
	require.Equal(t, 200, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	for _, field := range []string{"provider_id", "operator_id", "fingerprint", "credentials", "answer"} {
		require.NotContains(t, response.Body.String(), `"`+field+`"`)
	}
}

// TestStartRejectsMissingIdentity submits without authentication context or execution dependencies.
// The handler must reject the request before it can enqueue any provider work.
func TestStartRejectsMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/start", NewHandler(nil).Start)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/start", nil))
	require.Equal(t, 401, response.Code)
}
