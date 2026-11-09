package store

import (
	"context"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, q Querier, session *Session) error {
	row := q.QueryRow(ctx, `
INSERT INTO sessions (id, user_id, token_hash, csrf_token_hash, ip_address, user_agent,
    created_at, last_seen_at, expires_at, revoked_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *`,
		session.ID, session.UserID, session.TokenHash, session.CSRFTokenHash,
		session.IPAddress, session.UserAgent, session.CreatedAt, session.LastSeenAt,
		session.ExpiresAt, session.RevokedAt)
	return scanInto(row, session)
}

func (s *Store) GetSessionByHash(ctx context.Context, q Querier, hash []byte) (*Session, error) {
	return One[Session](q.QueryRow(ctx, `SELECT * FROM sessions WHERE token_hash=$1`, hash))
}

func (s *Store) GetSession(ctx context.Context, q Querier, id string) (*Session, error) {
	return One[Session](q.QueryRow(ctx, `SELECT * FROM sessions WHERE id=$1`, id))
}

func (s *Store) ListSessions(ctx context.Context, q Querier, userID string) ([]Session, error) {
	return Many[Session](q.Query(ctx, `
SELECT * FROM sessions WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`, userID))
}

func (s *Store) RevokeSession(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE id=$1`, id, time.Now().UTC())
	return err
}

func (s *Store) RevokeOtherSessions(ctx context.Context, q Querier, userID, keepID string) error {
	_, err := q.Exec(ctx, `
UPDATE sessions SET revoked_at=$3
WHERE user_id=$1 AND revoked_at IS NULL AND id <> $2`, userID, keepID, time.Now().UTC())
	return err
}

func (s *Store) TouchSession(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE sessions SET last_seen_at=$2 WHERE id=$1`, id, time.Now().UTC())
	return err
}

func (s *Store) CreateAPIToken(ctx context.Context, q Querier, token *APIToken) error {
	row := q.QueryRow(ctx, `
INSERT INTO api_tokens (id, user_id, name, token_prefix, token_hash, expires_at,
    last_used_at, revoked_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING *`,
		token.ID, token.UserID, token.Name, token.TokenPrefix, token.TokenHash,
		token.ExpiresAt, token.LastUsedAt, token.RevokedAt, token.CreatedAt)
	return scanInto(row, token)
}

func (s *Store) GetAPITokenByHash(ctx context.Context, q Querier, hash []byte) (*APIToken, error) {
	return One[APIToken](q.QueryRow(ctx, `SELECT * FROM api_tokens WHERE token_hash=$1`, hash))
}

func (s *Store) GetAPIToken(ctx context.Context, q Querier, id string) (*APIToken, error) {
	return One[APIToken](q.QueryRow(ctx, `SELECT * FROM api_tokens WHERE id=$1`, id))
}

func (s *Store) ListAPITokens(ctx context.Context, q Querier, userID string) ([]APIToken, error) {
	return Many[APIToken](q.Query(ctx, `
SELECT * FROM api_tokens WHERE user_id=$1 ORDER BY created_at DESC`, userID))
}

func (s *Store) RevokeAPIToken(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE api_tokens SET revoked_at=$2 WHERE id=$1`, id, time.Now().UTC())
	return err
}

func (s *Store) TouchAPIToken(ctx context.Context, q Querier, id string) error {
	_, err := q.Exec(ctx, `UPDATE api_tokens SET last_used_at=$2 WHERE id=$1`, id, time.Now().UTC())
	return err
}

func (s *Store) InsertAudit(ctx context.Context, q Querier, event *AuditEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.Outcome == "" {
		event.Outcome = "success"
	}
	if event.Details == nil {
		event.Details = JSONB{}
	}
	return q.QueryRow(ctx, `
INSERT INTO audit_events (actor_user_id, action, resource_type, resource_id, outcome,
    ip_address, user_agent, correlation_id, details, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id`,
		event.ActorUserID, event.Action, event.ResourceType, event.ResourceID,
		event.Outcome, event.IPAddress, event.UserAgent, event.CorrelationID,
		event.Details, event.CreatedAt).Scan(&event.ID)
}

func (s *Store) ListAudit(ctx context.Context, q Querier, limit, offset int, action, resourceType *string, from, to *time.Time) ([]AuditEvent, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if action != nil {
		args = append(args, *action)
		where += " AND action=$" + itoa(len(args))
	}
	if resourceType != nil {
		args = append(args, *resourceType)
		where += " AND resource_type=$" + itoa(len(args))
	}
	if from != nil {
		args = append(args, *from)
		where += " AND created_at >= $" + itoa(len(args))
	}
	if to != nil {
		args = append(args, *to)
		where += " AND created_at <= $" + itoa(len(args))
	}
	var total int
	if err := q.QueryRow(ctx, "SELECT count(*) FROM audit_events"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	query := `SELECT * FROM audit_events` + where +
		` ORDER BY created_at DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	items, err := Many[AuditEvent](q.Query(ctx, query, args...))
	return items, total, err
}

func (s *Store) UsernamesByIDs(ctx context.Context, q Querier, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id, username FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}
