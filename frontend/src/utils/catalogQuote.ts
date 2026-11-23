import type { MarketplaceGroup, MarketplaceModel } from '@/api/marketplace.types'
import {
  buildModelIndex,
  compareMarketplaceGroups,
  type ModelSearchEntry,
  type ModelPriceSummary,
} from './marketplaceCatalog'

// catalogQuote.ts connects an indexed model summary to its original routing quote.
// A model can appear in several groups with different prices. The compact card
// must obtain cache pricing and monitoring identity from the same group whose
// input/output pair supplied its summary, preserving a coherent quoted offer.
// Comparing only the input side could attach a cache price to a different offer
// when two routes share their input amount but charge different output amounts.
//
// Candidate groups follow the same stable order used by the catalog index, which
// determines the winning identity when primary quotes tie. Missing matches remain
// unavailable so a stale or inconsistent index cannot display cache pricing or
// monitoring from an unrelated offer. Unpriced entries can still retain their
// original capability metadata through an equally unpriced candidate.

// CatalogQuote identifies one original model offer and its routing group.
export interface CatalogQuote {
  group: MarketplaceGroup // Authorized original routing group.
  model: MarketplaceModel // Original group-specific model quote.
}

// samePrice compares the displayed billing unit and primary quote values.
// Variation flags describe the whole catalog and are intentionally excluded, since
// a single original offer cannot vary across the other groups in that catalog.
function samePrice(left: ModelPriceSummary, right: ModelPriceSummary): boolean {
  if (left.kind !== right.kind) return false
  if (left.kind === 'unpriced') return true
  if (left.kind === 'token' && right.kind === 'token')
    return left.input === right.input && left.output === right.output
  return 'amount' in left && 'amount' in right && left.amount === right.amount
}

// catalogQuote finds the original group that supplies an indexed price pair.
// Callers pass the already filtered, authorized group set. Each candidate is
// summarized independently using the same quote rules as the model index, keeping
// context defaults, explicit zeros, and billing units consistent during selection.
// A match preserves both price sides from one offer; cache prices and group
// observations can then be attached without borrowing data from another route.
// Unpriced summaries match unpriced originals; an inconsistent priced summary
// remains unavailable instead of borrowing another group's auxiliary metadata.
export function catalogQuote(
  entry: ModelSearchEntry,
  groups: readonly MarketplaceGroup[],
): CatalogQuote | undefined {
  for (const group of [...groups].sort(compareMarketplaceGroups)) {
    const model = group.models.find((candidate) => candidate.id === entry.id)
    if (!model) continue
    const price = buildModelIndex([{ ...group, models: [model] }])[0]?.price
    if (price && samePrice(entry.price, price)) return { group, model }
  }
  return undefined
}
