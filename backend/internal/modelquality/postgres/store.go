package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/modelquality"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// store.go persists model assessment admission, progress, and public history.
// A partial unique index permits one active job across all instances. Each claim
// assigns a fresh lease identity, and progress updates require that identity,
// preventing expired workers from overwriting a completed or abandoned report.
//
// Client UUIDs identify retried submissions independently from job identifiers.
// Comparable jobs share an opaque series UUID selected from their private
// provider configuration and suite. Expired running jobs become incomplete;
// they are never requeued automatically because execution may already have cost
// upstream resources. Private execution columns never enter public report JSON.

// Store owns assessment SQL operations over a caller-managed database pool.
type Store struct{ db *sql.DB } // shared application connection pool

// NewStore binds the application pool without issuing queries or owning shutdown.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// decode reads one safe report and maps missing records to the public error.
func decode(row *sql.Row) (modelquality.Report, error) {
	var report modelquality.Report
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return report, apperror.NotFound("QUALITY_NOT_FOUND", "Assessment not found")
		}
		return report, err
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return report, err
	}
	return report, nil
}

// Get returns the report projection for one known assessment UUID.
func (s *Store) Get(ctx context.Context, id string) (modelquality.Report, error) {
	return decode(s.db.QueryRowContext(ctx, `SELECT report FROM model_quality_jobs WHERE id=$1`, id))
}

// Active reads the current queued or running report for administrator recovery.
// An empty queue returns nil. Operators can recover progress after page reload
// without retaining a private provider identifier or credential in browser storage.
func (s *Store) Active(ctx context.Context) (*modelquality.Report, error) {
	report, err := decode(s.db.QueryRowContext(ctx, `SELECT report FROM model_quality_jobs WHERE status IN ('queued','running') LIMIT 1`))
	if apperror.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// Enqueue stores one new assessment or returns an identical retried submission.
// Global active-job conflicts report a recoverable conflict. The retry lookup
// verifies operator, provider, model, and label before returning an existing job,
// so another request cannot claim unrelated progress by guessing its UUID.
func (s *Store) Enqueue(ctx context.Context, job modelquality.Job, clientID string) (modelquality.Report, error) {
	var series string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT series_id FROM model_quality_jobs WHERE provider_id=$1 AND model=$2 AND fingerprint=$3 AND suite_version=$4 ORDER BY created_at DESC LIMIT 1),gen_random_uuid())`, job.ProviderID, job.Report.Model, job.Fingerprint, job.Report.SuiteVersion).Scan(&series)
	if err != nil {
		return modelquality.Report{}, err
	}
	job.Report.SeriesID = series
	raw, err := json.Marshal(job.Report)
	if err != nil {
		return modelquality.Report{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_quality_jobs(id,provider_id,operator_id,model,fingerprint,suite_version,series_id,client_id,status,report) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'queued',$9)`, job.Report.ID, job.ProviderID, job.OperatorID, job.Report.Model, job.Fingerprint, job.Report.SuiteVersion, series, clientID, raw)
	if err == nil {
		return job.Report, nil
	}
	var conflict *pq.Error
	if !errors.As(err, &conflict) || conflict.Code != "23505" {
		return modelquality.Report{}, err
	}
	existing, readErr := decode(s.db.QueryRowContext(ctx, `SELECT report FROM model_quality_jobs WHERE client_id=$1 AND operator_id=$2 AND provider_id=$3 AND model=$4 AND report->>'label'=$5`, clientID, job.OperatorID, job.ProviderID, job.Report.Model, job.Report.Label))
	if readErr == nil {
		return existing, nil
	}
	if !errors.Is(readErr, sql.ErrNoRows) && !apperror.IsNotFound(readErr) {
		return modelquality.Report{}, readErr
	}
	return modelquality.Report{}, apperror.Conflict("QUALITY_BUSY", "An assessment is already active or the submission identifier was reused")
}

// List reads the latest two hundred safe reports in reverse creation order.
// Queued and running jobs remain visible only through administrator detail reads;
// public history contains completed or interrupted assessments with sanitized outcomes.
func (s *Store) List(ctx context.Context) (_ []modelquality.Report, returnErr error) {
	reports := []modelquality.Report{}
	rows, err := s.db.QueryContext(ctx, `SELECT report FROM model_quality_jobs WHERE status IN ('complete','incomplete') ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
	for rows.Next() {
		var raw []byte
		var report modelquality.Report
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &report); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

// Claim marks expired work incomplete and leases one queued assessment atomically.
// A ten-minute lease outlives the worker's bounded execution. Abandoned work
// preserves its partial report and cannot be replayed by a future claim.
func (s *Store) Claim(ctx context.Context) (*modelquality.Job, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE model_quality_jobs SET status='incomplete',report=report||'{"status":"incomplete","signal":"interrupted","score":null}'::jsonb,updated_at=NOW() WHERE status='running' AND lease_until<NOW()`)
	if err != nil {
		return nil, err
	}
	var job modelquality.Job
	var raw []byte
	err = s.db.QueryRowContext(ctx, `WITH queued AS (SELECT id FROM model_quality_jobs WHERE status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE model_quality_jobs j SET status='running',lease_id=gen_random_uuid(),lease_until=NOW()+INTERVAL '10 minutes',report=report||'{"status":"running","signal":"running"}'::jsonb,updated_at=NOW() FROM queued WHERE j.id=queued.id RETURNING j.provider_id,j.operator_id,j.fingerprint,j.lease_id,j.report`).Scan(&job.ProviderID, &job.OperatorID, &job.Fingerprint, &job.LeaseID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &job.Report); err != nil {
		return nil, err
	}
	return &job, nil
}

// Save publishes progress only while this worker owns the current running lease.
// Final statuses release global admission. A lost lease returns an error rather
// than silently accepting progress that another worker has already invalidated.
func (s *Store) Save(ctx context.Context, job modelquality.Job) error {
	raw, err := json.Marshal(job.Report)
	if err != nil {
		return err
	}
	change, err := s.db.ExecContext(ctx, `UPDATE model_quality_jobs SET report=$3,status=$4,updated_at=NOW() WHERE id=$1 AND lease_id=$2 AND status='running'`, job.Report.ID, job.LeaseID, raw, job.Report.Status)
	if err != nil {
		return err
	}
	count, err := change.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("assessment %s no longer owns its lease", job.Report.ID)
	}
	return nil
}

// Baseline finds the newest complete result with identical private configuration.
// No matching result returns nil; JSON corruption remains an error. Report
// summarization independently checks chronological order and the thirty-day limit.
func (s *Store) Baseline(ctx context.Context, job modelquality.Job) (*modelquality.Report, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT report FROM model_quality_jobs WHERE provider_id=$1 AND model=$2 AND fingerprint=$3 AND suite_version=$4 AND status='complete' AND created_at<$5 AND created_at>=$5::timestamptz-INTERVAL '30 days' ORDER BY created_at DESC LIMIT 1`, job.ProviderID, job.Report.Model, job.Fingerprint, job.Report.SuiteVersion, job.Report.CheckedAt).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var report modelquality.Report
	if err = json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	return &report, nil
}
