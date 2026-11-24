package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/oauthpkce"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// social_oauth.go carries Telegram and Discord browser authorization into shared
// pending login and account binding. Each provider has its own state, verifier,
// nonce, callback, and target cookies, preventing one provider's callback from
// consuming another provider's ceremony. Existing browser proof and pending
// storage still own the final one-time account decision.
//
// Current-user binding resolves an authenticated target before issuing provider
// navigation. Callback authentication uses state and PKCE, with Telegram nonce
// validation and Discord issuer checking, before resolving the stable subject.
// New subjects enter explicit account choice instead of matching users by email.
// The shared continuation enforces registration policy, account proof, TOTP,
// transactional uniqueness, and protection against unlinking the final login method.

const (
	// Social cookies use the existing pending ceremony lifetime and provider-local
	// paths, while this request context key carries only a trusted route selection.
	socialProviderContext = "identity.social_provider"
)

// SocialHandler coordinates fixed social providers through shared pending authentication.
type SocialHandler struct {
	*PendingHandler                            // shared pending account continuation
	binding         *OAuthBindHandler          // authenticated target cookie signer
	client          identity.SocialOAuthClient // upstream identity proof resolver
	// loadConfig reads the enabled provider for each request; failures prevent
	// browser navigation or account continuation with unavailable or incomplete settings.
	loadConfig func(context.Context, string) (identity.SocialOAuthOptions, error)
}

// NewSocialHandler retains provider exchange and shared account continuation dependencies.
// Supply initialized handlers and a configuration reader before registering its
// routes; successful callbacks create pending state without publishing local credentials.
func NewSocialHandler(
	pending *PendingHandler,
	binding *OAuthBindHandler,
	client identity.SocialOAuthClient,
	load func(context.Context, string) (identity.SocialOAuthOptions, error),
) *SocialHandler {
	return &SocialHandler{PendingHandler: pending, binding: binding, client: client, loadConfig: load}
}

// SocialOAuthStart records a provider-specific authorization ceremony after admission checks.
// Binding intent requires an authenticated target. Missing configuration, invalid
// callbacks, or random generation errors stop before the authorization URL is returned.
//
// The recorded callback is used again during code exchange, so configuration
// edits cannot substitute a different redirect mid-ceremony. Shared browser proof
// is replaced before navigation, invalidating any older pending continuation in
// this browser. State, verifier, nonce, and intent cookies are HttpOnly and scoped
// to the selected provider's backend path for the existing ten-minute lifetime.
func (h *SocialHandler) SocialOAuthStart(c *gin.Context) {
	provider := c.GetString(socialProviderContext)
	if !h.RequireActionCaptchaForOAuthLoginStart(c) {
		return
	}
	options, err := h.loadConfig(c.Request.Context(), provider)
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	protocol, err := options.ProtocolOptions()
	if err != nil {
		httpx.ErrorFrom(c, apperror.BadRequest("OAUTH_PROVIDER_INVALID", "unsupported login provider"))
		return
	}
	callback := options.RedirectURL
	if callback == "" {
		// An omitted registered callback follows the current browser API origin,
		// keeping same-site cookies usable behind the configured public reverse proxy.
		scheme := "http"
		if IsRequestHTTPS(c) {
			scheme = "https"
		}
		callback = scheme + "://" + c.Request.Host + "/api/v1/auth/oauth/" + provider + "/callback"
	}
	if !identity.SocialOAuthRedirectAllowed(callback) {
		httpx.ErrorFrom(c, apperror.BadRequest("OAUTH_CONFIG_INVALID", "login callback must use your public HTTPS address"))
		return
	}
	intent := NormalizeOAuthIntent(c.Query("intent"))
	target := ""
	if intent == OauthIntentBindCurrentUser {
		target, err = h.binding.BuildOAuthBindUserCookieFromContext(c)
		if err != nil {
			httpx.ErrorFrom(c, err)
			return
		}
	}
	state, err := oauthpkce.Verifier()
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	verifier, err := oauthpkce.Verifier()
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	nonce, err := oauthpkce.Verifier()
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	browser, err := GenerateOAuthPendingBrowserSession()
	if err != nil {
		httpx.ErrorFrom(c, err)
		return
	}
	redirect := SanitizeFrontendRedirectPath(c.Query("redirect"))
	if redirect == "" {
		// Invalid or absent return paths use the dashboard so provider redirects
		// cannot navigate to a caller-supplied external destination after authentication.
		redirect = "/dashboard"
	}
	// Publish browser proof together with the provider's ceremony fields before
	// redirecting; a subsequent callback must present the same browser and state.
	for key, value := range map[string]string{
		"state": state, "verifier": verifier, "nonce": nonce,
		"intent": intent, "target": target, "redirect": redirect, "callback": callback,
	} {
		setSocialCookie(c, provider, key, value, OauthPendingCookieMaxAgeSec)
	}
	SetOAuthPendingBrowserCookie(c, browser, IsRequestHTTPS(c))
	ClearOAuthPendingSessionCookie(c, IsRequestHTTPS(c))
	CaptureOAuthPromoCode(c, IsRequestHTTPS(c))
	authorize, err := url.Parse(protocol.AuthorizeURL)
	if err != nil {
		httpx.InternalError(c, "Authorization endpoint is invalid")
		return
	}
	query := url.Values{
		"response_type": {"code"}, "client_id": {options.ClientID},
		"redirect_uri": {callback}, "scope": {protocol.Scopes}, "state": {state},
		"code_challenge": {oauthpkce.Challenge(verifier)}, "code_challenge_method": {"S256"},
	}
	if provider == identity.SocialTelegram {
		query.Set("nonce", nonce)
	}
	authorize.RawQuery = query.Encode()
	RespondOAuthStart(c, authorize.String())
}

// SocialOAuthCallback verifies browser and provider proofs before preparing account choice.
// Existing subject ownership selects its saved user. Current-user binding retains
// the signed target, while a new subject must explicitly create or prove a local account.
//
// Callback cookies are cleared on every exit. Their deletion does not itself
// guarantee replay protection: upstream authorization codes and the shared pending
// store enforce single consumption. No provider token enters the redirect URL,
// and email is never used to select an existing account without local proof.
func (h *SocialHandler) SocialOAuthCallback(c *gin.Context) {
	provider := c.GetString(socialProviderContext)
	frontend := "/auth/" + provider + "/callback"
	// Expire all ceremony cookies on both provider errors and success; pending
	// browser proof remains available for the separate account-completion transaction.
	defer func() {
		for _, key := range []string{"state", "verifier", "nonce", "intent", "target", "redirect", "callback"} {
			setSocialCookie(c, provider, key, "", -1)
		}
		ClearOAuthPromoCodeCookie(c, IsRequestHTTPS(c))
	}()
	fail := func(code, message string) { RedirectOAuthError(c, frontend, code, message, "") }
	if c.Query("error") != "" {
		fail("provider_error", "authorization was not completed")
		return
	}
	state, stateErr := ReadCookieDecoded(c, socialCookieName(provider, "state"))
	code := strings.TrimSpace(c.Query("code"))
	if stateErr != nil || state == "" || code == "" || subtle.ConstantTimeCompare([]byte(state), []byte(c.Query("state"))) != 1 {
		fail("invalid_state", "login session expired or does not match this browser")
		return
	}
	options, err := h.loadConfig(c.Request.Context(), provider)
	if err != nil {
		fail("config_error", "login provider is unavailable")
		return
	}
	if issuer := c.Query("iss"); provider == identity.SocialDiscord && issuer != "https://discord.com" {
		fail("invalid_issuer", "authorization issuer does not match Discord")
		return
	}
	// Failed cookie decoding yields empty proof and is rejected below before any
	// upstream request. These reads cannot silently downgrade PKCE or nonce validation.
	verifier, _ := ReadCookieDecoded(c, socialCookieName(provider, "verifier"))
	nonce, _ := ReadCookieDecoded(c, socialCookieName(provider, "nonce"))
	callback, _ := ReadCookieDecoded(c, socialCookieName(provider, "callback"))
	browser, _ := ReadOAuthPendingBrowserCookie(c)
	if verifier == "" || nonce == "" || browser == "" || !identity.SocialOAuthRedirectAllowed(callback) {
		fail("invalid_state", "login proof is missing or expired")
		return
	}
	options.RedirectURL = callback
	profile, err := h.client.Resolve(c.Request.Context(), options, code, verifier, nonce)
	if err != nil || profile == nil || strings.TrimSpace(profile.Subject) == "" {
		fail("identity_verification_failed", "the provider account could not be verified")
		return
	}
	redirect, _ := ReadCookieDecoded(c, socialCookieName(provider, "redirect"))
	redirect = SanitizeFrontendRedirectPath(redirect)
	if redirect == "" {
		// A missing or invalid optional return path keeps the established dashboard
		// destination while the separately required identity and browser proofs remain enforced.
		redirect = "/dashboard"
	}
	key := identity.PendingAuthIdentityKey{ProviderType: provider, ProviderKey: provider, ProviderSubject: profile.Subject}
	claims := map[string]any{
		"username": profile.Username, "subject": profile.Subject,
		"suggested_display_name": profile.DisplayName, "suggested_avatar_url": profile.AvatarURL,
	}
	draft := identity.PrepareLinuxDoChoice(key, profile.Email, profile.Email, redirect, browser, claims, "", nil,
		h.authService.IsEmailVerifyEnabled(c.Request.Context()), h.isForceEmailOnThirdPartySignup(c.Request.Context()))
	intent, _ := ReadCookieDecoded(c, socialCookieName(provider, "intent"))
	if intent == OauthIntentBindCurrentUser {
		userID, err := h.binding.ReadOAuthBindUserIDFromCookie(c, socialCookieName(provider, "target"))
		if err != nil {
			fail("invalid_state", "the binding target could not be verified")
			return
		}
		draft.Intent = OauthIntentBindCurrentUser
		draft.TargetUserID = &userID
		draft.CompletionResponse = map[string]any{"redirect": redirect}
	} else {
		owner, err := h.flow.Database.FindOAuthIdentityUser(c.Request.Context(), key)
		if err != nil {
			fail("session_error", "account lookup failed")
			return
		}
		if owner != nil {
			draft.TargetUserID = &owner.ID
			draft.ResolvedEmail = owner.Email
			draft.CompletionResponse = map[string]any{"redirect": redirect}
		}
	}
	draft.CompletionResponse["provider"] = provider
	if err := h.CreateOAuthPendingSession(c, draft); err != nil {
		fail("session_error", "could not continue account authentication")
		return
	}
	RedirectToFrontendCallback(c, frontend)
}

// socialCookieName isolates each provider's ceremony fields within the browser cookie jar.
func socialCookieName(provider, field string) string { return provider + "_oauth_" + field }

// setSocialCookie stores or expires one provider ceremony value with HttpOnly protection.
// Its path matches the provider callback, and SameSite Lax permits the top-level
// return from the authorization server without exposing proof to frontend JavaScript.
func setSocialCookie(c *gin.Context, provider, field, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: socialCookieName(provider, field), Value: EncodeCookieValue(value),
		Path: "/api/v1/auth/oauth/" + provider, MaxAge: age,
		HttpOnly: true, Secure: IsRequestHTTPS(c), SameSite: http.SameSiteLaxMode,
	})
}

// requireSocialLoginTotp keeps a verified social login pending until local TOTP succeeds.
// It runs after the adoption decision is saved but before identity adoption or
// browser-session consumption. A true return means this method wrote the httpx.
//
// The challenge binds the account to the existing browser continuation, allowing
// Login2FA to consume that same session after checking the authenticator code.
// Current-user binding has no loginUser and keeps the authenticated account's
// session. Missing TOTP infrastructure fails closed for an enabled protected
// account instead of publishing an access token from provider proof alone.
func (h *PendingHandler) requireSocialLoginTotp(c *gin.Context, session *identity.PendingAuthSession, user *identity.User) bool {
	if user == nil || !user.TotpEnabled ||
		(session.ProviderType != identity.SocialTelegram && session.ProviderType != identity.SocialDiscord) ||
		!h.settingSvc.IsTotpEnabled(c.Request.Context()) {
		return false
	}
	if h.totpService == nil {
		httpx.InternalError(c, "Two-factor authentication is unavailable")
		return true
	}
	token, err := h.totpService.CreatePendingOAuthBindLoginSession(c.Request.Context(), user.ID, user.Email, session.SessionToken, session.BrowserSessionKey)
	if err != nil {
		httpx.InternalError(c, "Could not create the two-factor challenge")
		return true
	}
	httpx.Success(c, map[string]any{
		"requires_2fa":      true,
		"temp_token":        token,
		"user_email_masked": identity.MaskEmail(user.Email),
		"provider":          session.ProviderType,
		"redirect":          session.RedirectTo,
	})
	return true
}
