package pricing

import (
	"slices"
	"strings"
)

// card_resolution.go combines configured cards with base prices and assigns
// their billing mode and source.
// A configured price card can replace amount buckets or simply adjust a catalog
// price's modifiers. The source label follows that choice: explicit amounts and
// token intervals select configuration pricing, while modifier-only cards keep
// the supplied base source and require an available base.
//
// ResolvePriceCards separates that decision from the long-context policy flag,
// so manually priced intervals remain part of resolution even when built-in
// tiers are disabled. ResolveConfiguredPricing resolves request and media units
// without token base amounts, and token resolution applies default overrides
// before amount and service-tier scaling. The resulting record can retain
// references to the supplied card metadata, which callers keep stable during
// use.

// ResolveCatalogPricing selects usable catalog amounts without card overrides.
// Nil or unavailable entries return nil, allowing the caller to retain its
// configured fallback. Native video, image, and token prices precede the
// optional request baseline so later catalog updates replace temporary request
// pricing automatically. Request zeros remain explicit prices. Video labels are
// copied in stable order; image amounts are selected by the shared image-unit
// resolver when the request supplies its dimensions.
func ResolveCatalogPricing(model string, raw *CatalogModelPricing, longContextEnabled bool) *ResolvedPricing {
	if raw == nil {
		return nil
	}
	resolved := &ResolvedPricing{Source: PricingSourceCatalog, LongContextPricingEnabled: longContextEnabled}
	switch DefaultBillingMode(model, raw.Mode, raw) {
	case "video":
		prices, err := NormalizeVideoPrices(raw.VideoPrices)
		if err != nil || len(prices) == 0 {
			return nil
		}
		resolved.Mode = BillingModeVideo
		for label, price := range prices {
			resolved.RequestTiers = append(resolved.RequestTiers, PricingInterval{TierLabel: label, PerRequestPrice: &price})
		}
		slices.SortFunc(resolved.RequestTiers, func(a, b PricingInterval) int {
			return strings.Compare(a.TierLabel, b.TierLabel)
		})
	case "image":
		if len(raw.ImagePrices) == 0 && !HasImageUnitPrice(raw) {
			return nil
		}
		resolved.Mode = BillingModeImage
	case "per_request":
		resolved.Mode = BillingModePerRequest
		resolved.DefaultPerRequestPrice = *raw.CostPerRequest
		resolved.DefaultPerRequestPricePresent = true
	default:
		base, err := ResolveModelPricing(model, raw)
		if err != nil || base == nil {
			return nil
		}
		return ResolvePriceCards(nil, base, PricingSourceCatalog, longContextEnabled)
	}
	return resolved
}

// ResolveConfiguredPricing returns effective amounts for a non-nil card and its
// optional base. An empty mode uses token rules. Request and media modes
// resolve unit tiers and amount factors directly; token mode first carries base
// support flags into card-default and interval conversion.
//
// Amount scaling follows overrides, and service-tier factors are attached after
// token amounts are resolved. The card's supported data is read rather than
// mutated, although unchanged metadata references can remain shared with the
// result. Callers validate the card and keep it stable during use of the
// returned resolution.
func ResolveConfiguredPricing(config *ModelPricingEntry, base *ModelPricing, source string) *ResolvedPricing {
	mode := config.BillingMode
	if mode == "" {
		mode = BillingModeToken
	}
	resolved := &ResolvedPricing{Mode: mode, Source: source, ConfigPricing: config}
	if mode == BillingModePerRequest || mode == BillingModeImage || mode == BillingModeVideo {
		ApplyRequestTierOverrides(config, resolved)
		ApplyResolvedPriceMultiplier(resolved, config)
		return resolved
	}
	resolved.BasePricing = base
	resolved.SupportsCacheBreakdown = resolved.BasePricing != nil && resolved.BasePricing.SupportsCacheBreakdown
	resolved.SupportsServiceTier = resolved.BasePricing != nil && resolved.BasePricing.SupportsServiceTier
	ApplyTokenOverrides(config, resolved)
	ApplyResolvedPriceMultiplier(resolved, config)
	ApplyResolvedFastModeMultiplier(resolved, config)
	return resolved
}

// PriceCardOverrides reports whether the card selects configuration-source
// resolution. Supplied amounts, including zero, and token field-bearing
// intervals qualify. Interval filtering checks presence rather than full
// numeric validity, so callers perform validation separately.
func PriceCardOverrides(card *ModelPricingEntry) bool {
	return card != nil && (HasExplicitPricingPrice(*card) || len(FilterValidTokenIntervals(card.Intervals)) > 0)
}

// PriceCardNeedsBase reports whether resolution should obtain token base
// prices. Absent cards and token or other modes need that lookup. Request,
// image, and video cards resolve their unit amounts independently of a token
// base.
func PriceCardNeedsBase(card *ModelPricingEntry) bool {
	return card == nil || (card.BillingMode != BillingModePerRequest && card.BillingMode != BillingModeImage && card.BillingMode != BillingModeVideo)
}

// ResolvePriceCards combines an optional card with supplied base prices and
// source metadata. Explicit card amounts or token intervals select
// configuration-source resolution. Otherwise token mode retains the supplied
// base source and applies only supported card modifiers when a base pointer is
// available.
//
// A missing base remains unavailable rather than becoming free because
// modifiers exist. The long-context flag is set after source resolution,
// controlling built-in tiers separately from configured intervals. Result
// metadata may share source pointers, so callers validate inputs and keep them
// stable while the returned pricing is consumed.
// @project-doc docs/domains/routing_and_billing.md#group_model_pricing
func ResolvePriceCards(configPricing *ModelPricingEntry, base *ModelPricing, baseSource string, longContextEnabled bool) *ResolvedPricing {
	var resolved *ResolvedPricing
	if PriceCardOverrides(configPricing) {
		resolved = ResolveConfiguredPricing(configPricing, base, PricingSourceConfig)
	} else {
		resolved = &ResolvedPricing{
			Mode:                   BillingModeToken,
			BasePricing:            base,
			Source:                 baseSource,
			SupportsCacheBreakdown: base != nil && base.SupportsCacheBreakdown,
			SupportsServiceTier:    base != nil && base.SupportsServiceTier,
		}
		ApplyPricingModifiers(resolved, configPricing)
	}
	resolved.LongContextPricingEnabled = longContextEnabled
	return resolved
}
