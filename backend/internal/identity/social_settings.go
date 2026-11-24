package identity

import (
	"strconv"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// social_settings.go prepares runtime Telegram and Discord settings and their
// administrator views. Credentials remain available only in internal snapshots
// and submitted updates; response views replace each secret with a presence flag.
// Blank submitted secrets retain stored credentials, while an empty callback
// selects the public request origin during authorization start.
//
// Partial updates are checked against the stored values after field-presence
// filtering. Enabling a provider therefore requires its effective client ID and
// secret even when those fields were saved in earlier requests. Disabling another
// provider or changing unrelated settings does not force incomplete drafts to
// become valid. Fixed protocol endpoints remain outside administrator configuration.

// SocialOAuthSettings carries runtime provider settings through internal administrator snapshots.
type SocialOAuthSettings struct {
	TelegramOAuthEnabled  bool   `json:"telegram_oauth_enabled"`   // Telegram login switch
	TelegramOAuthClientID string `json:"telegram_oauth_client_id"` // Telegram authorization client ID
	// TelegramOAuthClientSecret stays server-side; blank update text retains the
	// saved credential, and response serialization uses SocialOAuthView instead.
	TelegramOAuthClientSecret string `json:"telegram_oauth_client_secret"`
	TelegramOAuthRedirectURL  string `json:"telegram_oauth_redirect_url"` // optional registered backend callback
	DiscordOAuthEnabled       bool   `json:"discord_oauth_enabled"`       // Discord login switch
	DiscordOAuthClientID      string `json:"discord_oauth_client_id"`     // Discord authorization client ID
	// DiscordOAuthClientSecret stays server-side; blank update text retains the
	// saved credential, and response serialization uses SocialOAuthView instead.
	DiscordOAuthClientSecret string `json:"discord_oauth_client_secret"`
	DiscordOAuthRedirectURL  string `json:"discord_oauth_redirect_url"` // optional registered backend callback
}

// SocialOAuthView exposes editable provider configuration without returning client secrets.
type SocialOAuthView struct {
	TelegramOAuthEnabled                bool   `json:"telegram_oauth_enabled"`                  // Telegram login switch
	TelegramOAuthClientID               string `json:"telegram_oauth_client_id"`                // Telegram authorization client ID
	TelegramOAuthClientSecretConfigured bool   `json:"telegram_oauth_client_secret_configured"` // saved credential presence
	TelegramOAuthRedirectURL            string `json:"telegram_oauth_redirect_url"`             // optional registered backend callback
	DiscordOAuthEnabled                 bool   `json:"discord_oauth_enabled"`                   // Discord login switch
	DiscordOAuthClientID                string `json:"discord_oauth_client_id"`                 // Discord authorization client ID
	DiscordOAuthClientSecretConfigured  bool   `json:"discord_oauth_client_secret_configured"`  // saved credential presence
	DiscordOAuthRedirectURL             string `json:"discord_oauth_redirect_url"`              // optional registered backend callback
}

// SocialOAuthSettingsFromValues reads stored social credentials without startup fallbacks.
// Missing switches remain disabled, and credential whitespace is removed. This
// internal value must pass through PublicView before entering an administrator response.
func SocialOAuthSettingsFromValues(values map[string]string) SocialOAuthSettings {
	return SocialOAuthSettings{
		TelegramOAuthEnabled:      values["telegram_oauth_enabled"] == "true",
		TelegramOAuthClientID:     strings.TrimSpace(values["telegram_oauth_client_id"]),
		TelegramOAuthClientSecret: strings.TrimSpace(values["telegram_oauth_client_secret"]),
		TelegramOAuthRedirectURL:  strings.TrimSpace(values["telegram_oauth_redirect_url"]),
		DiscordOAuthEnabled:       values["discord_oauth_enabled"] == "true",
		DiscordOAuthClientID:      strings.TrimSpace(values["discord_oauth_client_id"]),
		DiscordOAuthClientSecret:  strings.TrimSpace(values["discord_oauth_client_secret"]),
		DiscordOAuthRedirectURL:   strings.TrimSpace(values["discord_oauth_redirect_url"]),
	}
}

// PublicView replaces internal social client secrets with configuration presence flags.
func (s SocialOAuthSettings) PublicView() SocialOAuthView {
	return SocialOAuthView{
		TelegramOAuthEnabled:                s.TelegramOAuthEnabled,
		TelegramOAuthClientID:               s.TelegramOAuthClientID,
		TelegramOAuthClientSecretConfigured: s.TelegramOAuthClientSecret != "",
		TelegramOAuthRedirectURL:            s.TelegramOAuthRedirectURL,
		DiscordOAuthEnabled:                 s.DiscordOAuthEnabled,
		DiscordOAuthClientID:                s.DiscordOAuthClientID,
		DiscordOAuthClientSecretConfigured:  s.DiscordOAuthClientSecret != "",
		DiscordOAuthRedirectURL:             s.DiscordOAuthRedirectURL,
	}
}

// appendUpdates prepares social fields while preserving previously saved blank-secret inputs.
// Callers apply their submitted-field filter before saving this map. Empty
// callback text is a meaningful update that selects automatic callback construction.
func (s SocialOAuthSettings) appendUpdates(updates map[string]string) {
	updates["telegram_oauth_enabled"] = strconv.FormatBool(s.TelegramOAuthEnabled)
	updates["telegram_oauth_client_id"] = strings.TrimSpace(s.TelegramOAuthClientID)
	updates["telegram_oauth_redirect_url"] = strings.TrimSpace(s.TelegramOAuthRedirectURL)
	updates["discord_oauth_enabled"] = strconv.FormatBool(s.DiscordOAuthEnabled)
	updates["discord_oauth_client_id"] = strings.TrimSpace(s.DiscordOAuthClientID)
	updates["discord_oauth_redirect_url"] = strings.TrimSpace(s.DiscordOAuthRedirectURL)
	if secret := strings.TrimSpace(s.TelegramOAuthClientSecret); secret != "" {
		updates["telegram_oauth_client_secret"] = secret
	}
	if secret := strings.TrimSpace(s.DiscordOAuthClientSecret); secret != "" {
		updates["discord_oauth_client_secret"] = secret
	}
}

// validateSocialOAuthUpdates checks effective credentials after submitted-field filtering.
// Only touched providers are checked. A disabled provider may retain incomplete
// credentials, but any explicit callback must satisfy the public HTTPS policy.
//
// Each selected value overrides its saved counterpart before validation, so a
// request cannot enable a provider by omitting its required credentials or erase
// a live client ID while leaving it enabled. Secret preservation has already
// removed blank secrets from updates; those entries consequently use storage.
func validateSocialOAuthUpdates(updates, current map[string]string) error {
	for _, provider := range []string{SocialTelegram, SocialDiscord} {
		prefix := provider + "_oauth_"
		effective := make(map[string]string, 4)
		touched := false
		for _, field := range []string{"enabled", "client_id", "client_secret", "redirect_url"} {
			key := prefix + field
			effective[field] = current[key]
			if value, ok := updates[key]; ok {
				effective[field] = value
				touched = true
			}
		}
		if !touched {
			continue
		}
		if callback := effective["redirect_url"]; callback != "" && !SocialOAuthRedirectAllowed(callback) {
			return apperror.BadRequest("OAUTH_CONFIG_INVALID", provider+" callback must be an absolute HTTPS URL")
		}
		if effective["enabled"] == "true" && (strings.TrimSpace(effective["client_id"]) == "" || strings.TrimSpace(effective["client_secret"]) == "") {
			return apperror.BadRequest("OAUTH_CONFIG_INVALID", provider+" client ID and client secret are required when enabled")
		}
	}
	return nil
}

// SocialOAuthPublicEnabled reports readiness without disclosing a provider's credentials.
// Missing or incomplete settings hide the browser button. An automatic callback
// remains valid here because the start handler validates the actual public origin.
func SocialOAuthPublicEnabled(values map[string]string, provider string) bool {
	prefix := provider + "_oauth_"
	callback := strings.TrimSpace(values[prefix+"redirect_url"])
	return values[prefix+"enabled"] == "true" &&
		strings.TrimSpace(values[prefix+"client_id"]) != "" &&
		strings.TrimSpace(values[prefix+"client_secret"]) != "" &&
		(callback == "" || SocialOAuthRedirectAllowed(callback))
}
