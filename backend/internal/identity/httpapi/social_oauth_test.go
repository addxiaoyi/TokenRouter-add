package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/pkg/oauthpkce"
)

// social_oauth_test.go exercises browser authorization admission and callback
// rejection before provider exchange or account writes. The tests retain real
// cookie encoding, fixed provider endpoints, and PKCE generation while avoiding
// external authorization servers and database dependencies.

// socialPendingStore captures the pending draft without performing account mutations.
type socialPendingStore struct {
	identity.PendingStore                                        // unused continuation methods fail if invoked
	input                 identity.CreatePendingAuthSessionInput // captured persistent session draft
}

// socialPendingDatabase resolves only the provider tuple requested by the callback.
type socialPendingDatabase struct {
	identity.PendingDatabase                                 // unused account operations fail if invoked
	owner                    *identity.User                  // optional existing identity owner
	key                      identity.PendingAuthIdentityKey // observed stable identity lookup
}

// socialProofClient returns a verified fixture profile through the provider boundary.
type socialProofClient struct {
	profile identity.SocialOAuthProfile // authenticated provider profile fixture
}

// socialTotpPolicy enables authenticator checks while leaving unused policy reads unavailable.
type socialTotpPolicy struct{ SessionHTTPSettings }

// IsTotpEnabled enables the fixture's local second-factor policy for every request.
func (socialTotpPolicy) IsTotpEnabled(context.Context) bool { return true }

// CreatePendingSession captures the draft and supplies the cookie publication token.
func (s *socialPendingStore) CreatePendingSession(_ context.Context, input identity.CreatePendingAuthSessionInput) (*identity.PendingAuthSession, error) {
	s.input = input
	return &identity.PendingAuthSession{SessionToken: "pending-token"}, nil
}

// HasDatabase makes the fixture's pending store available to callback creation.
func (*socialPendingDatabase) HasDatabase() bool { return true }

// FindOAuthIdentityUser retains the exact tuple and returns its configured owner.
func (d *socialPendingDatabase) FindOAuthIdentityUser(_ context.Context, key identity.PendingAuthIdentityKey) (*identity.User, error) {
	d.key = key
	return d.owner, nil
}

// Resolve returns the fixture's authenticated profile without using network credentials.
func (s socialProofClient) Resolve(context.Context, identity.SocialOAuthOptions, string, string, string) (*identity.SocialOAuthProfile, error) {
	return &s.profile, nil
}

// socialStartFixture constructs configured social handlers around the shared CAPTCHA fixture.
// Optional CAPTCHA controls anonymous starts. Binding retains a real signer,
// while absent account readers ensure unauthenticated targets fail before cookie publication.
func socialStartFixture(captcha bool) *SocialHandler {
	auth, _ := newOAuthCaptchaTestHandler(captcha)
	binding := NewOAuthBindHandler(auth.Session, identity.NewOAuthBindingSigner("social-test-signing-key"))
	return NewSocialHandler(auth.Pending, binding, nil, func(_ context.Context, provider string) (identity.SocialOAuthOptions, error) {
		return identity.SocialOAuthOptions{Provider: provider, ClientID: "client", ClientSecret: "private"}, nil
	})
}

// TestSocialStartUsesPublicOriginAndPKCE checks each provider's authorization
// URL and HttpOnly ceremony cookies. State, callback, and verifier stay isolated
// by provider, and secrets never enter the browser redirect.
func TestSocialStartUsesPublicOriginAndPKCE(t *testing.T) {
	for _, provider := range []string{identity.SocialTelegram, identity.SocialDiscord} {
		t.Run(provider, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(socialProviderContext, provider)
			c.Request = httptest.NewRequest(http.MethodGet, "https://shop.example/api/v1/auth/oauth/"+provider+"/start?redirect=%2Fprofile", nil)
			socialStartFixture(false).SocialOAuthStart(c)
			require.Equal(t, http.StatusFound, recorder.Code)
			authorize, err := url.Parse(recorder.Header().Get("Location"))
			require.NoError(t, err)
			require.NotContains(t, authorize.String(), "private")
			query := authorize.Query()
			require.Equal(t, "https://shop.example/api/v1/auth/oauth/"+provider+"/callback", query.Get("redirect_uri"))
			require.Equal(t, "S256", query.Get("code_challenge_method"))
			cookies := map[string]string{}
			for _, cookie := range recorder.Result().Cookies() {
				value, err := DecodeCookieValue(cookie.Value)
				require.NoError(t, err)
				cookies[cookie.Name] = value
				require.True(t, cookie.HttpOnly)
				require.True(t, cookie.Secure)
			}
			require.Equal(t, query.Get("state"), cookies[provider+"_oauth_state"])
			require.Equal(t, oauthpkce.Challenge(cookies[provider+"_oauth_verifier"]), query.Get("code_challenge"))
			if provider == identity.SocialTelegram {
				require.Equal(t, "oauth.telegram.org", authorize.Host)
				require.Equal(t, "openid profile", query.Get("scope"))
				require.Equal(t, cookies[provider+"_oauth_nonce"], query.Get("nonce"))
			} else {
				require.Equal(t, "discord.com", authorize.Host)
				require.Equal(t, "identify email", query.Get("scope"))
			}
		})
	}
}

// TestSocialStartRejectsUnprovedBindingAndCaptcha ensures admission failures
// happen before authorization cookies are issued. A query intent cannot replace
// either the anonymous CAPTCHA proof or authenticated current-user binding proof.
func TestSocialStartRejectsUnprovedBindingAndCaptcha(t *testing.T) {
	for _, path := range []string{"start", "bind/start"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set(socialProviderContext, identity.SocialTelegram)
		c.Request = httptest.NewRequest(http.MethodGet, "https://shop.example/api/v1/auth/oauth/telegram/"+path+"?intent=bind_current_user", nil)
		socialStartFixture(true).SocialOAuthStart(c)
		require.GreaterOrEqual(t, recorder.Code, 400)
		require.Empty(t, recorder.Header().Get("Location"))
		require.Empty(t, recorder.Header().Values("Set-Cookie"))
	}
}

// TestSocialCallbackRejectsStateAndIssuer checks forged, missing, cross-provider,
// and already-cleared ceremonies. Invalid requests stop before the nil upstream
// client could be invoked and expire provider-local proof cookies.
func TestSocialCallbackRejectsStateAndIssuer(t *testing.T) {
	for _, variant := range []string{"missing", "mismatch", "cross_provider", "wrong_issuer", "missing_issuer"} {
		t.Run(variant, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(socialProviderContext, identity.SocialDiscord)
			query := url.Values{"code": {"code"}, "state": {"proof"}, "iss": {"https://discord.com"}}
			if variant == "wrong_issuer" {
				query.Set("iss", "https://other.example")
			}
			if variant == "missing_issuer" {
				query.Del("iss")
			}
			c.Request = httptest.NewRequest(http.MethodGet, "https://shop.example/api/v1/auth/oauth/discord/callback?"+query.Encode(), nil)
			cookieName := "discord_oauth_state"
			if variant == "cross_provider" {
				cookieName = "telegram_oauth_state"
			}
			state := "proof"
			if variant == "mismatch" {
				state = "other"
			}
			if variant != "missing" {
				c.Request.AddCookie(&http.Cookie{Name: cookieName, Value: EncodeCookieValue(state)})
			}
			socialStartFixture(false).SocialOAuthCallback(c)
			require.Equal(t, http.StatusFound, recorder.Code)
			want := "invalid_state"
			if variant == "wrong_issuer" || variant == "missing_issuer" {
				want = "invalid_issuer"
			}
			require.Contains(t, recorder.Header().Get("Location"), want)
			for _, cookie := range recorder.Result().Cookies() {
				require.Less(t, cookie.MaxAge, 0)
			}
		})
	}
}

// TestSocialCallbackPreservesIdentityAndBindTarget distinguishes new subjects,
// persisted owners, and signed current-account binding. Matching email hints never
// trigger local email lookup or override the selected binding target.
func TestSocialCallbackPreservesIdentityAndBindTarget(t *testing.T) {
	for _, provider := range []string{identity.SocialTelegram, identity.SocialDiscord} {
		for _, intent := range []string{"new", "existing", "bind"} {
			t.Run(provider+"/"+intent, func(t *testing.T) {
				handler := socialStartFixture(false)
				handler.authService = identity.NewAuthService(&identity.AuthDependencies{}, nil)
				store := &socialPendingStore{}
				database := &socialPendingDatabase{}
				if intent == "existing" {
					database.owner = &identity.User{ID: 41, Email: "owner@example.com"}
				}
				handler.flow = &identity.PendingFlow{Store: store, Database: database}
				handler.client = socialProofClient{profile: identity.SocialOAuthProfile{Subject: "123456789", Email: "hint@example.com", DisplayName: "Display"}}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set(socialProviderContext, provider)
				c.Request = httptest.NewRequest(http.MethodGet, "https://shop.example/api/v1/auth/oauth/"+provider+"/callback?code=code&state=proof&iss=https%3A%2F%2Fdiscord.com", nil)
				ceremony := map[string]string{"state": "proof", "verifier": "verifier", "nonce": "nonce", "callback": "https://shop.example/api/v1/auth/oauth/" + provider + "/callback", "redirect": "/profile", "intent": OauthIntentLogin}
				if intent == "bind" {
					ceremony["intent"] = OauthIntentBindCurrentUser
					target, err := handler.binding.Signer.Sign(53)
					require.NoError(t, err)
					ceremony["target"] = target
				}
				for field, value := range ceremony {
					c.Request.AddCookie(&http.Cookie{Name: socialCookieName(provider, field), Value: EncodeCookieValue(value)})
				}
				browserRecorder := httptest.NewRecorder()
				browserContext, _ := gin.CreateTestContext(browserRecorder)
				SetOAuthPendingBrowserCookie(browserContext, "browser-proof", true)
				for _, cookie := range browserRecorder.Result().Cookies() {
					c.Request.AddCookie(cookie)
				}
				handler.SocialOAuthCallback(c)
				require.Equal(t, "/auth/"+provider+"/callback", recorder.Header().Get("Location"))
				require.Equal(t, provider, store.input.Identity.ProviderType)
				require.Equal(t, provider, store.input.Identity.ProviderKey)
				require.Equal(t, "123456789", store.input.Identity.ProviderSubject)
				require.Equal(t, "browser-proof", store.input.BrowserSessionKey)
				if intent == "new" {
					require.Nil(t, store.input.TargetUserID)
					payload, ok := store.input.LocalFlowState[OauthCompletionResponseKey].(map[string]any)
					require.True(t, ok)
					require.Equal(t, identity.OAuthPendingChoiceStep, payload["step"])
				} else {
					require.NotNil(t, store.input.TargetUserID)
					want := int64(41)
					if intent == "bind" {
						want = 53
					}
					require.Equal(t, want, *store.input.TargetUserID)
				}
			})
		}
	}
}

// TestSocialTotpFailsClosedWithoutService reproduces a protected account while
// challenge storage is unavailable. The handler returns an error instead of
// allowing provider proof to bypass the local authenticator requirement.
func TestSocialTotpFailsClosedWithoutService(t *testing.T) {
	handler := &PendingHandler{SessionHandler: &SessionHandler{settingSvc: socialTotpPolicy{}}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	require.True(t, handler.requireSocialLoginTotp(c, &identity.PendingAuthSession{ProviderType: identity.SocialTelegram}, &identity.User{ID: 1, TotpEnabled: true}))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "access_token")
}
