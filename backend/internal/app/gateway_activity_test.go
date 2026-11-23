package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/pkg/requestcontext"
)

// gateway_activity_test.go verifies request cleanup before dependent services stop.
// Gateway activity must finish before completion queues and Redis shut down.
// Two entered operations reproduce a drain timeout, and a completed operation
// exercises successful shutdown ordering so both retained dependencies and
// normal cleanup remain observable to the application.
// TestGatewayRequestActivityTimeoutKeepsCompletionAndStorageOpen enters two
// operations that remain active during shutdown. A deadline must leave
// completion and Redis stop hooks uncalled, reject new activity, and remain
// the manager's repeated Stop result after the operations finish.
func TestGatewayRequestActivityTimeoutKeepsCompletionAndStorageOpen(t *testing.T) {
	manager := lifecycle.New()
	activity := provideGatewayRequestActivity(manager)
	requestDone, err := activity.Enter()
	require.NoError(t, err)
	defer requestDone()
	attemptDone, err := activity.Enter()
	require.NoError(t, err)
	defer attemptDone()
	var completionStopped, redisClosed atomic.Bool
	// Stop hooks can publish observations during shutdown. Atomic flags let the
	// test inspect completion and storage cleanup safely after the drain deadline.
	manager.Register(lifecycle.Hook{Name: "completion", StopOrder: 40, Stop: func(context.Context) error { completionStopped.Store(true); return nil }})
	manager.Register(lifecycle.Hook{Name: "redis", StopOrder: 900, Stop: func(context.Context) error { redisClosed.Store(true); return nil }})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = manager.Stop(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// Manager reports the unfinished phase; the activity tracker retains the unfinished operation count.
	require.ErrorContains(t, err, "GatewayRequestsAndAttempts")
	detailCtx, detailCancel := context.WithTimeout(context.Background(), time.Second)
	defer detailCancel()
	require.ErrorContains(t, activity.StopContext(detailCtx), "2 operations unfinished")
	require.False(t, completionStopped.Load())
	require.False(t, redisClosed.Load())
	_, err = activity.Enter()
	require.Error(t, err)
	attemptDone()
	requestDone()
	require.ErrorIs(t, manager.Stop(context.Background()), context.DeadlineExceeded)
}

// TestGatewayRequestActivityFinishesBeforeCompletionAndStorage releases its
// entered operation before stopping the manager. Shutdown must succeed and run
// the completion hook before the Redis hook in their configured stop order.
func TestGatewayRequestActivityFinishesBeforeCompletionAndStorage(t *testing.T) {
	manager := lifecycle.New()
	activity := provideGatewayRequestActivity(manager)
	done, err := activity.Enter()
	require.NoError(t, err)
	var order []string
	manager.Register(lifecycle.Hook{Name: "completion", StopOrder: 40, Stop: func(context.Context) error { order = append(order, "completion"); return nil }})
	manager.Register(lifecycle.Hook{Name: "redis", StopOrder: 900, Stop: func(context.Context) error { order = append(order, "redis"); return nil }})
	done()
	require.NoError(t, manager.Stop(context.Background()))
	require.Equal(t, []string{"completion", "redis"}, order)
}

// TestGatewayRequestsAbortDetachedUpstreamDuringShutdown keeps an upstream HTTP call
// running after client cancellation. Shutdown must cancel that call and wait for
// handler cleanup before releasing storage, including nested API-key abort scopes.
func TestGatewayRequestsAbortDetachedUpstreamDuringShutdown(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprint(nested), func(t *testing.T) {
			entered := make(chan struct{})
			forced := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				select {
				case <-r.Context().Done():
				case <-forced:
				}
			}))
			// Release the upstream fixture even when shutdown fails, so the failing
			// regression cannot leave its server waiting for a disconnected handler.
			t.Cleanup(upstream.Close)
			t.Cleanup(func() { close(forced) })
			var cleaned atomic.Bool
			detached := make(chan context.Context, 1)
			handled := make(chan struct{})
			manager := lifecycle.New()
			activity := provideGatewayRequestActivity(manager)
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				release, err := activity.Enter()
				if err != nil {
					t.Error(err)
					return
				}
				defer release()
				ctx := r.Context()
				if nested {
					var abort context.CancelFunc
					ctx, abort = requestcontext.WithAbort(ctx)
					defer abort()
				}
				ctx = requestcontext.Detach(ctx)
				detached <- ctx
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
				if err != nil {
					t.Error(err)
					return
				}
				response, err := upstream.Client().Do(request)
				if err == nil {
					_ = response.Body.Close()
				} else if !errors.Is(err, context.Canceled) {
					t.Error(err)
				}
				cleaned.Store(true)
			})}
			lifecycle.TrackRequests(server, manager, requestcontext.WithAbort)
			manager.Register(lifecycle.Hook{Name: "storage", StopOrder: 900, Stop: func(context.Context) error {
				if !cleaned.Load() {
					return errors.New("storage closed before handler cleanup")
				}
				return nil
			}})
			client, disconnect := context.WithCancel(t.Context())
			defer disconnect()
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(client)
			// The handler remains independently active after the client cancels.
			// Its completion signal lets assertions distinguish upstream abort from cleanup.
			go func() {
				defer close(handled)
				server.Handler.ServeHTTP(httptest.NewRecorder(), request)
			}()
			<-entered
			work := <-detached
			disconnect()
			require.NoError(t, work.Err(), "client disconnect must preserve upstream work")
			stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.NoError(t, manager.Stop(stopCtx))
			<-handled
			require.True(t, cleaned.Load())
			require.ErrorIs(t, work.Err(), context.Canceled)
		})
	}
}
