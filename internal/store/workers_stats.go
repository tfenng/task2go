package store

import (
	"context"
	"time"
)

func (s *Store) RegisterWorker(ctx context.Context, q Querier, w *Worker) error {
	if w.StartedAt.IsZero() {
		w.StartedAt = time.Now().UTC()
	}
	if w.LastHeartbeatAt.IsZero() {
		w.LastHeartbeatAt = w.StartedAt
	}
	row := q.QueryRow(ctx, `
INSERT INTO workers (id, hostname, pid, role, started_at, last_heartbeat_at, stopped_at)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *`,
		w.ID, w.Hostname, w.PID, w.Role, w.StartedAt, w.LastHeartbeatAt, w.StoppedAt)
	return scanInto(row, w)
}

func (s *Store) HeartbeatWorker(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE workers SET last_heartbeat_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) StopWorker(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE workers SET stopped_at=now(), last_heartbeat_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) HasRecentWorker(ctx context.Context, q Querier, since time.Time) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM workers
    WHERE stopped_at IS NULL AND last_heartbeat_at >= $1
)`, since).Scan(&exists)
	return exists, err
}

func (s *Store) RunsInRange(ctx context.Context, q Querier, from, to time.Time) ([]Run, error) {
	return Many[Run](q.Query(ctx, `
SELECT * FROM runs WHERE created_at >= $1 AND created_at <= $2`, from, to))
}

func (s *Store) AttemptsInRange(ctx context.Context, q Querier, from, to time.Time) ([]Attempt, error) {
	return Many[Attempt](q.Query(ctx, `
SELECT * FROM attempts WHERE created_at >= $1 AND created_at <= $2`, from, to))
}

func (s *Store) VersionsByIDs(ctx context.Context, q Querier, ids []string) (map[string]*DefinitionVersion, error) {
	out := map[string]*DefinitionVersion{}
	if len(ids) == 0 {
		return out, nil
	}
	items, err := Many[DefinitionVersion](q.Query(ctx, `
SELECT * FROM task_definition_versions WHERE id = ANY($1)`, ids))
	if err != nil {
		return nil, err
	}
	for i := range items {
		out[items[i].ID] = &items[i]
	}
	return out, nil
}

func (s *Store) RetentionDelete(ctx context.Context, q Querier, now time.Time, runCutoff, attemptCutoff time.Time) (map[string]int, error) {
	out := map[string]int{}
	tag, err := q.Exec(ctx, `
DELETE FROM attempts WHERE created_at < $1 AND status <> 'running'`, attemptCutoff)
	if err != nil {
		return nil, err
	}
	out["attempts"] = int(tag.RowsAffected())
	tag, err = q.Exec(ctx, `
DELETE FROM runs WHERE created_at < $1
AND status IN ('succeeded','failed','cancelled','skipped','missed')`, runCutoff)
	if err != nil {
		return nil, err
	}
	out["runs"] = int(tag.RowsAffected())
	tag, err = q.Exec(ctx, `DELETE FROM audit_events WHERE created_at < $1`, runCutoff)
	if err != nil {
		return nil, err
	}
	out["audit"] = int(tag.RowsAffected())
	tag, err = q.Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1`, now.AddDate(0, 0, -30))
	if err != nil {
		return nil, err
	}
	out["sessions"] = int(tag.RowsAffected())
	tag, err = q.Exec(ctx, `
DELETE FROM api_tokens WHERE revoked_at IS NOT NULL AND revoked_at < $1`, now.AddDate(0, 0, -90))
	if err != nil {
		return nil, err
	}
	out["api_tokens"] = int(tag.RowsAffected())
	return out, nil
}
