package modelquality

import "time"

// report.go computes scores only for fully answered assessments and compares
// compatible recent baselines. Network failures and missing answers remain
// incomplete, preserving the distinction between an incorrect answer and an
// upstream service that could not deliver a complete response.
//
// Comparable runs share provider configuration, model, and suite at storage.
// The report layer additionally enforces the thirty-day time boundary before
// calculating a score change. A decrease of at least twenty-five points suggests
// another assessment, while incomplete reports retain no numeric score and
// cannot become a future baseline for another completed assessment.

// Summarize derives answer counts, a complete score, and a recent comparison.
// The caller selects a previous report with identical private configuration.
// Scores remain nil until every expected question has a completed answer, so
// network errors never count as incorrect model responses. A baseline must
// precede this run, use the same suite, and fall within thirty days; comparisons
// outside that boundary leave the completed result marked as its first run.
func Summarize(report Report, previous *Report) Report {
	report.Score, report.PreviousScore, report.Delta, report.BaselineAt = nil, nil, nil, nil
	report.Passed, report.Answered = 0, 0
	for _, probe := range report.Probes {
		if probe.Status == "pass" {
			report.Passed++
		}
		if probe.Status == "pass" || probe.Status == "fail" {
			report.Answered++
		}
	}
	report.Status, report.Signal = "incomplete", "incomplete"
	if report.Total == 0 || report.Answered != report.Total {
		return report
	}
	report.Status, report.Signal = "complete", "first_run"
	score := report.Passed * 100 / report.Total
	report.Score = &score
	if previous == nil || previous.Score == nil || previous.Status != "complete" || previous.SuiteVersion != report.SuiteVersion {
		return report
	}
	if previous.CheckedAt.After(report.CheckedAt) || report.CheckedAt.Sub(previous.CheckedAt) > 30*24*time.Hour {
		return report
	}
	delta := score - *previous.Score
	report.PreviousScore, report.Delta, report.BaselineAt = previous.Score, &delta, &previous.CheckedAt
	report.Signal = "stable"
	if delta <= -25 {
		report.Signal = "retest_needed"
	}
	if delta > 0 {
		report.Signal = "improved"
	}
	return report
}
