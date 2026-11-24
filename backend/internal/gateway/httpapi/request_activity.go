package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"sync"
)

// request_activity.go carries an optional observation callback from authorized
// generation routes to the existing body readers. Readers reuse their decoded
// bytes and call the callback at most once. This path does not take ownership
// of the body or add an independent read or a provider request.

// requestActivityKey is private to the ingress context carrying the observer.
type requestActivityKey struct{}

// WithRequestActivity installs a one-shot observer on an authenticated request.
// The callback must finish reading the borrowed bytes before returning. The
// observer sees only decoded, ambiguity-checked body reads in this package.
func WithRequestActivity(ctx context.Context, observe func([]byte)) context.Context {
	var once sync.Once
	callback := func(body []byte) {
		once.Do(func() { observe(body) })
	}
	return context.WithValue(ctx, requestActivityKey{}, callback)
}

// observeRequestActivity supplies borrowed bytes to the optional decoded-body observer.
func observeRequestActivity(req *http.Request, body []byte) {
	if observe, ok := req.Context().Value(requestActivityKey{}).(func([]byte)); ok {
		observe(bytes.TrimPrefix(body, utf8ByteOrderMark))
	}
}
