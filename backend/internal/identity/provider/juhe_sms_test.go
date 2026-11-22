package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// juhe_sms_test.go covers the Juhe SMS submission client against a local
// provider double.
// The provider accepts a form post and reports acceptance only through a
// zero error_code, so the tests pin the submitted fields and both reply
// outcomes.

// withJuheSMSEndpoint points the client at a local server for one test
// and restores the production endpoint afterwards.
func withJuheSMSEndpoint(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := JuheSMSEndpoint
	JuheSMSEndpoint = server.URL
	t.Cleanup(func() {
		JuheSMSEndpoint = previous
		server.Close()
	})
	return server
}

// TestJuheSMSClientSendsTemplateVariables submits one code and checks
// the form carries the key, number, template, and code variable the
// provider contract requires.
func TestJuheSMSClientSendsTemplateVariables(t *testing.T) {
	var received url.Values
	withJuheSMSEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		received = r.PostForm
		_, _ = w.Write([]byte(`{"reason":"success","result":{"count":1},"error_code":0}`))
	})

	err := (JuheSMSClient{}).SendSMSCode(context.Background(), "KEY-1", "TPL-7", "13800138000", "123456")
	require.NoError(t, err)
	require.Equal(t, "KEY-1", received.Get("key"))
	require.Equal(t, "13800138000", received.Get("mobile"))
	require.Equal(t, "TPL-7", received.Get("tpl_id"))
	require.Equal(t, "#code#=123456", received.Get("tpl_value"))
}

// TestJuheSMSClientSurfacesProviderError returns the provider reason when
// the reply carries a nonzero error code.
func TestJuheSMSClientSurfacesProviderError(t *testing.T) {
	withJuheSMSEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"reason":"invalid mobile","result":null,"error_code":205401}`))
	})

	err := (JuheSMSClient{}).SendSMSCode(context.Background(), "KEY-1", "TPL-7", "123", "123456")
	require.Error(t, err)
	require.Contains(t, err.Error(), "205401")
	require.Contains(t, err.Error(), "invalid mobile")
}
