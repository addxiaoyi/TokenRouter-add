package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// qq_oauth_test.go covers QQ token, identity, and profile parsing plus the
// client exchange against a local provider double.
// QQ answers in JSON under fmt=json but historically also as form text and
// JSONP, so the tests pin every accepted success shape and the coded
// failure shapes that must never parse as grants.

// TestParseQQTokenResponseJSON accepts the fmt=json success shape and
// retains the refresh fields.
func TestParseQQTokenResponseJSON(t *testing.T) {
	token, ok := ParseQQTokenResponse(`{"access_token":"AT-1","expires_in":7776000,"refresh_token":"RT-1"}`)
	require.True(t, ok)
	require.Equal(t, "AT-1", token.AccessToken)
	require.Equal(t, int64(7776000), token.ExpiresIn)
	require.Equal(t, "RT-1", token.RefreshToken)
}

// TestParseQQTokenResponseForm accepts the legacy form-encoded success
// shape.
func TestParseQQTokenResponseForm(t *testing.T) {
	token, ok := ParseQQTokenResponse("access_token=AT-2&expires_in=7776000&refresh_token=RT-2")
	require.True(t, ok)
	require.Equal(t, "AT-2", token.AccessToken)
	require.Equal(t, int64(7776000), token.ExpiresIn)
}

// TestParseQQTokenResponseRejectsFailures rejects coded error replies and
// empty bodies so a failed grant never parses as a token.
func TestParseQQTokenResponseRejectsFailures(t *testing.T) {
	_, ok := ParseQQTokenResponse(`{"code":100020,"msg":"code is reused error"}`)
	require.False(t, ok)

	_, ok = ParseQQTokenResponse("")
	require.False(t, ok)

	_, ok = ParseQQTokenResponse(`{"expires_in":7776000}`)
	require.False(t, ok)
}

// TestParseQQMeResponse covers the plain JSON shape, the JSONP wrapper,
// the unionid field, and coded failures.
func TestParseQQMeResponse(t *testing.T) {
	openID, unionID, err := ParseQQMeResponse(`{"client_id":"102","openid":"OID-1","unionid":"UID-1"}`)
	require.NoError(t, err)
	require.Equal(t, "OID-1", openID)
	require.Equal(t, "UID-1", unionID)

	openID, unionID, err = ParseQQMeResponse(`callback( {"client_id":"102","openid":"OID-2"} );`)
	require.NoError(t, err)
	require.Equal(t, "OID-2", openID)
	require.Equal(t, "", unionID)

	_, _, err = ParseQQMeResponse(`{"code":100016,"msg":"access token check failed"}`)
	require.Error(t, err)

	_, _, err = ParseQQMeResponse(`{"client_id":"102"}`)
	require.Error(t, err)
}

// TestParseQQUserInfo prefers the largest QQ avatar and treats a nonzero
// ret as a provider failure.
func TestParseQQUserInfo(t *testing.T) {
	nickname, avatar, err := ParseQQUserInfo(`{"ret":0,"nickname":"Whale","figureurl_qq_1":"http://img/40","figureurl_qq_2":"http://img/100"}`)
	require.NoError(t, err)
	require.Equal(t, "Whale", nickname)
	require.Equal(t, "http://img/100", avatar)

	_, _, err = ParseQQUserInfo(`{"ret":-1,"msg":"error"}`)
	require.Error(t, err)
}

// TestQQClientAgainstDouble runs the full exchange against a local
// provider: the token grant succeeds, the identity reply arrives as
// JSONP carrying a unionid, and the free-form nickname falls back to the
// generated username while the subject prefers the unionid.
func TestQQClientAgainstDouble(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "CODE-1", r.URL.Query().Get("code"))
		require.Equal(t, "json", r.URL.Query().Get("fmt"))
		_, _ = w.Write([]byte(`{"access_token":"AT-9","expires_in":7776000,"refresh_token":"RT-9"}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "1", r.URL.Query().Get("unionid"))
		_, _ = w.Write([]byte(`callback( {"client_id":"102","openid":"OID-9","unionid":"UID-9"} );`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "OID-9", r.URL.Query().Get("openid"))
		_, _ = w.Write([]byte(`{"ret":0,"nickname":"鲸鱼娘","figureurl_qq_2":"http://img/100"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	options := QQOptions{
		ClientID:     "102",
		ClientSecret: "secret",
		TokenURL:     server.URL + "/token",
		MeURL:        server.URL + "/me",
		UserInfoURL:  server.URL + "/userinfo",
	}
	client := QQClient{}
	token, err := client.ExchangeCode(context.Background(), options, "CODE-1", "https://site.example/api/v1/auth/oauth/qq/callback")
	require.NoError(t, err)
	require.Equal(t, "AT-9", token.AccessToken)

	email, username, subject, displayName, avatar, err := client.FetchUserInfo(context.Background(), options, token)
	require.NoError(t, err)
	require.Equal(t, "", email)
	require.Equal(t, "UID-9", subject)
	require.Equal(t, "鲸鱼娘", displayName)
	require.Equal(t, identity.QQFallbackUsername("UID-9"), username)
	require.Equal(t, "http://img/100", avatar)
}

// TestQQExchangeCodeSurfacesProviderError returns the typed exchange
// error when the grant reply carries a provider code instead of a token.
func TestQQExchangeCodeSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":100020,"msg":"code is reused error"}`))
	}))
	defer server.Close()

	options := QQOptions{ClientID: "102", ClientSecret: "secret", TokenURL: server.URL}
	_, err := QQExchangeCode(context.Background(), options, "CODE-X", "https://site.example/callback")
	require.Error(t, err)
	var exchangeErr *QQTokenExchangeError
	require.True(t, errors.As(err, &exchangeErr))
	require.True(t, strings.Contains(exchangeErr.Body, "100020"))
}
