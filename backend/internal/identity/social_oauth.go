package identity

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// social_oauth.go defines Telegram and Discord authorization settings and their
// verified identity boundary. Both providers use an authorization code with PKCE,
// but Telegram identifies the caller through a signed ID token while Discord
// supplies the stable account ID through its authenticated user endpoint.
// Provider endpoints and scopes belong to these definitions so an administrator
// cannot redirect an existing client's secret to another authorization server.
//
// Stored settings contain only enablement, credentials, and the registered local
// callback. Browser login and current-account binding share those settings while
// keeping their pending intent distinct. Profile email is a suggestion, and
// existing account ownership always comes from the persisted provider subject.

const (
	// Social provider labels are persisted identity types and settings prefixes;
	// changing them would require migrating existing bindings and pending sessions.
	SocialTelegram = "telegram" // Telegram login identity
	SocialDiscord  = "discord"  // Discord login identity
)

// SocialOAuthOptions retains one provider's credentials and registered callback address.
type SocialOAuthOptions struct {
	Provider     string // canonical identity provider type
	ClientID     string // registered authorization client ID
	ClientSecret string // server-only authorization client secret
	RedirectURL  string // optional explicit backend callback
}

// SocialOAuthProfile contains identity and profile fields verified through the provider.
type SocialOAuthProfile struct {
	Subject     string // stable external account subject
	Username    string // optional public username suggestion
	DisplayName string // optional public display-name suggestion
	AvatarURL   string // optional public avatar suggestion
	// Email is populated only when the provider reports it verified. It remains
	// a signup hint and never authorizes merging with an existing local account.
	Email string
}

// SocialOAuthClient exchanges browser authorization for verified external identity.
type SocialOAuthClient interface {
	// Resolve exchanges the code using the saved PKCE verifier and callback.
	// Telegram additionally requires a matching ID-token nonce; all failures must
	// return before creating a local pending login or binding session.
	Resolve(context.Context, SocialOAuthOptions, string, string, string) (*SocialOAuthProfile, error)
}

// SocialOAuthSettingKeys returns the supported providers' public and secret setting names.
func SocialOAuthSettingKeys() []string {
	var keys []string
	for _, provider := range []string{SocialTelegram, SocialDiscord} {
		for _, suffix := range []string{"enabled", "client_id", "client_secret", "redirect_url"} {
			keys = append(keys, provider+"_oauth_"+suffix)
		}
	}
	return keys
}

// GetSocialOAuthConfig loads an enabled provider's current credentials for one ceremony.
// Missing credentials fail closed. An omitted callback is derived from the
// current public request origin by the HTTP handler and retained for code exchange.
func (s *OAuthSettings) GetSocialOAuthConfig(ctx context.Context, provider string) (SocialOAuthOptions, error) {
	if provider != SocialTelegram && provider != SocialDiscord {
		return SocialOAuthOptions{}, apperror.BadRequest("OAUTH_PROVIDER_INVALID", "unsupported login provider")
	}
	if s == nil || s.settingRepo == nil {
		return SocialOAuthOptions{}, apperror.ServiceUnavailable("CONFIG_NOT_READY", "login settings are unavailable")
	}
	prefix := provider + "_oauth_"
	values, err := s.settingRepo.GetMultiple(ctx, []string{prefix + "enabled", prefix + "client_id", prefix + "client_secret", prefix + "redirect_url"})
	if err != nil {
		return SocialOAuthOptions{}, fmt.Errorf("read %s login settings: %w", provider, err)
	}
	if values[prefix+"enabled"] != "true" {
		return SocialOAuthOptions{}, apperror.ServiceUnavailable("OAUTH_DISABLED", "this login provider is disabled")
	}
	options := SocialOAuthOptions{
		Provider: provider, ClientID: strings.TrimSpace(values[prefix+"client_id"]),
		ClientSecret: strings.TrimSpace(values[prefix+"client_secret"]), RedirectURL: strings.TrimSpace(values[prefix+"redirect_url"]),
	}
	if options.ClientID == "" || options.ClientSecret == "" {
		return SocialOAuthOptions{}, apperror.ServiceUnavailable("OAUTH_CONFIG_INVALID", "login client ID and client secret are required")
	}
	if options.RedirectURL != "" && !SocialOAuthRedirectAllowed(options.RedirectURL) {
		return SocialOAuthOptions{}, apperror.BadRequest("OAUTH_CONFIG_INVALID", "login callback must be an absolute HTTPS URL")
	}
	return options, nil
}

// SocialOAuthRedirectAllowed checks registered callback URLs before browser authorization.
// HTTPS is required outside loopback development; credentials, fragments, and
// query parameters are rejected because the authorization server supplies its query.
func SocialOAuthRedirectAllowed(raw string) bool {
	callback, err := url.Parse(raw)
	if err != nil || callback.Hostname() == "" || callback.User != nil || callback.RawQuery != "" || callback.Fragment != "" {
		return false
	}
	loopback := callback.Hostname() == "localhost" || callback.Hostname() == "127.0.0.1" || callback.Hostname() == "::1"
	return callback.Scheme == "https" || callback.Scheme == "http" && loopback
}

// ProtocolOptions returns fixed endpoints and verification policy for the selected provider.
// Telegram requires RS256 identity tokens and no userinfo request. Discord uses
// its authenticated user resource and the identify/email scopes without requesting bot access.
func (o SocialOAuthOptions) ProtocolOptions() (OIDCOAuthOptions, error) {
	options := OIDCOAuthOptions{
		ClientID: o.ClientID, ClientSecret: o.ClientSecret, RedirectURL: o.RedirectURL,
		TokenAuthMethod: "client_secret_basic", UsePKCE: true,
	}
	switch o.Provider {
	case SocialTelegram:
		options.IssuerURL = "https://oauth.telegram.org"
		options.AuthorizeURL = "https://oauth.telegram.org/auth"
		options.TokenURL = "https://oauth.telegram.org/token"
		options.JWKSURL = "https://oauth.telegram.org/.well-known/jwks.json"
		options.Scopes = "openid profile"
		options.ValidateIDToken = true
		options.AllowedSigningAlgs = "RS256"
	case SocialDiscord:
		options.IssuerURL = "https://discord.com"
		options.AuthorizeURL = "https://discord.com/oauth2/authorize"
		options.TokenURL = "https://discord.com/api/oauth2/token"
		options.UserInfoURL = "https://discord.com/api/v10/users/@me"
		options.Scopes = "identify email"
	default:
		return OIDCOAuthOptions{}, fmt.Errorf("unsupported social login provider")
	}
	return options, nil
}
