package provider

import "testing"

// cost_rate_test.go checks whether upstream costs have an intentional multiplier.
// The legacy default of one remains unconfirmed until the administrator verifies
// it, while an explicit free rate and existing nondefault rates retain meaning.

// TestCostRateConfirmed distinguishes default, explicit, and subsequently edited
// multipliers. Changing a previously confirmed rate must not confirm a new default.
func TestCostRateConfirmed(t *testing.T) {
	for _, test := range []struct {
		name  string         // fixture case name
		rate  *float64       // stored upstream multiplier
		extra map[string]any // explicit confirmation evidence
		want  bool           // expected confirmation state
	}{
		{name: "missing"},
		{name: "default", rate: new(1.0)},
		{name: "free", rate: new(0.0), want: true},
		{name: "configured", rate: new(0.2), want: true},
		{name: "confirmed one", rate: new(1.0), extra: map[string]any{"profit_cost_rate": 1.0}, want: true},
		{name: "stale confirmation", rate: new(1.0), extra: map[string]any{"profit_cost_rate": 0.2}},
		{name: "invalid", rate: new(-1.0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := &Record{RateMultiplier: test.rate, Extra: FilterSchedulerExtra(test.extra)}
			if got := row.CostRateConfirmed(); got != test.want {
				t.Fatalf("CostRateConfirmed() = %v, want %v", got, test.want)
			}
		})
	}
}
