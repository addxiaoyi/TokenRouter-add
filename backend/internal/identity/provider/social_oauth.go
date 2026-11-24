package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/identity"
)

// social_oauth.go resolves Telegram and Discord accounts through their fixed
// authorization protocols. Telegram reuses the existing OIDC code exchange and
// cryptographic ID-token verifier; Discord reads its authenticated user resource.
// Neither provider's nickname or email can replace the stable external subject
// used by pending login and account binding.
//
// Verified provider fields become optional profile suggestions. Telegram's signed
// picture claim supplies its avatar, while Discord's account ID and validated
// avatar hash form the public CDN URL. Tokens and provider response bodies stay
// outside returned errors and local profile metadata so credential failures do
// not disclose upstream access credentials to browser clients or application logs.

var (
	// Account IDs and avatar hashes are provider-defined path components; accepting
	// only their documented characters prevents profile responses from changing CDN destinations.
	socialSubject = regexp.MustCompile(`^[0-9]{1,32}$`)
	discordAvatar = regexp.MustCompile(`^(a_)?[a-fA-F0-9]{32}$`)
)

// SocialClient resolves fixed social providers using the existing bounded OAuth transport.
type SocialClient struct{}

// Resolve exchanges a code and returns only an authenticated stable account identity.
// The callback and verifier come from the browser ceremony. Telegram validates
// signature, issuer, audience, expiration, and nonce before exposing profile claims;
// Discord requires a successful Bearer-authenticated user response with a numeric ID.
//
// Email is retained only with Discord's explicit verification flag. The local
// continuation still requires account proof before binding an existing user, so
// an email match does not turn this provider response into account ownership.
func (SocialClient) Resolve(ctx context.Context, options identity.SocialOAuthOptions, code, verifier, nonce string) (*identity.SocialOAuthProfile, error) {
	cfg, err := options.ProtocolOptions()
	if err != nil {
		return nil, err
	}
	token, err := OidcExchangeCode(ctx, cfg, code, options.RedirectURL, verifier)
	if err != nil {
		return nil, fmt.Errorf("%s authorization code exchange failed", options.Provider)
	}
	if options.Provider == identity.SocialTelegram {
		return socialTelegramProfile(ctx, cfg, token.IDToken, nonce)
	}
	if token.AccessToken == "" || strings.ContainsAny(token.AccessToken, " \t\r\n") || !strings.EqualFold(token.TokenType, "Bearer") {
		return nil, fmt.Errorf("discord did not return a usable access token")
	}
	status, body, err := doIdentityOAuthRequest(ctx, http.MethodGet, cfg.UserInfoURL, "Bearer "+token.AccessToken, nil, nil)
	if err != nil || status != http.StatusOK {
		return nil, fmt.Errorf("discord account lookup failed")
	}
	return socialDiscordProfile(body)
}

// socialTelegramProfile verifies a signed Telegram token before exposing account hints.
// The caller supplies fixed Telegram protocol options and the saved browser nonce.
// Missing expiration, issuance time, or a stable numeric subject rejects the proof.
//
// The shared JWT verifier checks signature, audience, issuer, expiry, and nonce.
// Telegram's required claim checks run afterward because generic OIDC settings
// permit providers with fewer claims. No email is inferred from a Telegram
// nickname or subject, so pending signup still needs a local account decision.
func socialTelegramProfile(ctx context.Context, cfg identity.OIDCOAuthOptions, token, nonce string) (*identity.SocialOAuthProfile, error) {
	claims, err := OidcParseAndValidateIDToken(ctx, cfg, token, nonce)
	if err != nil || nonce == "" || claims == nil || claims.ExpiresAt == nil || claims.IssuedAt == nil ||
		claims.IssuedAt.After(time.Now()) || !socialSubject.MatchString(claims.Subject) {
		return nil, fmt.Errorf("telegram identity token could not be verified")
	}
	return &identity.SocialOAuthProfile{Subject: claims.Subject, Username: claims.PreferredUsername, DisplayName: claims.Name, AvatarURL: claims.Picture}, nil
}

// socialDiscordProfile interprets authenticated Discord user fields without inferring ownership.
// A numeric ID is mandatory. Missing names and avatars remain optional, and an
// unverified email is discarded instead of becoming a local signup suggestion.
func socialDiscordProfile(body []byte) (*identity.SocialOAuthProfile, error) {
	var user struct {
		ID         string `json:"id"`          // stable Discord account ID
		Username   string `json:"username"`    // public account username
		GlobalName string `json:"global_name"` // public display name
		Avatar     string `json:"avatar"`      // optional CDN avatar hash
		Email      string `json:"email"`       // optional upstream email hint
		Verified   bool   `json:"verified"`    // upstream email verification flag
	}
	if err := json.Unmarshal(body, &user); err != nil || !socialSubject.MatchString(user.ID) {
		return nil, fmt.Errorf("discord returned an invalid account identity")
	}
	profile := &identity.SocialOAuthProfile{Subject: user.ID, Username: user.Username, DisplayName: user.GlobalName}
	if profile.DisplayName == "" {
		// Accounts without a global display name still provide their public username,
		// which remains an optional profile suggestion and never an identity key.
		profile.DisplayName = user.Username
	}
	if user.Verified {
		profile.Email = strings.TrimSpace(user.Email)
	}
	if discordAvatar.MatchString(user.Avatar) {
		profile.AvatarURL = "https://cdn.discordapp.com/avatars/" + user.ID + "/" + user.Avatar + ".png"
	}
	return profile, nil
}
