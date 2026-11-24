package modelquality

import (
	"context"
	"time"
)

// models.go defines persisted quality jobs and their public assessment reports.
// Provider identity and configuration fingerprints remain private job fields.
// Public reports retain only model labels, fixed-suite outcomes, timings, and
// comparison identifiers, keeping upstream answers and credentials out of responses.

// Target identifies one eligible provider and its configured text models.
type Target struct {
	ID     int64    `json:"id"`     // administrator provider identifier
	Name   string   `json:"name"`   // administrator provider label
	Models []string `json:"models"` // supported configured text models
}

// ProbeResult records one completed answer or an operational test failure.
type ProbeResult struct {
	ID         string `json:"id"`                   // stable suite question identifier
	Category   string `json:"category"`             // public question category
	Title      string `json:"title"`                // public question display name
	Status     string `json:"status"`               // pass, fail, or error
	DurationMS int64  `json:"duration_ms"`          // elapsed milliseconds
	ErrorCode  string `json:"error_code,omitempty"` // sanitized operational failure category
}

// Report is the public projection of one bounded model assessment.
type Report struct {
	ID            string        `json:"id"`             // opaque assessment identifier
	SeriesID      string        `json:"series_id"`      // opaque comparable configuration identifier
	Label         string        `json:"label"`          // administrator supplied public label
	Model         string        `json:"model"`          // public tested model name
	SuiteVersion  string        `json:"suite_version"`  // versioned question and execution contract
	CheckedAt     time.Time     `json:"checked_at"`     // assessment start time
	Status        string        `json:"status"`         // queued, running, complete, or incomplete
	Score         *int          `json:"score"`          // percentage; nil for incomplete assessment
	PreviousScore *int          `json:"previous_score"` // comparable previous percentage
	Delta         *int          `json:"delta"`          // difference in percentage points
	BaselineAt    *time.Time    `json:"baseline_at"`    // previous comparable assessment time
	Signal        string        `json:"signal"`         // comparison or incomplete outcome
	Passed        int           `json:"passed"`         // correctly answered question count
	Answered      int           `json:"answered"`       // complete valid response count
	Total         int           `json:"total"`          // required suite question count
	DurationMS    int64         `json:"duration_ms"`    // total elapsed milliseconds
	Probes        []ProbeResult `json:"probes"`         // ordered sanitized question outcomes
}

// Job retains private execution identity beside a safe public report.
type Job struct {
	ProviderID  int64  // selected provider identifier
	OperatorID  int64  // initiating administrator identifier
	Fingerprint string // private configuration digest
	LeaseID     string // claimed worker identity
	Report      Report // current assessment progress
}

// StartInput selects a configured provider model and its public display label.
type StartInput struct {
	ProviderID int64  `json:"provider_id"` // selected provider identifier
	Model      string `json:"model"`       // selected configured model
	Label      string `json:"label"`       // optional public display label
	ClientID   string `json:"client_id"`   // stable submission UUID
}

// Backend validates current permissions and executes bounded provider assessments.
type Backend interface {
	// Targets lists administrator-visible providers with eligible configured text models.
	Targets(context.Context) ([]Target, error)
	// Fingerprint verifies current administrator access and captures comparable provider configuration.
	Fingerprint(context.Context, int64, int64, string) (string, error)
	// Answer runs one prompt with the job's verified configuration and completion rules.
	// Operational failures return an error; the service never grades partial output.
	Answer(context.Context, Job, string) (string, error)
}

// Store persists queued work, public progress, and independently leased execution.
type Store interface {
	// Enqueue inserts one idempotent job or returns its already persisted report.
	Enqueue(context.Context, Job, string) (Report, error)
	// Get reads one public projection or reports an unknown assessment identifier.
	Get(context.Context, string) (Report, error)
	// List returns bounded recent public reports without private provider configuration.
	List(context.Context) ([]Report, error)
	// Active returns the queued or running assessment for administrator recovery.
	Active(context.Context) (*Report, error)
	// Claim obtains one queued job and marks abandoned work incomplete without replaying it.
	Claim(context.Context) (*Job, error)
	// Save publishes progress only while the claimed job still belongs to this worker.
	Save(context.Context, Job) error
	// Baseline finds a completed assessment with the same configuration and suite.
	Baseline(context.Context, Job) (*Report, error)
}
