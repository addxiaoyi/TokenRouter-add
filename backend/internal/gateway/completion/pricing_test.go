package completion

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// pricing_test.go checks how completion selects image and token prices from
// observed output. Fixtures separate returned files, animation frames, and
// token counts so only the configured billing mode determines which quantity
// supplies the charge.

// imageTokenCatalog supplies token prices for the specified image model.
type imageTokenCatalog struct{}

// requestCatalog supplies a fixed catalog baseline for media completion tests.
type requestCatalog struct{}

// GetModelPricing supplies an explicit request baseline without native media
// units. The returned entry belongs to the caller, allowing fixtures to resolve
// the same price for text, image, video, and audio output.
func (requestCatalog) GetModelPricing(string) *pricing.CatalogModelPricing {
	price := 0.01
	return &pricing.CatalogModelPricing{CatalogRules: pricing.CatalogRules{CostPerRequest: &price}, TokenPricingAbsent: true}
}

// ForceUpdate retains the request fixture without contacting an external catalog.
func (requestCatalog) ForceUpdate() error { return nil }

// TestCatalogRequestMediaCounts applies request metadata to image, video, and
// audio completions. Files remain countable outputs while frame counts, video
// seconds, and continuous audio units cannot inflate a request baseline.
func TestCatalogRequestMediaCounts(t *testing.T) {
	calculator := billing.NewCalculator(requestCatalog{}, billing.CalculatorOptions{PreferCatalogPricing: true})
	resolver := billing.NewPriceResolver(nil, calculator, nil, nil)
	recorder := NewRecorder(Dependencies{Calculator: calculator, Prices: resolver}, RecorderOptions{})
	id := int64(66)
	key := &KeySnapshot{GroupID: &id, Group: &GroupSnapshot{ID: id}}
	for _, tc := range []struct {
		name   string  // completion fixture label
		output *Result // observed completed media usage
		want   float64 // expected base request charge
	}{
		{"image", &Result{Model: "request", ImageCount: 2, ImageFrameCount: 50}, 0.02},
		{"video", &Result{Model: "request", VideoCount: 2, VideoDurationSeconds: 60, VideoResolution: "720p"}, 0.02},
		{"audio", &Result{Model: "request", AudioUsage: &AudioUsage{Mode: "tts", DurationOrUnits: 120}}, 0.01},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, err := recorder.CalculateOpenAIRecordUsageCostAt(context.Background(), tc.output, key, []string{"request"}, 2, 2, 2, 2, UsageTokens{}, "", time.Now())
			require.NoError(t, err)
			require.Equal(t, "per_request", cost.BillingMode)
			require.InDelta(t, tc.want, cost.TotalCost, 1e-12)
			require.InDelta(t, tc.want*2, cost.ActualCost, 1e-12)
		})
	}
}

// GetModelPricing supplies fixed ordinary-input and image-output token prices
// for gpt-image-2. Other models return nil; the fixture has no per-image price,
// so positive token evidence selects exact token billing in the tested paths.
func (imageTokenCatalog) GetModelPricing(model string) *pricing.CatalogModelPricing {
	if model != "gpt-image-2" {
		return nil
	}
	return &pricing.CatalogModelPricing{InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, OutputCostPerImageToken: 3e-6}
}

// ForceUpdate satisfies the catalogue interface while retaining the fixed
// image-token fixture prices.
func (imageTokenCatalog) ForceUpdate() error { return nil }

// TestImageTokenUsageUsesExactTokenPricing supplies token evidence for an image
// result whose catalogue lacks a per-image rate. It checks both recording paths'
// token mode, image-output pricing, and missing-price rejection after that token
// evidence is removed.
func TestImageTokenUsageUsesExactTokenPricing(t *testing.T) {
	calculator := billing.NewCalculator(imageTokenCatalog{}, billing.CalculatorOptions{})
	recorder := NewRecorder(Dependencies{Calculator: calculator}, RecorderOptions{})
	result := &Result{Model: "gpt-image-2", ImageCount: 1, Usage: TokenUsage{InputTokens: 100, OutputTokens: 20, ImageOutputTokens: 20}}
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 20, ImageOutputTokens: 20}
	// Image output occupies the reported output bucket, so use its image-token
	// price once rather than also adding the ordinary output-token rate.
	want := 100*1e-6 + 20*3e-6
	openAI, err := recorder.CalculateOpenAIRecordUsageCostAt(context.Background(), result, &KeySnapshot{}, []string{result.Model}, 1, 1, 1, 1, tokens, "", time.Now())
	require.NoError(t, err)
	require.Equal(t, string(BillingModeToken), openAI.BillingMode)
	// Permit arithmetic rounding when comparing the two token-priced dollar totals.
	require.InDelta(t, want, openAI.ActualCost, 1e-12)
	messages := recorder.CalculateRecordUsageCost(context.Background(), result, &KeySnapshot{}, &ProviderSnapshot{}, result.Model, result.Model, "", "", 1, 1, nil)
	require.Equal(t, string(BillingModeToken), messages.BillingMode)
	require.InDelta(t, want, messages.ActualCost, 1e-12)
	result.Usage = TokenUsage{}
	_, err = recorder.CalculateOpenAIRecordUsageCostAt(context.Background(), result, &KeySnapshot{}, []string{result.Model}, 1, 1, 1, 1, UsageTokens{}, "", time.Now())
	require.ErrorIs(t, err, pricing.ErrModelPricingUnavailable)
}

// TestImageBillingUnits preserves file counts for request and token cards.
// Only image mode consumes separately observed frame units, including mixed outputs
// whose static files each contribute one unit.
func TestImageBillingUnits(t *testing.T) {
	result := &Result{ImageCount: 2, ImageFrameCount: 18}
	require.Equal(t, 18, imageBillingUnits(result, nil))
	require.Equal(t, 18, imageBillingUnits(result, &ResolvedPricing{Mode: BillingModeImage}))
	require.Equal(t, 2, imageBillingUnits(result, &ResolvedPricing{Mode: BillingModePerRequest}))
	require.Equal(t, 2, imageBillingUnits(result, &ResolvedPricing{Mode: BillingModeToken}))
	result.ImageFrameCount = 0
	require.Equal(t, 2, imageBillingUnits(result, nil))
}
