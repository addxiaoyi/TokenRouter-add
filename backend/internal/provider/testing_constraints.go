package provider

import "context"

// testing_constraints.go carries bounded assessment requirements through native
// provider test adapters. Ordinary connection tests retain zero-value behavior;
// model assessments require protocol completion and a bounded output allowance.
// The requirements belong to one test context and never modify saved providers.

// TestConstraints specifies additional requirements for a scored provider assessment.
type TestConstraints struct {
	OutputTokens int  // positive requested output token cap
	Complete     bool // require explicit successful protocol completion
}

// testConstraintsKey isolates assessment metadata from other provider context values.
type testConstraintsKey struct{}

// WithTestConstraints adds immutable per-request assessment requirements to a context.
// Only internal callers set these values. The derived context preserves deadline
// and cancellation, while ordinary tests with no value retain existing behavior.
func WithTestConstraints(ctx context.Context, constraints TestConstraints) context.Context {
	return context.WithValue(ctx, testConstraintsKey{}, constraints)
}

// TestConstraintsFromContext returns assessment requirements or their ordinary-test defaults.
func TestConstraintsFromContext(ctx context.Context) TestConstraints {
	constraints, _ := ctx.Value(testConstraintsKey{}).(TestConstraints)
	return constraints
}
