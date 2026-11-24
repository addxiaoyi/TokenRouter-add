package pricing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCatalogRequestPrices preserves standalone request prices and explicit free
// amounts. Invalid prices reject publication, and cloned metadata cannot mutate
// the retained catalog through its request-price pointer.
func TestCatalogRequestPrices(t *testing.T) {
	entries, err := DecodeCatalogEntries([]byte(`{"request":{"cost_per_request":0.01},"free":{"cost_per_request":0}}`))
	require.NoError(t, err)
	prices, diagnostics, err := ParsePricingEntries(entries)
	require.NoError(t, err)
	require.NoError(t, diagnostics.ValidationError())
	require.Equal(t, 0.01, *prices["request"].CostPerRequest)
	require.Zero(t, *prices["free"].CostPerRequest)
	require.True(t, prices["request"].TokenPricingAbsent)
	cloned := CloneCatalogPrice(prices["request"])
	*cloned.CostPerRequest = 3
	require.Equal(t, 0.01, *prices["request"].CostPerRequest)
	entries, err = DecodeCatalogEntries([]byte(`{"invalid":{"cost_per_request":-0.01}}`))
	require.NoError(t, err)
	_, diagnostics, _ = ParsePricingEntries(entries)
	require.ErrorContains(t, diagnostics.ValidationError(), "cost_per_request")
}

// TestCatalogProviderCompatibility supplies legacy provider names beside
// absent, populated, empty, and null current fields. Decoding and serialization
// must use provider, retain its precedence, and remove litellm_provider.
func TestCatalogProviderCompatibility(t *testing.T) {
	for _, tc := range []struct {
		fields, // submitted provider JSON fields
		want string // expected decoded provider label
	}{
		{`"litellm_provider":"openai"`, "openai"},
		{`"litellm_provider":"old","provider":"new"`, "new"},
		{`"litellm_provider":"old","provider":""`, ""},
		{`"litellm_provider":"old","provider":null`, ""},
	} {
		raw, err := DecodeCatalogEntries([]byte(`{"model":{"input_cost_per_token":0,` + tc.fields + `}}`))
		require.NoError(t, err)
		require.NotContains(t, string(raw["model"]), "litellm_provider")
		values, diagnostics, err := ParsePricingEntries(raw)
		require.NoError(t, err)
		require.NoError(t, diagnostics.ValidationError())
		require.Equal(t, tc.want, values["model"].Provider)
		output, err := json.Marshal(values["model"])
		require.NoError(t, err)
		require.NotContains(t, string(output), "litellm_provider")
		require.Contains(t, string(output), `"provider":`)
	}
}

// TestCatalogVideoDimensionsNormalizeBeforePublication parses custom video rates
// and conflicting aliases. Valid tables retain free dimensions; conflicts become
// publication diagnostics instead of selecting a map iteration winner.
func TestCatalogVideoDimensionsNormalizeBeforePublication(t *testing.T) {
	entries, err := DecodeCatalogEntries([]byte(`{"video":{"video_prices":{"768P":0.12,"2K":0}}}`))
	require.NoError(t, err)
	values, diagnostics, err := ParsePricingEntries(entries)
	require.NoError(t, err)
	require.NoError(t, diagnostics.ValidationError())
	require.Equal(t, MediaPrices{"768p": 0.12, "2k": 0}, values["video"].VideoPrices)
	entries, err = DecodeCatalogEntries([]byte(`{"video":{"video_prices":{"HD":0.1,"720p":0.2}}}`))
	require.NoError(t, err)
	_, diagnostics, err = ParsePricingEntries(entries)
	require.Error(t, err)
	require.ErrorContains(t, diagnostics.ValidationError(), "duplicate video resolution")
}
