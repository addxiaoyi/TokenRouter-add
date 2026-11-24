package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// social_settings_test.go covers partial administrator writes and public
// credential masking for Telegram and Discord. Validation operates on the
// effective stored configuration, so an unrelated edit cannot disable a provider
// or expose the saved secret in a response.

// TestSocialSettingsPartialUpdates checks enablement, secret preservation, and
// invalid callback rejection through the registered participant. Missing credentials
// fail before persistence while untouched provider values remain absent.
func TestSocialSettingsPartialUpdates(t *testing.T) {
	participant := SettingsParticipant(nil)
	for _, provider := range []string{SocialTelegram, SocialDiscord} {
		t.Run(provider, func(t *testing.T) {
			prefix := provider + "_oauth_"
			current := map[string]string{prefix + "enabled": "false", prefix + "client_id": "client", prefix + "client_secret": "private"}
			input := settings.Fields{prefix + "enabled": json.RawMessage(`true`), prefix + "client_secret": json.RawMessage(`"  "`)}
			prepared, err := participant.Prepare(context.Background(), input, current)
			require.NoError(t, err)
			require.Equal(t, map[string]string{prefix + "enabled": "true"}, prepared.Values)
			_, err = participant.Prepare(context.Background(), input, nil)
			require.Error(t, err)
			current[prefix+"enabled"] = "true"
			_, err = participant.Prepare(context.Background(), settings.Fields{prefix + "client_id": json.RawMessage(`""`)}, current)
			require.Error(t, err)
			_, err = participant.Prepare(context.Background(), settings.Fields{prefix + "redirect_url": json.RawMessage(`"http://public.example/callback"`)}, current)
			require.Error(t, err)
			prepared, err = participant.Prepare(context.Background(), settings.Fields{prefix + "redirect_url": json.RawMessage(`""`)}, current)
			require.NoError(t, err)
			require.Equal(t, map[string]string{prefix + "redirect_url": ""}, prepared.Values)
		})
	}
}

// TestSocialSettingsMaskingAndReadiness ensures browser and administrator views
// reveal credential presence only. Incomplete providers are hidden until both
// credentials and an acceptable callback policy are available.
func TestSocialSettingsMaskingAndReadiness(t *testing.T) {
	values := map[string]string{"telegram_oauth_enabled": "true", "telegram_oauth_client_id": "123", "telegram_oauth_client_secret": "sensitive-fixture"}
	view := SocialOAuthSettingsFromValues(values).PublicView()
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sensitive-fixture")
	require.Contains(t, string(encoded), `"telegram_oauth_client_secret_configured":true`)
	require.True(t, SocialOAuthPublicEnabled(values, SocialTelegram))
	require.False(t, SocialOAuthPublicEnabled(values, SocialDiscord))
	delete(values, "telegram_oauth_client_secret")
	require.False(t, SocialOAuthPublicEnabled(values, SocialTelegram))
	for _, callback := range []string{"https://shop.example/api/callback", "http://localhost:3000/callback", "http://127.0.0.1/callback"} {
		require.True(t, SocialOAuthRedirectAllowed(callback), callback)
	}
	for _, callback := range []string{"http://shop.example/callback", "//shop.example/callback", "https://user:secret@shop.example/callback", "https://shop.example/callback?x=1", "https://shop.example/callback#fragment"} {
		require.False(t, SocialOAuthRedirectAllowed(callback), callback)
	}
}

// TestSocialIdentityUnbindProtection keeps the only external sign-in identity
// attached. A second recognized identity permits removal while provider paths
// preserve the authenticated binding intent.
func TestSocialIdentityUnbindProtection(t *testing.T) {
	service := &UserService{}
	user := &User{SignupSource: SocialTelegram}
	records := []UserAuthIdentityRecord{{ProviderType: SocialTelegram}}
	require.False(t, service.ProfileCanUnbindProvider(SocialTelegram, user, records))
	records = append(records, UserAuthIdentityRecord{ProviderType: SocialDiscord})
	require.True(t, service.ProfileCanUnbindProvider(SocialTelegram, user, records))
	for _, provider := range []string{SocialTelegram, SocialDiscord} {
		path, err := ProfileBuildUserIdentityBindAuthorizeURL(provider, "/profile")
		require.NoError(t, err)
		require.Contains(t, path, "/oauth/"+provider+"/bind/start?")
		require.Contains(t, path, "intent=bind_current_user")
		require.Equal(t, provider, AuthNormalizeOAuthSignupSource(provider))
	}
}
