package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// cost_rate_test.go protects the boundary for explicit upstream-rate confirmation.
// Invalid identifiers, missing rates, negative values, and unsupported precision
// must be rejected before the handler can call provider configuration storage.

// TestSetCostRateRejectsInvalidInput uses no storage dependency to expose accidental
// writes. Every malformed request must fail validation with a client error.
func TestSetCostRateRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{`{}`, `{"rate":null}`, `{"rate":-1}`, `{"rate":1.00001}`, `{"rate":1000000}`, `{"rate":"1"}`} {
		router := gin.New()
		handler := &ManagementHandler{}
		router.PUT("/providers/:id/cost-rate", handler.SetCostRate)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/providers/1/cost-rate", strings.NewReader(input))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code)
	}
}
