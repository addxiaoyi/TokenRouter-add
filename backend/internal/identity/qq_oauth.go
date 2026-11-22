package identity

import (
	"context"
	"fmt"
	"strings"
)

// qq_oauth.go builds QQ pending account choices and defines the provider
// exchange inputs and results.
// QQ互联 supplies no email address, so a QQ identity always binds through
// a subject-derived synthetic address, and the provider nickname remains a
// display suggestion. The stable subject prefers the cross-application
// unionid when the QQ application has it enabled and falls back to the
// application-scoped openid otherwise.
//
// The choice builder reuses the LinuxDo email-choice construction because
// QQ's policy inputs are its no-provider-email subset: no compatibility
// email or compatibility account can exist, while local email verification
// and forced-email signup policies keep their usual precedence.

// QQOAuthProviderKey is the canonical provider key for QQ identities.
const QQOAuthProviderKey = "qq"

// QQOAuthOptions supplies provider endpoints and credentials for one
// authorization ceremony. The endpoint addresses are fixed by QQ互联；
// the settings reader fills them from the platform constants so tests and
// deployments share one resolution path.
type QQOAuthOptions struct {
	Enabled      bool   // configured QQ login switch
	ClientID     string // QQ互联 application identifier (AppID)
	ClientSecret string // QQ互联 application credential (AppKey)
	AuthorizeURL string // provider browser authorization endpoint
	TokenURL     string // provider token exchange endpoint
	MeURL        string // provider openid lookup endpoint
	UserInfoURL  string // provider profile lookup endpoint
	RedirectURL  string // registered backend authorization callback
	// FrontendRedirectURL selects the browser return destination. OAuth
	// settings can override the built-in /auth/qq/callback route through
	// startup or stored configuration.
	FrontendRedirectURL string
}

// QQTokenResponse holds provider access credentials and their returned
// token metadata.
type QQTokenResponse struct {
	AccessToken  string `json:"access_token"`            // upstream bearer access credential
	ExpiresIn    int64  `json:"expires_in"`              // seconds
	RefreshToken string `json:"refresh_token,omitempty"` // optional upstream renewal credential
}

// QQTokenExchangeError retains failed exchange status, provider messages
// and diagnostic response text.
type QQTokenExchangeError struct {
	StatusCode          int    // token exchange HTTP status
	ProviderError       string // raw provider error code
	ProviderDescription string // raw provider error description
	// Body retains failed response text for diagnostics. Error emits the
	// status and normalized provider messages, leaving this payload out of
	// its display result.
	Body string
}

// Error renders the token-exchange status and nonblank trimmed provider
// messages. A nil error receiver yields empty text, and Body is retained
// separately rather than included in this display result.
func (e *QQTokenExchangeError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("token exchange status=%d", e.StatusCode)}
	if strings.TrimSpace(e.ProviderError) != "" {
		parts = append(parts, "error="+strings.TrimSpace(e.ProviderError))
	}
	if strings.TrimSpace(e.ProviderDescription) != "" {
		parts = append(parts, "error_description="+strings.TrimSpace(e.ProviderDescription))
	}
	return strings.Join(parts, " ")
}

// QQOAuthClient exchanges authorization codes and extracts upstream
// identity and profile values.
type QQOAuthClient interface {
	// ExchangeCode uses the supplied options, authorization code, and
	// callback URI to request upstream credentials. The caller retains
	// browser state and local account decisions; provider or transport
	// failures remain errors.
	ExchangeCode(context.Context, QQOAuthOptions, string, string) (*QQTokenResponse, error)
	// FetchUserInfo uses the upstream token to resolve the stable subject
	// and returns email, username, subject, display name and avatar URL in
	// that order. QQ supplies no email, so the email result is always
	// empty; the caller derives the local binding address from the
	// subject instead.
	FetchUserInfo(context.Context, QQOAuthOptions, *QQTokenResponse) (string, string, string, string, string, error)
}

// OAuthQQMaxSubjectLength bounds subject bytes so the qq- prefix and
// accepted subject fit within the synthetic address's 64-byte local part.
const OAuthQQMaxSubjectLength = 64 - len("qq-")

// OAuthIsSafeQQSubject reports whether a trimmed subject fits a synthetic
// QQ address. It requires a nonempty value within the byte bound and
// accepts the ASCII letters, digits, underscores and hyphens QQ uses in
// openid and unionid values.
func OAuthIsSafeQQSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > OAuthQQMaxSubjectLength {
		return false
	}
	for _, r := range subject {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// OAuthQQSyntheticEmail builds the reserved address for a trimmed
// subject. Empty subjects return an empty string; callers handling
// external input first check OAuthIsSafeQQSubject because construction
// itself accepts any nonempty text.
func OAuthQQSyntheticEmail(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	return "qq-" + subject + QQConnectSyntheticEmailDomain
}

// QQFallbackUsername builds a stable username suggestion from a trimmed
// subject for signups whose QQ nickname cannot serve as a username.
// Blank input uses qq_user.
func QQFallbackUsername(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "qq_user"
	}
	return "qq_" + subject
}

// PrepareQQChoice builds a pending account-choice draft for a QQ signup.
// QQ never supplies an email, so the draft passes the synthetic address
// as both suggestion and resolved hint with no compatibility account;
// PrepareLinuxDoChoice then applies the local email-verification and
// forced-email policies exactly as it does for a provider whose profile
// lacked a usable email.
func PrepareQQChoice(
	identity PendingAuthIdentityKey,
	syntheticEmail string,
	redirectTo string,
	browserSessionKey string,
	upstreamClaims map[string]any,
	emailVerificationRequired bool,
	forceEmailOnSignup bool,
) OAuthPendingDraft {
	return PrepareLinuxDoChoice(
		identity,
		syntheticEmail,
		syntheticEmail,
		redirectTo,
		browserSessionKey,
		upstreamClaims,
		"",
		nil,
		emailVerificationRequired,
		forceEmailOnSignup,
	)
}
