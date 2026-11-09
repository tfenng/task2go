package store

import (
	"context"
	"time"
)

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (s *Store) GetDefinition(ctx context.Context, q Querier, id string) (*Definition, error) {
	return One[Definition](q.QueryRow(ctx, `SELECT * FROM task_definitions WHERE id=$1`, id))
}

func (s *Store) GetDefinitionByKey(ctx context.Context, q Querier, key string) (*Definition, error) {
	return One[Definition](q.QueryRow(ctx, `SELECT * FROM task_definitions WHERE key=$1`, key))
}

func (s *Store) ListDefinitions(ctx context.Context, q Querier, limit, offset int, search *string, archived *bool) ([]Definition, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if search != nil && *search != "" {
		args = append(args, "%"+*search+"%")
		n := itoa(len(args))
		where += " AND (name ILIKE $" + n + " OR key ILIKE $" + n + ")"
	}
	if archived != nil {
		args = append(args, *archived)
		where += " AND is_archived=$" + itoa(len(args))
	}
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM task_definitions"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	query := `SELECT * FROM task_definitions` + where +
		` ORDER BY updated_at DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	items, err := Many[Definition](q.Query(ctx, query, args...))
	return items, total, err
}

func (s *Store) CreateDefinition(ctx context.Context, q Querier, d *Definition) error {
	row := q.QueryRow(ctx, `
INSERT INTO task_definitions (id, key, name, description, active_version_id,
    is_archived, archived_at, created_by, created_at, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *`,
		d.ID, d.Key, d.Name, d.Description, d.ActiveVersionID, d.IsArchived,
		d.ArchivedAt, d.CreatedBy, d.CreatedAt, d.UpdatedAt, d.Version)
	return scanInto(row, d)
}

func (s *Store) SaveDefinition(ctx context.Context, q Querier, d *Definition) error {
	d.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE task_definitions SET name=$2, description=$3, active_version_id=$4,
    is_archived=$5, archived_at=$6, updated_at=$7, version=$8
WHERE id=$1
RETURNING *`,
		d.ID, d.Name, d.Description, d.ActiveVersionID, d.IsArchived,
		d.ArchivedAt, d.UpdatedAt, d.Version)
	return scanInto(row, d)
}

func (s *Store) GetVersion(ctx context.Context, q Querier, id string) (*DefinitionVersion, error) {
	return One[DefinitionVersion](q.QueryRow(ctx, `SELECT * FROM task_definition_versions WHERE id=$1`, id))
}

func (s *Store) CreateVersion(ctx context.Context, q Querier, v *DefinitionVersion) error {
	row := q.QueryRow(ctx, `
INSERT INTO task_definition_versions (id, definition_id, version_no, executor_type,
    parameter_schema, executor_config, timeout_seconds, retry_policy, max_concurrency,
    checksum, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING *`,
		v.ID, v.DefinitionID, v.VersionNo, v.ExecutorType, v.ParameterSchema,
		v.ExecutorConfig, v.TimeoutSeconds, v.RetryPolicy, v.MaxConcurrency,
		v.Checksum, v.CreatedBy, v.CreatedAt)
	return scanInto(row, v)
}

func (s *Store) MaxVersionNo(ctx context.Context, q Querier, definitionID string) (int, error) {
	var n *int
	if err := q.QueryRow(ctx, `SELECT max(version_no) FROM task_definition_versions WHERE definition_id=$1`, definitionID).Scan(&n); err != nil {
		return 0, err
	}
	if n == nil {
		return 0, nil
	}
	return *n, nil
}

func (s *Store) ActiveVersion(ctx context.Context, q Querier, d *Definition, versionID *string) (*DefinitionVersion, error) {
	if versionID != nil {
		version, err := s.GetVersion(ctx, q, *versionID)
		if err != nil || version == nil {
			return nil, err
		}
		if version.DefinitionID != d.ID {
			return nil, nil
		}
		return version, nil
	}
	if d.ActiveVersionID == nil {
		return nil, nil
	}
	version, err := s.GetVersion(ctx, q, *d.ActiveVersionID)
	if err != nil || version == nil {
		return nil, err
	}
	if version.DefinitionID != d.ID {
		return nil, nil
	}
	return version, nil
}

func (s *Store) GetRuntime(ctx context.Context, q Querier, definitionID string) (*DefinitionRuntime, error) {
	return One[DefinitionRuntime](q.QueryRow(ctx, `SELECT * FROM task_definition_runtime WHERE definition_id=$1`, definitionID))
}

func (s *Store) CreateRuntime(ctx context.Context, q Querier, r *DefinitionRuntime) error {
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now().UTC()
	}
	row := q.QueryRow(ctx, `
INSERT INTO task_definition_runtime (definition_id, running_count, max_concurrency, updated_at)
VALUES ($1,$2,$3,$4) RETURNING *`,
		r.DefinitionID, r.RunningCount, r.MaxConcurrency, r.UpdatedAt)
	return scanInto(row, r)
}

func (s *Store) SaveRuntime(ctx context.Context, q Querier, r *DefinitionRuntime) error {
	r.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE task_definition_runtime SET running_count=$2, max_concurrency=$3, updated_at=$4
WHERE definition_id=$1 RETURNING *`,
		r.DefinitionID, r.RunningCount, r.MaxConcurrency, r.UpdatedAt)
	return scanInto(row, r)
}

func (s *Store) IncrementRuntime(ctx context.Context, q Querier, definitionID string) (bool, error) {
	tag, err := q.Exec(ctx, `
UPDATE task_definition_runtime SET running_count=running_count+1, updated_at=now()
WHERE definition_id=$1 AND running_count < max_concurrency`, definitionID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) DecrementRuntime(ctx context.Context, q Querier, definitionID string) error {
	_, err := q.Exec(ctx, `
UPDATE task_definition_runtime
SET running_count=GREATEST(running_count-1, 0), updated_at=now()
WHERE definition_id=$1`, definitionID)
	return err
}
