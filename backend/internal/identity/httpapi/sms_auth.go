package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/server/clientip"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// sms_auth.go handles SMS verification-code sends, phone login, and
// current-user phone binding.
// Phone numbers are authentication identities whose subject is the
// normalized number itself. A verified code either resolves the bound
// account through the shared login completion (including its TOTP
// challenge), opens a pending registration for an unbound number, or
// attaches the number to the authenticated account. Code generation,
// budgets, and delivery stay in the identity SMS challenge service; this
// handler owns request admission and account selection.

// SMSHandler manages phone code sends, phone login, and phone binding
// continuations.
type SMSHandler struct {
	*PendingHandler                                                          // shared browser pending authentication
	loadSettings    func(context.Context) (identity.SMSLoginSettings, error) // current SMS configuration callback
}

// SendSMSCodeRequest carries the target number and the shared CAPTCHA
// proof fields for a public code send.
type SendSMSCodeRequest struct {
	Phone                 string `json:"phone" binding:"required"` // mainland mobile number input
	TurnstileToken        string `json:"turnstile_token"`          // shared enabled CAPTCHA proof
	TencentCaptchaTicket  string `json:"tencent_captcha_ticket"`   // Tencent action ticket text
	TencentCaptchaRandstr string `json:"tencent_captcha_randstr"`  // Tencent ticket random string
}

// SMSLoginRequest carries the number and its received code.
type SMSLoginRequest struct {
	Phone string `json:"phone" binding:"required"` // mainland mobile number input
	Code  string `json:"code" binding:"required"`  // received verification code
}

// PhoneBindRequest carries the number and code for a current-user
// binding.
type PhoneBindRequest struct {
	Phone string `json:"phone" binding:"required"` // mainland mobile number input
	Code  string `json:"code" binding:"required"`  // received verification code
}

// NewSMSHandler retains the shared pending handler and the SMS
// configuration reader. Initialize dependencies before serving phone
// requests. Both dependencies must remain usable while the handler serves
// login and account-binding requests.
func NewSMSHandler(p *PendingHandler, load func(context.Context) (identity.SMSLoginSettings, error)) *SMSHandler {
	return &SMSHandler{p, load}
}

// SendSMSCode checks the feature switch and CAPTCHA before asking the
// challenge service to deliver a code. Provider configuration, cooldown,
// and the daily cap are enforced inside that service.
func (h *SMSHandler) SendSMSCode(c *gin.Context) {
	var req SendSMSCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if _, err := h.requireSMSEnabled(c); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	phone, ok := identity.NormalizeSMSPhoneNumber(req.Phone)
	if !ok {
		response.ErrorFrom(c, identity.ErrInvalidPhoneNumber)
		return
	}
	proof := captchaProof(req.TurnstileToken, req.TencentCaptchaTicket, req.TencentCaptchaRandstr)
	if err := h.authService.VerifyCaptcha(c.Request.Context(), proof, clientip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.authService.SendSMSCode(c.Request.Context(), phone); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// SMSLogin verifies the code and completes login for the bound account.
// An unbound number opens a pending registration instead, so the browser
// continues through the shared pending create-account flow; the code is
// already consumed, and the pending session carries the verified number
// as its identity subject.
func (h *SMSHandler) SMSLogin(c *gin.Context) {
	var req SMSLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if _, err := h.requireSMSEnabled(c); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	phone, ok := identity.NormalizeSMSPhoneNumber(req.Phone)
	if !ok {
		response.ErrorFrom(c, identity.ErrInvalidPhoneNumber)
		return
	}
	if err := h.authService.VerifySMSCode(c.Request.Context(), phone, strings.TrimSpace(req.Code)); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	identityKey := identity.PendingAuthIdentityKey{
		ProviderType:    identity.SMSPhoneProviderType,
		ProviderKey:     identity.SMSPhoneProviderKey,
		ProviderSubject: phone,
	}
	existingUser, err := h.flow.Database.FindOAuthIdentityUser(c.Request.Context(), identityKey)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if existingUser == nil {
		h.openPhoneRegistration(c, identityKey, phone)
		return
	}

	user := existingUser
	if err := EnsureLoginUserActive(user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.totpService != nil && h.settingSvc.IsTotpEnabled(c.Request.Context()) && user.TotpEnabled {
		// The account's second factor still applies to code login; the
		// shared Login2FA endpoint completes this challenge.
		tempToken, err := h.totpService.CreateLoginSession(c.Request.Context(), user.ID, user.Email)
		if err != nil {
			response.InternalError(c, "Failed to create 2FA session")
			return
		}
		response.Success(c, TotpLoginResponse{
			Requires2FA:     true,
			TempToken:       tempToken,
			UserEmailMasked: identity.MaskEmail(user.Email),
		})
		return
	}

	h.authService.RecordSuccessfulLogin(c.Request.Context(), user.ID)
	h.respondWithTokenPair(c, user)
}

// SendPhoneBindCode delivers a binding code to the submitted number for
// the authenticated account. CAPTCHA is not repeated because the caller
// already holds a panel session; cooldown and daily caps still apply.
func (h *SMSHandler) SendPhoneBindCode(c *gin.Context) {
	var req SendSMSCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if _, ok := authctx.GetAuthSubjectFromContext(c); !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("UNAUTHORIZED", "authentication required"))
		return
	}
	if _, err := h.requireSMSEnabled(c); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	phone, ok := identity.NormalizeSMSPhoneNumber(req.Phone)
	if !ok {
		response.ErrorFrom(c, identity.ErrInvalidPhoneNumber)
		return
	}
	if err := h.authService.SendSMSCode(c.Request.Context(), phone); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// BindPhone verifies the code and attaches the number identity to the
// authenticated account. The shared binding operation rejects a number
// already owned by another account.
func (h *SMSHandler) BindPhone(c *gin.Context) {
	var req PhoneBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok {
		response.ErrorFrom(c, infraerrors.Unauthorized("UNAUTHORIZED", "authentication required"))
		return
	}
	if _, err := h.requireSMSEnabled(c); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	phone, ok := identity.NormalizeSMSPhoneNumber(req.Phone)
	if !ok {
		response.ErrorFrom(c, identity.ErrInvalidPhoneNumber)
		return
	}
	if err := h.authService.VerifySMSCode(c.Request.Context(), phone, strings.TrimSpace(req.Code)); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.flow.Database.ApplyBinding(c.Request.Context(), identity.PendingBinding{Session: &identity.PendingAuthSession{
		Intent:          OauthIntentBindCurrentUser,
		ProviderType:    identity.SMSPhoneProviderType,
		ProviderKey:     identity.SMSPhoneProviderKey,
		ProviderSubject: phone,
		ResolvedEmail:   user.Email,
	}, OverrideUserID: &user.ID}); err != nil {
		RespondPendingOAuthBindingApplyError(c, err)
		return
	}
	response.Success(c, gin.H{"bound": true, "phone": phone})
}

// requireSMSEnabled loads the current SMS settings and rejects the
// request when the feature switch is off.
func (h *SMSHandler) requireSMSEnabled(c *gin.Context) (identity.SMSLoginSettings, error) {
	cfg, err := h.loadSettings(c.Request.Context())
	if err != nil {
		return identity.SMSLoginSettings{}, err
	}
	if !cfg.Enabled {
		return identity.SMSLoginSettings{}, identity.ErrSMSLoginDisabled
	}
	return cfg, nil
}

// openPhoneRegistration stores a pending registration for a verified but
// unbound number and reports that the browser must continue through the
// pending create-account flow.
func (h *SMSHandler) openPhoneRegistration(c *gin.Context, identityKey identity.PendingAuthIdentityKey, phone string) {
	browserSessionKey, err := GenerateOAuthPendingBrowserSession()
	if err != nil {
		response.ErrorFrom(c, infraerrors.InternalServer("OAUTH_BROWSER_SESSION_GEN_FAILED", "failed to generate oauth browser session").WithCause(err))
		return
	}
	secureCookie := IsRequestHTTPS(c)
	SetOAuthPendingBrowserCookie(c, browserSessionKey, secureCookie)
	ClearOAuthPendingSessionCookie(c, secureCookie)
	draft := identity.OAuthPendingDraft{
		Intent:            OauthIntentLogin,
		Identity:          identityKey,
		RedirectTo:        QQOAuthDefaultRedirectTo,
		BrowserSessionKey: browserSessionKey,
		UpstreamIdentityClaims: map[string]any{
			"phone":   phone,
			"subject": phone,
		},
		CompletionResponse: map[string]any{
			"step":                   "create_account_required",
			"phone":                  phone,
			"create_account_allowed": true,
			"choice_reason":          "phone_signup",
			"redirect":               QQOAuthDefaultRedirectTo,
		},
	}
	if err := h.CreateOAuthPendingSession(c, draft); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"requires_registration": true,
		"step":                  "create_account_required",
		"phone":                 phone,
	})
}
