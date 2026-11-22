package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/logging"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
	"github.com/TokenFlux/TokenRouter/internal/pkg/oauthpkce"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// qq_oauth.go handles QQ browser authorization, account continuation,
// and callback redirects.
// QQ login uses the stable provider subject for account identity and a
// synthetic email for binding, because QQ互联 never supplies an email
// address. Start captures browser proof and login or current-user intent;
// callback verifies that state before asking the identity flow to select
// an account.
//
// Existing provider ownership becomes a pending login target, and
// current-user binding retains its signed target cookie. Eligible direct
// signup saves its provider binding before delivering credentials in a
// fragment; invitation-required results and local email policies continue
// in pending state instead.

// QQ browser cookie values, intent labels, and local defaults carry
// authorization state and the selected account continuation through
// provider navigation.
const (
	QQOAuthCookiePath         = "/api/v1/auth/oauth/qq"
	QQOAuthStateCookieName    = "qq_oauth_state"     // QQ authorization state cookie
	QQOAuthRedirectCookie     = "qq_oauth_redirect"  // QQ local return cookie
	QQOAuthIntentCookieName   = "qq_oauth_intent"    // QQ account intent cookie
	QQOAuthBindUserCookieName = "qq_oauth_bind_user" // signed target account cookie
	QQOAuthCookieMaxAgeSec    = 10 * 60              // seconds
	QQOAuthDefaultRedirectTo  = "/dashboard"
	QQOAuthDefaultFrontendCB  = "/auth/qq/callback"
)

// QQHandler manages subject-based QQ login and browser current-user
// binding continuations.
type QQHandler struct {
	*PendingHandler                                                        // shared browser pending authentication
	binding         *OAuthBindHandler                                      // signed current-account binding helper
	client          identity.QQOAuthClient                                 // QQ authorization and profile client
	loadConfig      func(context.Context) (identity.QQOAuthOptions, error) // current QQ configuration callback
}

// CompleteQQOAuthRequest carries invitation and profile choices for
// legacy pending signup completion.
type CompleteQQOAuthRequest struct {
	InvitationCode string `json:"invitation_code" binding:"required"` // required registration invitation code
	AffCode        string `json:"aff_code,omitempty"`                 // optional signup affiliate code
	// AdoptDisplayName and AdoptAvatar keep omitted choices as nil and
	// supplied false values as refusals, matching the shared adoption
	// decision helper.
	AdoptDisplayName *bool `json:"adopt_display_name,omitempty"` // optional provider-name adoption choice
	AdoptAvatar      *bool `json:"adopt_avatar,omitempty"`       // optional provider-avatar adoption choice
}

// NewQQHandler retains shared pending and binding handlers, provider
// client, and configuration reader. Initialize dependencies for the
// registered login and binding paths before serving authorization
// requests.
func NewQQHandler(p *PendingHandler, b *OAuthBindHandler, client identity.QQOAuthClient, load func(context.Context) (identity.QQOAuthOptions, error)) *QQHandler {
	return &QQHandler{p, b, client, load}
}

// QQOAuthStart checks applicable action CAPTCHA and records browser state
// before returning the authorize URL. It normalizes intent, captures
// signup context, and signs the current-account target when binding is
// selected.
//
// A browser key replaces prior pending-session proof. Return paths use
// the validated local default when unusable. Cookies are emitted before
// later binding or URL construction can fail, so a failed start can leave
// partial browser state. POST receives the authorize URL as JSON; other
// accepted methods redirect.
func (h *QQHandler) QQOAuthStart(c *gin.Context) {
	if !h.RequireActionCaptchaForOAuthLoginStart(c) {
		return
	}
	cfg, err := h.loadConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	state, err := oauthpkce.Verifier()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_STATE_GEN_FAILED", "failed to generate oauth state").WithCause(err))
		return
	}

	redirectTo := SanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirectTo == "" {
		redirectTo = QQOAuthDefaultRedirectTo
	}

	browserSessionKey, err := GenerateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}

	secureCookie := IsRequestHTTPS(c)
	// Replace browser proof and expire its prior pending token beside the
	// new QQ start context. Binding work follows, so its failures can
	// leave these already emitted cookie headers.
	QQSetCookie(c, QQOAuthStateCookieName, EncodeCookieValue(state), QQOAuthCookieMaxAgeSec, secureCookie)
	QQSetCookie(c, QQOAuthRedirectCookie, EncodeCookieValue(redirectTo), QQOAuthCookieMaxAgeSec, secureCookie)
	intent := NormalizeOAuthIntent(c.Query("intent"))
	QQSetCookie(c, QQOAuthIntentCookieName, EncodeCookieValue(intent), QQOAuthCookieMaxAgeSec, secureCookie)
	CaptureOAuthPromoCode(c, secureCookie)
	SetOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	ClearOAuthPendingSessionCookie(c, secureCookie)
	if intent == OauthIntentBindCurrentUser {
		bindCookieValue, err := h.binding.BuildOAuthBindUserCookieFromContext(c)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		QQSetCookie(c, QQOAuthBindUserCookieName, EncodeCookieValue(bindCookieValue), QQOAuthCookieMaxAgeSec, secureCookie)
	} else {
		QQClearCookie(c, QQOAuthBindUserCookieName, secureCookie)
	}

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured"))
		return
	}

	authURL, err := BuildQQAuthorizeURL(cfg, state, redirectURI)
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BUILD_URL_FAILED", "failed to build oauth authorization url").WithCause(err))
		return
	}

	RespondOAuthStart(c, authURL)
}

// QQOAuthCallback verifies state and browser proof before resolving the
// external subject. Required account dependencies are supplied by handler
// assembly.
//
// The subject-derived synthetic address feeds binding identity because QQ
// supplies no email at all. Signed current-user targets and existing
// provider owners become pending sessions. Otherwise eligible direct
// signup can persist an account and binding before fragment delivery;
// invitation-required results and local email policies become pending
// choice. Callback cleanup is registered after required code/state
// input, leaving earlier failure cookies until expiry.
func (h *QQHandler) QQOAuthCallback(c *gin.Context) {
	cfg, cfgErr := h.loadConfig(c.Request.Context())
	if cfgErr != nil {
		response.ErrorFrom(c, cfgErr)
		return
	}

	frontendCallback := strings.TrimSpace(cfg.FrontendRedirectURL)
	if frontendCallback == "" {
		frontendCallback = QQOAuthDefaultFrontendCB
	}

	if providerErr := strings.TrimSpace(c.Query("error")); providerErr != "" {
		RedirectOAuthError(c, frontendCallback, "provider_error", providerErr, c.Query("error_description"))
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		RedirectOAuthError(c, frontendCallback, "missing_params", "missing code/state", "")
		return
	}

	secureCookie := IsRequestHTTPS(c)
	// Once code and state are present, expire QQ callback and promotion
	// cookies on every later exit. Pending browser proof remains available
	// for the continuation created by successful resolution.
	defer func() {
		QQClearCookie(c, QQOAuthStateCookieName, secureCookie)
		QQClearCookie(c, QQOAuthRedirectCookie, secureCookie)
		QQClearCookie(c, QQOAuthIntentCookieName, secureCookie)
		QQClearCookie(c, QQOAuthBindUserCookieName, secureCookie)
		ClearOAuthPromoCodeCookie(c, secureCookie)
	}()

	expectedState, err := ReadCookieDecoded(c, QQOAuthStateCookieName)
	if err != nil || expectedState == "" || state != expectedState {
		RedirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth state", "")
		return
	}

	// An unreadable return cookie contributes empty text, then local-path
	// validation selects the dashboard default. Browser proof is checked
	// separately and cannot be replaced by that return value.
	redirectTo, _ := ReadCookieDecoded(c, QQOAuthRedirectCookie)
	redirectTo = SanitizeFrontendRedirectPath(redirectTo)
	if redirectTo == "" {
		redirectTo = QQOAuthDefaultRedirectTo
	}
	// Read failures produce empty browser proof, so missing or malformed
	// cookies are rejected before provider exchange and pending state
	// creation.
	browserSessionKey, _ := ReadOAuthPendingBrowserCookie(c)
	if strings.TrimSpace(browserSessionKey) == "" {
		RedirectOAuthError(c, frontendCallback, "missing_browser_session", "missing oauth browser session", "")
		return
	}
	// An unreadable intent has no binding label. Normalization selects
	// login, so a leftover bind-target cookie cannot by itself select the
	// current-user binding branch.
	intent, _ := ReadCookieDecoded(c, QQOAuthIntentCookieName)
	intent = NormalizeOAuthIntent(intent)

	redirectURI := strings.TrimSpace(cfg.RedirectURL)
	if redirectURI == "" {
		RedirectOAuthError(c, frontendCallback, "config_error", "oauth redirect url not configured", "")
		return
	}

	tokenResp, err := h.client.ExchangeCode(c.Request.Context(), cfg, code, redirectURI)
	if err != nil {
		description := ""
		var exchangeErr *identity.QQTokenExchangeError
		if errors.As(err, &exchangeErr) && exchangeErr != nil {
			// Bound response bodies to two KiB at complete UTF-8 prefixes
			// for logs.
			logging.LegacyPrintf("handler.qq_oauth",
				"[QQ OAuth] token exchange failed: status=%d provider_error=%q provider_description=%q body=%s",
				exchangeErr.StatusCode,
				exchangeErr.ProviderError,
				exchangeErr.ProviderDescription,
				logredact.TruncateUTF8Value(exchangeErr.Body, 2048),
			)
			description = exchangeErr.Error()
		} else {
			logging.LegacyPrintf("handler.qq_oauth", "[QQ OAuth] token exchange failed: %v", err)
			description = err.Error()
		}
		RedirectOAuthError(c, frontendCallback, "token_exchange_failed", "failed to exchange oauth code", OAuthSingleLine(description))
		return
	}

	_, username, subject, displayName, avatarURL, err := h.client.FetchUserInfo(c.Request.Context(), cfg, tokenResp)
	if err != nil {
		logging.LegacyPrintf("handler.qq_oauth", "[QQ OAuth] userinfo fetch failed: %v", err)
		RedirectOAuthError(c, frontendCallback, "userinfo_failed", "failed to fetch user info", "")
		return
	}

	// QQ supplies no email; use a stable subject-derived synthetic email
	// for binding identity.
	email := identity.OAuthQQSyntheticEmail(subject)
	identityKey := identity.PendingAuthIdentityKey{
		ProviderType:    "qq",
		ProviderKey:     identity.QQOAuthProviderKey,
		ProviderSubject: subject,
	}
	upstreamClaims := map[string]any{
		"email":                  email,
		"username":               username,
		"subject":                subject,
		"suggested_display_name": displayName,
		"suggested_avatar_url":   avatarURL,
	}
	if intent == OauthIntentBindCurrentUser {
		targetUserID, err := h.binding.ReadOAuthBindUserIDFromCookie(c, QQOAuthBindUserCookieName)
		if err != nil {
			RedirectOAuthError(c, frontendCallback, "invalid_state", "invalid oauth bind target", "")
			return
		}
		if err := h.CreateOAuthPendingSession(c, OauthPendingSessionPayload{
			Intent:                 OauthIntentBindCurrentUser,
			Identity:               identityKey,
			TargetUserID:           &targetUserID,
			ResolvedEmail:          email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse: map[string]any{
				"redirect": redirectTo,
			},
		}); err != nil {
			RedirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth bind", "")
			return
		}
		RedirectToFrontendCallback(c, frontendCallback)
		return
	}

	existingIdentityUser, err := h.flow.Database.FindOAuthIdentityUser(c.Request.Context(), identityKey)
	if err != nil {
		RedirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
		return
	}
	if existingIdentityUser != nil {
		if err := h.CreateOAuthPendingSession(c, OauthPendingSessionPayload{
			Intent:                 OauthIntentLogin,
			Identity:               identityKey,
			TargetUserID:           &existingIdentityUser.ID,
			ResolvedEmail:          existingIdentityUser.Email,
			RedirectTo:             redirectTo,
			BrowserSessionKey:      browserSessionKey,
			UpstreamIdentityClaims: upstreamClaims,
			CompletionResponse: map[string]any{
				"redirect": redirectTo,
			},
		}); err != nil {
			RedirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
			return
		}
		RedirectToFrontendCallback(c, frontendCallback)
		return
	}

	emailVerificationRequired := h != nil && h.authService != nil && h.authService.IsEmailVerifyEnabled(c.Request.Context())
	forceEmailOnSignup := h.isForceEmailOnThirdPartySignup(c.Request.Context())
	if !emailVerificationRequired && !forceEmailOnSignup {
		if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
			RedirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
		// Authentication may issue a pair before external-subject binding
		// succeeds. Keep that binding ahead of fragment delivery; failure
		// withholds the response after earlier account or credential work.
		tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(c.Request.Context(), email, username, "", "", ReadOAuthPromoCode(c), "qq")
		if err == nil {
			if err := h.flow.Database.ApplyBinding(c.Request.Context(), identity.PendingBinding{Session: &identity.PendingAuthSession{
				Intent:                 OauthIntentLogin,
				ProviderType:           identityKey.ProviderType,
				ProviderKey:            identityKey.ProviderKey,
				ProviderSubject:        identityKey.ProviderSubject,
				ResolvedEmail:          email,
				UpstreamIdentityClaims: upstreamClaims,
			}, OverrideUserID: &user.ID, ForceBind: true}); err != nil {
				RedirectOAuthError(c, frontendCallback, "session_error", "failed to bind oauth identity", "")
				return
			}
			h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
			ClearOAuthPendingSessionCookie(c, secureCookie)
			ClearOAuthPendingBrowserCookie(c, secureCookie)
			RedirectOAuthTokenPair(c, frontendCallback, tokenPair, redirectTo)
			return
		}
		if !errors.Is(err, identity.ErrOAuthInvitationRequired) {
			RedirectOAuthError(c, frontendCallback, "session_error", infraerrors.Reason(err), infraerrors.Message(err))
			return
		}
	}
	if err := h.CreateQQChoiceSession(
		c,
		identityKey,
		email,
		redirectTo,
		browserSessionKey,
		upstreamClaims,
		emailVerificationRequired,
		forceEmailOnSignup,
	); err != nil {
		RedirectOAuthError(c, frontendCallback, "session_error", "failed to continue oauth login", "")
		return
	}
	RedirectToFrontendCallback(c, frontendCallback)
}

// CompleteQQOAuthRegistration handles a browser-bound legacy invitation
// completion or returns updated account-choice state. The flow first
// reevaluates local email requirements and backend-mode registration
// admission.
//
// A usable synthetic email and username plus available provider identity
// are required before creating or resolving the account. Saved adoption
// choice then enters the combined adoption-and-consumption database
// operation. Credentials may already have been issued by authentication
// before that later operation fails. Successful completion records
// login, expires pending cookies, and writes a top-level token response.
func (h *QQHandler) CompleteQQOAuthRegistration(c *gin.Context) {
	var req CompleteQQOAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST", "message": err.Error()})
		return
	}

	secureCookie := IsRequestHTTPS(c)
	sessionToken, err := ReadOAuthPendingSessionCookie(c)
	if err != nil {
		ClearOAuthPendingSessionCookie(c, secureCookie)
		ClearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, identity.ErrPendingAuthSessionNotFound)
		return
	}
	browserSessionKey, err := ReadOAuthPendingBrowserCookie(c)
	if err != nil {
		ClearOAuthPendingSessionCookie(c, secureCookie)
		ClearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, identity.ErrPendingAuthBrowserMismatch)
		return
	}
	pendingSvc, err := h.pendingIdentityService()
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	session, err := pendingSvc.GetBrowserSession(c.Request.Context(), sessionToken, browserSessionKey)
	if err != nil {
		ClearOAuthPendingSessionCookie(c, secureCookie)
		ClearOAuthPendingBrowserCookie(c, secureCookie)
		response.ErrorFrom(c, err)
		return
	}
	if err := EnsurePendingOAuthCompleteRegistrationSession(session); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updatedSession, handled, err := h.flow.LegacyRegistrationStatus(c.Request.Context(), session, h.authService != nil && h.authService.IsEmailVerifyEnabled(c.Request.Context()), h.isForceEmailOnThirdPartySignup(c.Request.Context()))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	} else if handled {
		c.JSON(http.StatusOK, BuildPendingOAuthSessionStatusPayload(updatedSession))
		return
	}
	session = updatedSession
	if err := h.ensureBackendModeAllowsNewUserLogin(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	email := strings.TrimSpace(session.ResolvedEmail)
	username := PendingSessionStringValue(session.UpstreamIdentityClaims, "username")
	if email == "" || username == "" {
		response.ErrorFrom(c, infraerrors.BadRequest("PENDING_AUTH_SESSION_INVALID", "pending auth registration context is invalid"))
		return
	}

	if !h.flow.Available() {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("PENDING_AUTH_NOT_READY", "pending auth service is not ready"))
		return
	}
	if err := h.flow.Database.EnsureRegistrationIdentityAvailable(c.Request.Context(), session); err != nil {
		RespondPendingOAuthBindingApplyError(c, err)
		return
	}
	decision, err := h.ensurePendingOAuthAdoptionDecision(c, session.ID, OauthAdoptionDecisionRequest{
		AdoptDisplayName: req.AdoptDisplayName,
		AdoptAvatar:      req.AdoptAvatar,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	tokenPair, user, err := h.authService.LoginOrRegisterOAuthWithTokenPairAndPromoCode(c.Request.Context(), email, username, req.InvitationCode, req.AffCode, PendingOAuthPromoCode(session), "qq")
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// Authentication has already produced credentials. Persist adoption
	// and the browser claim together before returning them; transaction
	// failure leaves that earlier credential issuance outside this
	// rollback scope.
	if err := h.flow.Database.ApplyAdoptionAndConsume(c.Request.Context(), session, decision, user.ID); err != nil {
		RespondPendingOAuthBindingApplyError(c, err)
		return
	}
	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	ClearOAuthPendingSessionCookie(c, secureCookie)
	ClearOAuthPendingBrowserCookie(c, secureCookie)

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"expires_in":    tokenPair.ExpiresIn,
		"token_type":    "Bearer",
	})
}

// BuildQQAuthorizeURL encodes client, redirect, and state into the
// configured authorize URL. QQ互联 selects its default user-info scope
// when no scope parameter is supplied; unrelated existing query values
// remain in the parsed URL.
func BuildQQAuthorizeURL(cfg identity.QQOAuthOptions, state string, redirectURI string) (string, error) {
	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// CreateQQChoiceSession prepares account-choice state from the synthetic
// address and local email policies, then stores the pending draft. The
// identity helper owns policy precedence, and persistence errors remain
// visible to the callback caller.
func (h *QQHandler) CreateQQChoiceSession(
	c *gin.Context,
	identityKey identity.PendingAuthIdentityKey,
	syntheticEmail string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
	emailVerificationRequired bool,
	forceEmailOnSignup bool,
) error {
	draft := identity.PrepareQQChoice(identityKey, syntheticEmail, redirectTo, browserSessionKey, upstreamClaims, emailVerificationRequired, forceEmailOnSignup)
	return h.CreateOAuthPendingSession(c, draft)
}

// QQSetCookie emits the supplied encoded value at the QQ browser cookie
// path. maxAgeSec is seconds; the caller supplies HTTPS state, and
// HttpOnly/SameSite Lax support provider callback navigation.
func QQSetCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     QQOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// QQClearCookie emits deletion of a QQ state cookie at its matching path.
// The caller supplies HTTPS state, while HttpOnly and SameSite Lax
// attributes match the browser continuation cookie.
func QQClearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     QQOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
