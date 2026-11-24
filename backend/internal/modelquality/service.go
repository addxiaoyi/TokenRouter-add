package modelquality

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// service.go validates assessment submissions and executes one leased fixed suite.
// Each question has a separate timeout, and a process-wide worker handles one
// job at a time. Database admission also limits active work across instances,
// so repeated clicks cannot start unbounded billable provider requests.
//
// Results persist after each question. Cancellation records incomplete progress,
// while crashed work expires without automatic replay because upstream calls
// may already have incurred cost. Configuration and administrator permissions
// are checked before every question. Only complete answers enter grading, and
// public reports never include the original answer or private failure details.

// Service owns validated assessment admission and durable execution dependencies.
// @project-doc docs/domains/model_quality.md#assessment_contract
type Service struct {
	Store   Store   // persisted assessment lifecycle
	Backend Backend // current configuration and provider execution
}

// NewService binds assessment storage and execution without starting background work.
func NewService(store Store, backend Backend) *Service {
	return &Service{Store: store, Backend: backend}
}

// Start validates a model selection before inserting a bounded idempotent job.
// Fingerprint verifies current administrative authority and configured model access.
// ClientID must remain stable across retries of the same submission; changing
// its model or label produces a conflict instead of altering committed work.
func (s *Service) Start(ctx context.Context, operator int64, input StartInput) (Report, error) {
	input.Model = strings.TrimSpace(input.Model)
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" {
		input.Label = input.Model
	}
	if operator <= 0 || input.ProviderID <= 0 || input.Model == "" || utf8.RuneCountInString(input.Model) > 200 || utf8.RuneCountInString(input.Label) > 80 {
		return Report{}, apperror.BadRequest("QUALITY_INVALID", "Select a provider, model, and a label of at most 80 characters")
	}
	if _, err := uuid.Parse(input.ClientID); err != nil {
		return Report{}, apperror.BadRequest("QUALITY_INVALID", "A valid submission identifier is required")
	}
	fingerprint, err := s.Backend.Fingerprint(ctx, operator, input.ProviderID, input.Model)
	if err != nil {
		return Report{}, err
	}
	report := Report{ID: uuid.NewString(), Label: input.Label, Model: input.Model, SuiteVersion: SuiteVersion, CheckedAt: time.Now().UTC(), Status: "queued", Signal: "queued", Total: len(Suite()), Probes: []ProbeResult{}}
	return s.Store.Enqueue(ctx, Job{ProviderID: input.ProviderID, OperatorID: operator, Fingerprint: fingerprint, Report: report}, input.ClientID)
}

// Run claims queued jobs until application shutdown cancels its worker context.
// Empty queues poll every five seconds. Storage failures are logged and pause
// admission until the next poll; claimed work runs serially through execute.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := s.Store.Claim(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "claim model assessment failed", "error", err)
		} else if job != nil {
			s.execute(ctx, *job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute evaluates one job while preserving progress after every completed question.
// It never retries a provider request automatically. A nine-minute job budget
// is shorter than the storage lease, while each question gets sixty seconds.
// Current configuration must match the queued fingerprint before every call;
// permission revocation or configuration change ends this run as incomplete.
// Final publication uses a separate bounded context so shutdown can retain
// partial progress without keeping the worker alive beyond its shutdown budget.
func (s *Service) execute(ctx context.Context, job Job) {
	started := time.Now()
	work, cancel := context.WithTimeout(ctx, 9*time.Minute)
	defer cancel()
	previous, err := s.Store.Baseline(work, job)
	if err == nil {
		for _, probe := range Suite() {
			questionStart := time.Now()
			entry := ProbeResult{ID: probe.ID, Category: probe.Category, Title: probe.Title, Status: "error"}
			fingerprint, checkErr := s.Backend.Fingerprint(work, job.OperatorID, job.ProviderID, job.Report.Model)
			if checkErr != nil || fingerprint != job.Fingerprint {
				entry.ErrorCode = "configuration_or_access_changed"
				job.Report.Probes = append(job.Report.Probes, entry)
				break
			}
			question, stop := context.WithTimeout(work, time.Minute)
			answer, answerErr := s.Backend.Answer(question, job, probe.Prompt)
			stop()
			entry.DurationMS = time.Since(questionStart).Milliseconds()
			if answerErr != nil || strings.TrimSpace(answer) == "" {
				entry.ErrorCode = "incomplete_response"
				if errors.Is(answerErr, context.DeadlineExceeded) {
					entry.ErrorCode = "timeout"
				}
			} else {
				entry.Status = "fail"
				if Matches(probe, answer) {
					entry.Status = "pass"
				}
			}
			job.Report.Probes = append(job.Report.Probes, entry)
			job.Report.DurationMS = time.Since(started).Milliseconds()
			if err = s.Store.Save(work, job); err != nil {
				slog.ErrorContext(ctx, "save model assessment progress failed", "id", job.Report.ID, "error", err)
				break
			}
			if entry.Status == "error" {
				break
			}
		}
	} else {
		slog.ErrorContext(ctx, "read model assessment baseline failed", "id", job.Report.ID, "error", err)
	}
	job.Report = Summarize(job.Report, previous)
	job.Report.DurationMS = time.Since(started).Milliseconds()
	// A canceled request can have consumed upstream capacity. Persist its partial
	// outcomes independently so a later worker never replays the same paid questions.
	final, finish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finish()
	if err = s.Store.Save(final, job); err != nil {
		slog.ErrorContext(ctx, "finish model assessment failed", "id", job.Report.ID, "error", err)
	}
}
