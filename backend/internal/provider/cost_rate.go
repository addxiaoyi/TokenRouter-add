package provider

import "math"

// cost_rate.go identifies upstream multipliers suitable for profit reporting.
// Provider creation historically supplies one even without an administrator's
// input. Existing nondefault rates establish configuration; an intentional rate
// of one needs matching confirmation evidence in the provider's Extra map.

// CostRateConfirmed reports whether the stored rate has configuration evidence.
// An absent or invalid multiplier is unconfirmed. Explicit zero is a valid free
// cost, and confirmation of one is tied to that value rather than future edits.
func (r *Record) CostRateConfirmed() bool {
	if r == nil || r.RateMultiplier == nil {
		return false
	}
	rate := *r.RateMultiplier
	if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return false
	}
	if rate != 1 {
		return true
	}
	confirmed, ok := r.Extra["profit_cost_rate"].(float64)
	return ok && confirmed == rate
}
