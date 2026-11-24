package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// social_oauth_test.go checks authenticated identity extraction and code-exchange
// parameters with local provider responses. Signed Telegram tokens exercise the
// real cryptographic verifier, while Discord fixtures keep verified email hints
// separate from the stable user identifier.

// TestSocialTelegramIdentityProof rejects invalid identity claims despite a valid
// browser nonce. Only the correctly signed, current, intended-audience proof exposes
// the stable subject and signed profile fields.
func TestSocialTelegramIdentityProof(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(OidcJWKSet{Keys: []OidcJWK{buildRSAJWK("social", &key.PublicKey)}}))
	}))
	t.Cleanup(server.Close)
	cfg, err := (identity.SocialOAuthOptions{Provider: identity.SocialTelegram, ClientID: "123"}).ProtocolOptions()
	require.NoError(t, err)
	cfg.JWKSURL = server.URL
	for _, variant := range []string{"valid", "nonce", "issuer", "audience", "expired", "missing_exp", "missing_iat", "future_iat", "subject", "signature", "algorithm"} {
		t.Run(variant, func(t *testing.T) {
			claims := jwt.MapClaims{"iss": "https://oauth.telegram.org", "aud": "123", "sub": "456", "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": "proof", "name": "Sample", "picture": "https://example.org/avatar.png"}
			signer := any(key)
			method := jwt.SigningMethod(jwt.SigningMethodRS256)
			switch variant {
			case "nonce":
				claims["nonce"] = "other"
			case "issuer":
				claims["iss"] = "https://attacker.example"
			case "audience":
				claims["aud"] = "another-client"
			case "expired":
				claims["exp"] = time.Now().Add(-time.Minute).Unix()
			case "missing_exp":
				delete(claims, "exp")
			case "missing_iat":
				delete(claims, "iat")
			case "future_iat":
				claims["iat"] = time.Now().Add(time.Hour).Unix()
			case "subject":
				claims["sub"] = "nickname"
			case "signature":
				signer = otherKey
			case "algorithm":
				method, signer = jwt.SigningMethodHS256, []byte("wrong-key")
			}
			token := jwt.NewWithClaims(method, claims)
			token.Header["kid"] = "social"
			signed, err := token.SignedString(signer)
			require.NoError(t, err)
			profile, err := socialTelegramProfile(context.Background(), cfg, signed, "proof")
			if variant != "valid" {
				require.Error(t, err)
				require.Nil(t, profile)
				require.NotContains(t, err.Error(), signed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "456", profile.Subject)
			require.Equal(t, "Sample", profile.DisplayName)
			require.Equal(t, "https://example.org/avatar.png", profile.AvatarURL)
			require.Empty(t, profile.Email)
			_, err = socialTelegramProfile(context.Background(), cfg, signed, "")
			require.Error(t, err)
		})
	}
}

// TestSocialCodeExchangeCarriesPKCE checks both providers against a local token
// endpoint. It protects Basic client authentication and the original callback and
// verifier values without putting the secret into the form or URL.
func TestSocialCodeExchangeCarriesPKCE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		clientID, secret, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "client", clientID)
		require.Equal(t, "secret", secret)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "code", r.Form.Get("code"))
		require.Equal(t, "verifier", r.Form.Get("code_verifier"))
		require.Equal(t, "https://shop.example/callback", r.Form.Get("redirect_uri"))
		require.Empty(t, r.Form.Get("client_secret"))
		_, err := w.Write([]byte(`{"access_token":"access","token_type":"Bearer","id_token":"signed"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	for _, provider := range []string{identity.SocialTelegram, identity.SocialDiscord} {
		cfg, err := (identity.SocialOAuthOptions{Provider: provider, ClientID: "client", ClientSecret: "secret"}).ProtocolOptions()
		require.NoError(t, err)
		cfg.TokenURL = server.URL
		_, err = OidcExchangeCode(context.Background(), cfg, "code", "https://shop.example/callback", "verifier")
		require.NoError(t, err)
	}
}

// TestSocialDiscordProfileRequiresStableIdentity checks email verification and
// avatar components independently. A nickname or path-shaped account identifier
// never becomes a usable authentication subject.
func TestSocialDiscordProfileRequiresStableIdentity(t *testing.T) {
	profile, err := socialDiscordProfile([]byte(`{"id":"123456789","username":"sample","email":"user@example.com","verified":false,"avatar":"../../elsewhere"}`))
	require.NoError(t, err)
	require.Equal(t, "sample", profile.DisplayName)
	require.Empty(t, profile.Email)
	require.Empty(t, profile.AvatarURL)
	profile, err = socialDiscordProfile([]byte(`{"id":"123456789","username":"sample","global_name":"Display","email":"user@example.com","verified":true,"avatar":"0123456789abcdef0123456789abcdef"}`))
	require.NoError(t, err)
	require.Equal(t, "user@example.com", profile.Email)
	require.Equal(t, "Display", profile.DisplayName)
	require.Equal(t, "https://cdn.discordapp.com/avatars/123456789/0123456789abcdef0123456789abcdef.png", profile.AvatarURL)
	for _, body := range []string{`{}`, `{"id":"../123"}`, `{"id":123}`, `invalid`} {
		_, err := socialDiscordProfile([]byte(body))
		require.Error(t, err)
	}
}
