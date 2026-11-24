package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// model_quality_test.go checks that scored assessments honor configured native
// text protocols. Explicit routing must prevent standalone connection-test
// defaults from sending a Chat-only provider a Responses request and reporting
// valid Chat output as an incomplete Responses stream.

// TestQualityRouteHonorsNativeChat supplies a provider configured for Chat only.
// Its assessment must retain the Chat entry and endpoint instead of inheriting
// an unrelated standalone Responses default during provider test preparation.
func TestQualityRouteHonorsNativeChat(t *testing.T) {
	record := &provider.Record{ID: 1, Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey, Status: provider.StatusActive, Credentials: map[string]any{"upstream_protocols": []string{"openai_chat_completions"}, "model_whitelist": []string{"gpt-4o-mini"}}}
	route := qualityRoute(record, "gpt-4o-mini")
	require.Equal(t, capability.ProtocolOpenAIChatCompletions, route.UpstreamProtocol)
	require.Equal(t, route.UpstreamProtocol, route.ClientProtocol)
	require.Contains(t, qualityModels(record), "gpt-4o-mini")
}
