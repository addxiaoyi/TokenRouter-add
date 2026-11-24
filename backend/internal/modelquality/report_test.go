package modelquality

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// report_test.go verifies complete-only scoring and bounded baseline comparisons.
// Fixtures distinguish operational failures from incorrect answers and keep
// historical changes comparable only within the same suite and time boundary.

// TestSummarizeCompleteOnly rejects scoring when any question could not produce an answer.
// Complete incorrect answers count toward the score, while operational errors
// preserve an incomplete result with no numeric score or baseline delta.
func TestSummarizeCompleteOnly(t *testing.T) {
	report := Report{Total: 8, SuiteVersion: SuiteVersion, CheckedAt: time.Now()}
	for range 7 {
		report.Probes = append(report.Probes, ProbeResult{Status: "pass"})
	}
	report.Probes = append(report.Probes, ProbeResult{Status: "error"})
	require.Nil(t, Summarize(report, nil).Score)
	report.Probes[7].Status = "fail"
	summary := Summarize(report, nil)
	require.Equal(t, 87, *summary.Score)
	require.Equal(t, "complete", summary.Status)
}

// TestSummarizeBaselineWindow distinguishes large declines from expired and future baselines.
// Only a completed comparable run in the previous thirty days contributes a delta.
func TestSummarizeBaselineWindow(t *testing.T) {
	now := time.Now()
	previousScore := 100
	report := Report{Total: 8, SuiteVersion: SuiteVersion, CheckedAt: now}
	for i := range 8 {
		status := "pass"
		if i < 2 {
			status = "fail"
		}
		report.Probes = append(report.Probes, ProbeResult{Status: status})
	}
	previous := Report{Status: "complete", SuiteVersion: SuiteVersion, Score: &previousScore, CheckedAt: now.Add(-time.Hour)}
	require.Equal(t, "retest_needed", Summarize(report, &previous).Signal)
	previous.CheckedAt = now.Add(-31 * 24 * time.Hour)
	require.Nil(t, Summarize(report, &previous).Delta)
	previous.CheckedAt = now.Add(time.Hour)
	require.Nil(t, Summarize(report, &previous).Delta)
}
