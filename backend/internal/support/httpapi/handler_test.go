package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/support"
)

// handler_test.go verifies support route ownership at the HTTP boundary.
// The authenticated account and route scope must override attempted owner and
// staff values in request JSON. Invalid bodies and missing authentication stop
// before storage, while successful responses retain private cache controls.

// scopedStore records the actor selected by the HTTP handler for persistence.
type scopedStore struct {
	support.Store
	actor support.Actor
	calls int
} // recorded authorized write scope

// Create records trusted scope and returns one deterministic committed ticket.
func (s *scopedStore) Create(_ context.Context, actor support.Actor, _ support.CreateInput) (int64, error) {
	s.actor = actor
	s.calls++
	return 1, nil
}

// Get supplies the committed conversation after the handler creates its ticket.
func (s *scopedStore) Get(context.Context, support.Actor, int64, int64) (support.Thread, error) {
	return support.Thread{Ticket: support.Ticket{ID: 1}, Messages: []support.Message{}}, nil
}

// TestCreateUsesAuthenticatedOwner supplies spoofed owner and administrator fields.
// The persisted actor remains the authenticated ordinary user, and missing
// authentication cannot create a conversation regardless of the submitted body.
func TestCreateUsesAuthenticatedOwner(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		store := &scopedStore{}
		handler := NewHandler(support.NewService(store), nil)
		engine := gin.New()
		engine.POST("/support", func(c *gin.Context) {
			if authenticated {
				c.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: 7})
			}
			handler.Create(c)
		})
		request := httptest.NewRequest(http.MethodPost, "/support", strings.NewReader(`{"subject":"Need help","message":"The request failed","client_id":"`+uuid.NewString()+`","user_id":99,"is_staff":true}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if authenticated {
			require.Equal(t, 200, response.Code)
			require.Equal(t, support.Actor{UserID: 7}, store.actor)
			require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		} else {
			require.Equal(t, 401, response.Code)
			require.Zero(t, store.calls)
		}
	}
}
