package store

import (
	"context"
	"time"
)

func (s *Store) GetSchedule(ctx context.Context, q Querier, id string) (*Schedule, error) {
	return One[Schedule](q.QueryRow(ctx, `SELECT * FROM schedules WHERE id=$1`, id))
}

func (s *Store) ListSchedules(ctx context.Context, q Querier, limit, offset int, definitionID, status *string) ([]Schedule, int, error) {
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
	if err := q.QueryRow(ctx, `SELECT count(*) FROM schedules`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	query := `SELECT * FROM schedules` + where +
		` ORDER BY created_at DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	items, err := Many[Schedule](q.Query(ctx, query, args...))
	return items, total, err
}

func (s *Store) CreateSchedule(ctx context.Context, q Querier, schedule *Schedule) error {
	row := q.QueryRow(ctx, `
INSERT INTO schedules (id, definition_id, definition_version_id, name, trigger_type,
    fire_at, cron_expr, interval_seconds, timezone, parameters, parameters_redacted,
    overlap_policy, misfire_grace_seconds, status, next_fire_at, last_fire_at,
    created_by, created_at, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
RETURNING *`,
		schedule.ID, schedule.DefinitionID, schedule.DefinitionVersionID, schedule.Name,
		schedule.TriggerType, schedule.FireAt, schedule.CronExpr, schedule.IntervalSeconds,
		schedule.Timezone, schedule.Parameters, schedule.ParametersRedacted,
		schedule.OverlapPolicy, schedule.MisfireGraceSeconds, schedule.Status,
		schedule.NextFireAt, schedule.LastFireAt, schedule.CreatedBy, schedule.CreatedAt,
		schedule.UpdatedAt, schedule.Version)
	return scanInto(row, schedule)
}

func (s *Store) SaveSchedule(ctx context.Context, q Querier, schedule *Schedule) error {
	schedule.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE schedules SET definition_id=$2, definition_version_id=$3, name=$4,
    trigger_type=$5, fire_at=$6, cron_expr=$7, interval_seconds=$8, timezone=$9,
    parameters=$10, parameters_redacted=$11, overlap_policy=$12,
    misfire_grace_seconds=$13, status=$14, next_fire_at=$15, last_fire_at=$16,
    updated_at=$17, version=$18
WHERE id=$1 RETURNING *`,
		schedule.ID, schedule.DefinitionID, schedule.DefinitionVersionID, schedule.Name,
		schedule.TriggerType, schedule.FireAt, schedule.CronExpr, schedule.IntervalSeconds,
		schedule.Timezone, schedule.Parameters, schedule.ParametersRedacted,
		schedule.OverlapPolicy, schedule.MisfireGraceSeconds, schedule.Status,
		schedule.NextFireAt, schedule.LastFireAt, schedule.UpdatedAt, schedule.Version)
	return scanInto(row, schedule)
}

func (s *Store) DeleteSchedule(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM schedules WHERE id=$1`, id)
	return err
}

func (s *Store) DeleteAllSchedules(ctx context.Context, q Querier) (int, error) {
	tag, err := q.Exec(ctx, `DELETE FROM schedules`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) ListDueSchedules(ctx context.Context, q Querier, now time.Time, limit int) ([]Schedule, error) {
	return Many[Schedule](q.Query(ctx, `
SELECT * FROM schedules
WHERE status='active' AND next_fire_at <= $1
ORDER BY next_fire_at
FOR UPDATE SKIP LOCKED
LIMIT $2`, now, limit))
}

func (s *Store) ScheduleOccurrenceExists(ctx context.Context, q Querier, scheduleID string, scheduledFor time.Time) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
SELECT EXISTS (SELECT 1 FROM runs WHERE schedule_id=$1 AND scheduled_for=$2)`,
		scheduleID, scheduledFor).Scan(&exists)
	return exists, err
}

func (s *Store) ScheduleHasActiveRun(ctx context.Context, q Querier, scheduleID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM runs WHERE schedule_id=$1 AND status IN ('queued','running','retry_wait')
)`, scheduleID).Scan(&exists)
	return exists, err
}

func (s *Store) PromoteRetryWait(ctx context.Context, q Querier, now time.Time) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE runs SET status='queued', queued_at=$1, next_retry_at=NULL,
    updated_at=now(), version=version+1
WHERE status='retry_wait' AND next_retry_at IS NOT NULL AND next_retry_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CancelScheduledRunsForDefinition(ctx context.Context, q Querier, definitionID, code, message string) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE runs SET status='cancelled', error_code=$2, error_message=$3,
    finished_at=now(), updated_at=now(), version=version+1
WHERE definition_id=$1 AND status='scheduled'`, definitionID, code, message)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CancelPendingRunsForDefinition(ctx context.Context, q Querier, definitionID, code, message string) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE runs SET status='cancelled', error_code=$2, error_message=$3,
    finished_at=now(), updated_at=now(), version=version+1
WHERE definition_id=$1 AND status IN ('scheduled','queued','retry_wait')`, definitionID, code, message)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) PauseActiveSchedulesForDefinition(ctx context.Context, q Querier, definitionID string) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE schedules SET status='paused', next_fire_at=NULL, updated_at=now(), version=version+1
WHERE definition_id=$1 AND status='active'`, definitionID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CancelSchedulesForDefinition(ctx context.Context, q Querier, definitionID string) (int, error) {
	tag, err := q.Exec(ctx, `
UPDATE schedules SET status='cancelled', next_fire_at=NULL, updated_at=now(), version=version+1
WHERE definition_id=$1`, definitionID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) ScheduleListForDefinition(ctx context.Context, q Querier, definitionID string) ([]Schedule, error) {
	return Many[Schedule](q.Query(ctx, `SELECT * FROM schedules WHERE definition_id=$1`, definitionID))
}

func (s *Store) CountRunningRunsForDefinition(ctx context.Context, q Querier, definitionID string) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM runs WHERE definition_id=$1 AND status='running'`, definitionID).Scan(&n)
	return n, err
}

func (s *Store) FindActiveRunForTarget(ctx context.Context, q Querier, key, value string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (
        SELECT 1 FROM runs
        WHERE status IN ('queued','running','retry_wait')
          AND target_snapshot->>$1 = $2
    )`
	err := q.QueryRow(ctx, query, key, value).Scan(&exists)
	return exists, err
}

func (s *Store) FindVersionReferencingTarget(ctx context.Context, q Querier, targetID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
SELECT EXISTS (SELECT 1 FROM task_definition_versions WHERE executor_config->>'target_id'=$1)`,
		targetID).Scan(&exists)
	return exists, err
}

func (s *Store) FindTargetUsingCredential(ctx context.Context, q Querier, credentialID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM execution_targets WHERE credential_id=$1)`, credentialID).Scan(&exists)
	return exists, err
}

func (s *Store) FailQueuedRunsForTarget(ctx context.Context, q Querier, targetID, credentialID string) (int, error) {
	query := `UPDATE runs SET status='failed', error_code='TARGET_DISABLED',
        error_message='execution target or credential disabled',
        finished_at=now(), updated_at=now(), version=version+1
        WHERE status IN ('queued','retry_wait')`
	args := []any{}
	if targetID != "" {
		args = append(args, targetID)
		query += ` AND target_snapshot->>'target_id'=$` + itoa(len(args))
	}
	if credentialID != "" {
		args = append(args, credentialID)
		query += ` AND target_snapshot->>'credential_id'=$` + itoa(len(args))
	}
	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) GetScheduleForUpdate(ctx context.Context, q Querier, id string) (*Schedule, error) {
	return One[Schedule](q.QueryRow(ctx, `SELECT * FROM schedules WHERE id=$1 FOR UPDATE`, id))
}
