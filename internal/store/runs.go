package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// runColumns must stay in store.Run field order: single-row scans are positional.
// Alembic-created databases append schedule_id/scheduled_for at the end, so
// RETURNING */SELECT * cannot be scanned positionally into Run.
const runColumns = `id, definition_id, definition_version_id, target_snapshot,
    schedule_id, scheduled_for, trigger_type, status, idempotency_key, request_hash,
    parameters, parameters_redacted, queued_at, started_at, finished_at, next_retry_at,
    attempt_count, max_attempts, worker_id, lease_expires_at, parent_run_id,
    cancel_requested_at, cancel_requested_by, error_code, error_message,
    result_summary, created_by, created_at, updated_at, version, timeout_seconds`

func (s *Store) GetRun(ctx context.Context, q Querier, id string) (*Run, error) {
	return One[Run](q.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id=$1`, id))
}

func (s *Store) ListRuns(ctx context.Context, q Querier, limit, offset int, definitionID, status *string) ([]Run, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if definitionID != nil {
		args = append(args, *definitionID)
		where += " AND definition_id=$" + itoa(len(args))
	}
	if status != nil && *status != "" {
		args = append(args, *status)
		where += " AND status=$" + itoa(len(args))
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM runs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	query := `SELECT * FROM runs` + where +
		` ORDER BY created_at DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	items, err := Many[Run](q.Query(ctx, query, args...))
	return items, total, err
}

func (s *Store) CreateRun(ctx context.Context, q Querier, r *Run) error {
	row := q.QueryRow(ctx, `
INSERT INTO runs (id, definition_id, definition_version_id, target_snapshot,
    schedule_id, scheduled_for, trigger_type, status, idempotency_key, request_hash,
    parameters, parameters_redacted, queued_at, started_at, finished_at, next_retry_at,
    attempt_count, max_attempts, worker_id, lease_expires_at, parent_run_id,
    cancel_requested_at, cancel_requested_by, error_code, error_message,
    result_summary, created_by, created_at, updated_at, version, timeout_seconds)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
    $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)
RETURNING `+runColumns,
		r.ID, r.DefinitionID, r.DefinitionVersionID, r.TargetSnapshot, r.ScheduleID,
		r.ScheduledFor, r.TriggerType, r.Status, r.IdempotencyKey, r.RequestHash,
		r.Parameters, r.ParametersRedacted, r.QueuedAt, r.StartedAt, r.FinishedAt,
		r.NextRetryAt, r.AttemptCount, r.MaxAttempts, r.WorkerID, r.LeaseExpiresAt,
		r.ParentRunID, r.CancelRequestedAt, r.CancelRequestedBy, r.ErrorCode,
		r.ErrorMessage, r.ResultSummary, r.CreatedBy, r.CreatedAt, r.UpdatedAt, r.Version,
		r.TimeoutSeconds)
	return scanInto(row, r)
}

func (s *Store) SaveRun(ctx context.Context, q Querier, r *Run) error {
	r.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE runs SET target_snapshot=$2, schedule_id=$3, scheduled_for=$4, trigger_type=$5,
    status=$6, idempotency_key=$7, request_hash=$8, parameters=$9, parameters_redacted=$10,
    queued_at=$11, started_at=$12, finished_at=$13, next_retry_at=$14, attempt_count=$15,
    max_attempts=$16, worker_id=$17, lease_expires_at=$18, parent_run_id=$19,
    cancel_requested_at=$20, cancel_requested_by=$21, error_code=$22, error_message=$23,
    result_summary=$24, updated_at=$25, version=$26
WHERE id=$1 RETURNING `+runColumns,
		r.ID, r.TargetSnapshot, r.ScheduleID, r.ScheduledFor, r.TriggerType, r.Status,
		r.IdempotencyKey, r.RequestHash, r.Parameters, r.ParametersRedacted,
		r.QueuedAt, r.StartedAt, r.FinishedAt, r.NextRetryAt, r.AttemptCount,
		r.MaxAttempts, r.WorkerID, r.LeaseExpiresAt, r.ParentRunID,
		r.CancelRequestedAt, r.CancelRequestedBy, r.ErrorCode, r.ErrorMessage,
		r.ResultSummary, r.UpdatedAt, r.Version)
	return scanInto(row, r)
}

func (s *Store) DeleteRun(ctx context.Context, q Querier, id string) (bool, error) {
	tag, err := q.Exec(ctx, `DELETE FROM runs WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) FindRunByIdempotency(ctx context.Context, q Querier, userID, key string) (*Run, error) {
	return One[Run](q.QueryRow(ctx, `
SELECT `+runColumns+` FROM runs WHERE created_by=$1 AND idempotency_key=$2`, userID, key))
}

func (s *Store) AttemptsFor(ctx context.Context, q Querier, runID string) ([]Attempt, error) {
	return Many[Attempt](q.Query(ctx, `
SELECT * FROM attempts WHERE run_id=$1 ORDER BY attempt_no`, runID))
}

func (s *Store) GetAttempt(ctx context.Context, q Querier, id string) (*Attempt, error) {
	return One[Attempt](q.QueryRow(ctx, `SELECT * FROM attempts WHERE id=$1`, id))
}

func (s *Store) CreateAttempt(ctx context.Context, q Querier, a *Attempt) error {
	row := q.QueryRow(ctx, `
INSERT INTO attempts (id, run_id, attempt_no, status, executor_type, target_id,
    worker_id, started_at, finished_at, duration_ms, exit_code, http_status,
    error_class, error_message, stdout_excerpt, stderr_excerpt, response_excerpt,
    truncated, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
RETURNING *`,
		a.ID, a.RunID, a.AttemptNo, a.Status, a.ExecutorType, a.TargetID,
		a.WorkerID, a.StartedAt, a.FinishedAt, a.DurationMS, a.ExitCode, a.HTTPStatus,
		a.ErrorClass, a.ErrorMessage, a.StdoutExcerpt, a.StderrExcerpt,
		a.ResponseExcerpt, a.Truncated, a.CreatedAt)
	return scanInto(row, a)
}

func (s *Store) SaveAttempt(ctx context.Context, q Querier, a *Attempt) error {
	row := q.QueryRow(ctx, `
UPDATE attempts SET status=$2, target_id=$3, worker_id=$4, finished_at=$5,
    duration_ms=$6, exit_code=$7, http_status=$8, error_class=$9, error_message=$10,
    stdout_excerpt=$11, stderr_excerpt=$12, response_excerpt=$13, truncated=$14
WHERE id=$1 RETURNING *`,
		a.ID, a.Status, a.TargetID, a.WorkerID, a.FinishedAt, a.DurationMS,
		a.ExitCode, a.HTTPStatus, a.ErrorClass, a.ErrorMessage, a.StdoutExcerpt,
		a.StderrExcerpt, a.ResponseExcerpt, a.Truncated)
	return scanInto(row, a)
}

type Claimed struct {
	Run     *Run
	Version *DefinitionVersion
	Attempt *Attempt
}

func (s *Store) ClaimRun(ctx context.Context, q Querier, workerID string, leaseSeconds int) (*Claimed, error) {
	var runID string
	err := q.QueryRow(ctx, `
SELECT r.id FROM runs r
JOIN task_definition_runtime rt ON rt.definition_id = r.definition_id
WHERE r.status='queued' AND rt.running_count < rt.max_concurrency
ORDER BY r.queued_at, r.id
FOR UPDATE OF r SKIP LOCKED
LIMIT 1`).Scan(&runID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	run, err := One[Run](q.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id=$1 FOR UPDATE`, runID))
	if err != nil || run == nil {
		return nil, err
	}
	version, err := s.GetVersion(ctx, q, run.DefinitionVersionID)
	if err != nil {
		return nil, err
	}
	if version == nil {
		now := time.Now().UTC()
		run.Status = "failed"
		run.ErrorCode = strptr("DEFINITION_VERSION_MISSING")
		run.FinishedAt = &now
		run.Version++
		if err := s.SaveRun(ctx, q, run); err != nil {
			return nil, err
		}
		return nil, nil
	}
	ok, err := s.IncrementRuntime(ctx, q, run.DefinitionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	now := time.Now().UTC()
	lease := now.Add(time.Duration(leaseSeconds) * time.Second)
	run.Status = "running"
	run.StartedAt = &now
	run.WorkerID = &workerID
	run.LeaseExpiresAt = &lease
	run.AttemptCount++
	if err := s.SaveRun(ctx, q, run); err != nil {
		return nil, err
	}
	attempt := &Attempt{
		ID: NewID(), RunID: run.ID, AttemptNo: run.AttemptCount, Status: "running",
		ExecutorType: version.ExecutorType, WorkerID: &workerID, StartedAt: now,
		CreatedAt: now,
	}
	if err := s.CreateAttempt(ctx, q, attempt); err != nil {
		return nil, err
	}
	return &Claimed{Run: run, Version: version, Attempt: attempt}, nil
}

func (s *Store) ReapExpired(ctx context.Context, q Querier) (int, error) {
	rows, err := q.Query(ctx, `
SELECT * FROM runs
WHERE status='running' AND lease_expires_at IS NOT NULL AND lease_expires_at < now()
FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	runs, err := pgx.CollectRows(rows, pgx.RowToStructByName[Run])
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	for i := range runs {
		run := &runs[i]
		attempts, err := s.AttemptsFor(ctx, q, run.ID)
		if err != nil {
			return 0, err
		}
		for j := range attempts {
			attempt := &attempts[j]
			if attempt.Status == "running" {
				attempt.Status = "failed"
				attempt.FinishedAt = &now
				attempt.ErrorClass = strptr("WORKER_LOST")
				attempt.ErrorMessage = strptr("worker lease expired")
				if err := s.SaveAttempt(ctx, q, attempt); err != nil {
					return 0, err
				}
			}
		}
		run.Status = "failed"
		run.ErrorCode = strptr("WORKER_LOST")
		run.ErrorMessage = strptr("worker lease expired")
		run.FinishedAt = &now
		run.LeaseExpiresAt = nil
		run.Version++
		if err := s.SaveRun(ctx, q, run); err != nil {
			return 0, err
		}
		if err := s.DecrementRuntime(ctx, q, run.DefinitionID); err != nil {
			return 0, err
		}
		audit := &AuditEvent{
			Action: "run.failed", ResourceType: "task_run", ResourceID: &run.ID,
			CorrelationID: "-", Details: JSONB{"reaper": true},
		}
		if err := s.InsertAudit(ctx, q, audit); err != nil {
			return 0, err
		}
	}
	return len(runs), nil
}

func (s *Store) RenewLease(ctx context.Context, q Querier, runID string, leaseSeconds int) error {
	_, err := q.Exec(ctx, `
UPDATE runs SET lease_expires_at=now() + ($2 * interval '1 second')
WHERE id=$1 AND status='running'`, runID, leaseSeconds)
	return err
}

func (s *Store) CountRunningForDefinition(ctx context.Context, q Querier, definitionID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM runs WHERE definition_id=$1 AND status='running'`, definitionID).Scan(&n)
	return n, err
}

func (s *Store) DeleteRunsForDefinition(ctx context.Context, q Querier, definitionID string) ([]string, error) {
	rows, err := q.Query(ctx, `DELETE FROM runs WHERE definition_id=$1 RETURNING id`, definitionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
