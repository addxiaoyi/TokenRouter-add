package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// test_streams_test.go verifies strict completion requirements used by assessments.
// Ordinary connection probes retain their compatibility behavior, while scored
// output rejects truncated responses and EOF without a protocol terminal event.

// assessmentSink accepts fixture output without writing to an HTTP connection.
type assessmentSink struct{}

// Begin accepts fixture stream setup without committing any external response.
func (assessmentSink) Begin(context.Context, bool) error { return nil }

// Emit accepts typed fixture events for stream parser completion assertions.
func (assessmentSink) Emit(context.Context, provider.TestEvent) error { return nil }

// TestAssessmentRejectsIncompleteStreams supplies truncated Chat and unterminated Messages output.
// Strict runs fail both cases, preventing operational truncation from receiving an incorrect-answer score.
func TestAssessmentRejectsIncompleteStreams(t *testing.T) {
	ctx := provider.WithTestConstraints(t.Context(), provider.TestConstraints{Complete: true, OutputTokens: 1024})
	run := NewTestRun(ctx, nil, assessmentSink{})
	defer run.Cancel()
	require.Error(t, (TestStreamOutput{}).ChatCompletions(run, strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"2\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")))
	require.Error(t, (TestStreamOutput{}).Anthropic(run, strings.NewReader("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"2\"}}\n\n")))
}

// TestAssessmentAcceptsCompletedStream provides a complete answer and explicit stop reason.
// Strict completion must retain successful ordinary output while applying its native token budget.
func TestAssessmentAcceptsCompletedStream(t *testing.T) {
	ctx := provider.WithTestConstraints(t.Context(), provider.TestConstraints{Complete: true, OutputTokens: 1024})
	run := NewTestRun(ctx, nil, assessmentSink{})
	defer run.Cancel()
	require.NoError(t, (TestStreamOutput{}).ChatCompletions(run, strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"247\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")))
	payload := map[string]any{"max_tokens": 256}
	applyTestOutputBudget(run, "openai_chat_completions", payload)
	require.Equal(t, 1024, payload["max_completion_tokens"])
	require.NotContains(t, payload, "max_tokens")
}
