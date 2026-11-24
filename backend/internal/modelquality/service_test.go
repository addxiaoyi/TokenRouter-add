package modelquality

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// service_test.go exercises complete and interrupted assessment execution with
// in-memory boundaries. Provider answers follow the actual fixed suite, while
// injected failures verify that the service stops without grading missing output
// or automatically repeating a potentially billable question.

// assessmentStore captures progress and final status for synchronous worker tests.
type assessmentStore struct {
	Store
	saved []Job
} // ordered published job snapshots

// assessmentBackend supplies expected answers or one controlled operational failure.
type assessmentBackend struct {
	Backend
	calls   int
	failAt  int
	changed bool
} // executed fixture questions and failure selection

// Save records progress by value while retaining immutable probe fixture contents.
func (s *assessmentStore) Save(_ context.Context, job Job) error {
	s.saved = append(s.saved, job)
	return nil
}

// Baseline returns no prior assessment for isolated worker grading scenarios.
func (s *assessmentStore) Baseline(context.Context, Job) (*Report, error) { return nil, nil }

// Fingerprint returns the queued identity unless the fixture models changed configuration.
func (b *assessmentBackend) Fingerprint(context.Context, int64, int64, string) (string, error) {
	if b.changed {
		return "changed", nil
	}
	return "same", nil
}

// Answer returns the fixed suite's expected response or an injected request failure.
func (b *assessmentBackend) Answer(_ context.Context, _ Job, _ string) (string, error) {
	b.calls++
	if b.calls == b.failAt {
		return "", errors.New("fixture upstream failure")
	}
	return Suite()[b.calls-1].Expected, nil
}

// TestExecuteCompleteAndInterrupted distinguishes a full score from partial operational failure.
// No failed question is retried, and configuration changes stop before the first request.
func TestExecuteCompleteAndInterrupted(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		failAt   int
		changed  bool
		calls    int
		complete bool
	}{{"complete", 0, false, 8, true}, {"failure", 3, false, 3, false}, {"changed", 0, true, 0, false}} {
		t.Run(scenario.name, func(t *testing.T) {
			store := &assessmentStore{}
			backend := &assessmentBackend{failAt: scenario.failAt, changed: scenario.changed}
			service := NewService(store, backend)
			service.execute(t.Context(), Job{Fingerprint: "same", Report: Report{Status: "running", Total: 8, CheckedAt: time.Now(), SuiteVersion: SuiteVersion}})
			require.Equal(t, scenario.calls, backend.calls)
			last := store.saved[len(store.saved)-1].Report
			if scenario.complete {
				require.Equal(t, 100, *last.Score)
				require.Equal(t, "complete", last.Status)
			} else {
				require.Nil(t, last.Score)
				require.Equal(t, "incomplete", last.Status)
			}
		})
	}
}
