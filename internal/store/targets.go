package store

import (
	"context"
	"time"
)

func (s *Store) GetCredential(ctx context.Context, q Querier, id string) (*Credential, error) {
	return One[Credential](q.QueryRow(ctx, `SELECT * FROM credentials WHERE id=$1`, id))
}

func (s *Store) GetCredentialByName(ctx context.Context, q Querier, name string) (*Credential, error) {
	return One[Credential](q.QueryRow(ctx, `SELECT * FROM credentials WHERE name=$1`, name))
}

func (s *Store) ListCredentials(ctx context.Context, q Querier, limit, offset int) ([]Credential, int, error) {
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM credentials`).Scan(&total); err != nil {
		return nil, 0, err
	}
	items, err := Many[Credential](q.Query(ctx, `
SELECT * FROM credentials ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, limit, offset))
	return items, total, err
}

func (s *Store) CreateCredential(ctx context.Context, q Querier, c *Credential) error {
	row := q.QueryRow(ctx, `
INSERT INTO credentials (id, name, kind, ciphertext, key_version, metadata,
    is_active, created_by, created_at, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *`,
		c.ID, c.Name, c.Kind, c.Ciphertext, c.KeyVersion, c.Metadata,
		c.IsActive, c.CreatedBy, c.CreatedAt, c.UpdatedAt, c.Version)
	return scanInto(row, c)
}

func (s *Store) SaveCredential(ctx context.Context, q Querier, c *Credential) error {
	c.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE credentials SET name=$2, kind=$3, ciphertext=$4, key_version=$5, metadata=$6,
    is_active=$7, updated_at=$8, version=$9
WHERE id=$1 RETURNING *`,
		c.ID, c.Name, c.Kind, c.Ciphertext, c.KeyVersion, c.Metadata,
		c.IsActive, c.UpdatedAt, c.Version)
	return scanInto(row, c)
}

func (s *Store) DeleteCredential(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM credentials WHERE id=$1`, id)
	return err
}

func (s *Store) GetTarget(ctx context.Context, q Querier, id string) (*ExecutionTarget, error) {
	return One[ExecutionTarget](q.QueryRow(ctx, `SELECT * FROM execution_targets WHERE id=$1`, id))
}

func (s *Store) ListTargets(ctx context.Context, q Querier, limit, offset int, typeFilter *string) ([]ExecutionTarget, int, error) {
	where := ""
	args := []any{}
	if typeFilter != nil && *typeFilter != "" {
		args = append(args, *typeFilter)
		where = " WHERE type=$1"
	}
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM execution_targets`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	query := `SELECT * FROM execution_targets` + where +
		` ORDER BY updated_at DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	items, err := Many[ExecutionTarget](q.Query(ctx, query, args...))
	return items, total, err
}

func (s *Store) CreateTarget(ctx context.Context, q Querier, t *ExecutionTarget) error {
	row := q.QueryRow(ctx, `
INSERT INTO execution_targets (id, name, type, config, credential_id, is_active,
    created_by, created_at, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *`,
		t.ID, t.Name, t.Type, t.Config, t.CredentialID, t.IsActive,
		t.CreatedBy, t.CreatedAt, t.UpdatedAt, t.Version)
	return scanInto(row, t)
}

func (s *Store) SaveTarget(ctx context.Context, q Querier, t *ExecutionTarget) error {
	t.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE execution_targets SET name=$2, type=$3, config=$4, credential_id=$5,
    is_active=$6, updated_at=$7, version=$8
WHERE id=$1 RETURNING *`,
		t.ID, t.Name, t.Type, t.Config, t.CredentialID, t.IsActive, t.UpdatedAt, t.Version)
	return scanInto(row, t)
}

func (s *Store) DeleteTarget(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM execution_targets WHERE id=$1`, id)
	return err
}
