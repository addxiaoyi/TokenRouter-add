package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// user_monitoring_test.go checks authenticated identity and observation limits.
// Query parameters cannot select another user, and malformed windows never
// invoke the aggregate reader. HTTP success preserves an explicit disabled
// state and private cache policy rather than presenting fabricated metrics.

// monitoringReader captures the authenticated account and validated time window.
type monitoringReader struct {
	user  int64
	hours int
	calls int
} // observed authorized read arguments

// Read records the trusted request identity and returns explicitly disabled monitoring.
func (r *monitoringReader) Read(_ context.Context, user int64, hours int) (ops.UserMonitoring, error) {
	r.user = user
	r.hours = hours
	r.calls++
	return ops.UserMonitoring{Enabled: false, Groups: []ops.MonitoringGroup{}}, nil
}

// TestUserMonitoringAuthAndWindow probes unauthenticated, invalid, and valid requests.
// Only the valid authenticated request reaches storage, and user_id in the URL
// cannot replace the middleware's account identity.
func TestUserMonitoringAuthAndWindow(t *testing.T) {
	for _, tc := range []struct {
		query  string
		user   int64
		status int
	}{
		{"?hours=24", 0, 401}, {"?hours=8760", 7, 400}, {"?hours=x", 7, 400}, {"?hours=72&user_id=99", 7, 200}, {"?hours=1", 7, 200},
	} {
		reader := &monitoringReader{}
		handler := NewUserMonitoringHandler(reader)
		engine := gin.New()
		engine.GET("/monitoring", func(c *gin.Context) {
			if tc.user > 0 {
				c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: tc.user})
			}
			handler.Read(c)
		})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/monitoring"+tc.query, nil))
		require.Equal(t, tc.status, response.Code)
		if tc.status == 200 {
			require.Equal(t, int64(7), reader.user)
			if tc.query == "?hours=1" {
				require.Equal(t, 1, reader.hours)
			} else {
				require.Equal(t, 72, reader.hours)
			}
			require.Contains(t, response.Body.String(), `"enabled":false`)
			require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		} else {
			require.Zero(t, reader.calls)
		}
	}
}
