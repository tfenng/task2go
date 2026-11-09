package store

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
)

func One[T any](row pgx.Row) (*T, error) {
	var value T
	if err := scanRow(row, &value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	normalizeTimes(&value)
	return &value, nil
}

func normalizeTimes(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return
	}
	timeType := reflect.TypeOf(time.Time{})
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !field.CanSet() {
			continue
		}
		switch {
		case field.Type() == timeType:
			field.Set(reflect.ValueOf(field.Interface().(time.Time).UTC()))
		case field.Kind() == reflect.Pointer && field.Type().Elem() == timeType && !field.IsNil():
			t := field.Interface().(*time.Time)
			utc := t.UTC()
			field.Set(reflect.ValueOf(&utc))
		}
	}
}

func scanRow(row pgx.Row, target any) error {
	value := reflect.ValueOf(target).Elem()
	typ := value.Type()
	dest := make([]any, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		dest = append(dest, value.Field(i).Addr().Interface())
	}
	return row.Scan(dest...)
}

func Many[T any](rows pgx.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, err
	}
	for i := range items {
		normalizeTimes(&items[i])
	}
	return items, nil
}

func (s *Store) GetUser(ctx context.Context, q Querier, id string) (*User, error) {
	return One[User](q.QueryRow(ctx, `SELECT * FROM users WHERE id=$1`, id))
}

func (s *Store) GetUserByUsernameCI(ctx context.Context, q Querier, username string) (*User, error) {
	return One[User](q.QueryRow(ctx, `SELECT * FROM users WHERE lower(username)=lower($1)`, username))
}

func (s *Store) ListUsers(ctx context.Context, q Querier, limit, offset int) ([]User, int, error) {
	var total int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	items, err := Many[User](q.Query(ctx, `SELECT * FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset))
	return items, total, err
}

func (s *Store) CreateUser(ctx context.Context, q Querier, u *User) error {
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = now
	}
	if u.Version == 0 {
		u.Version = 1
	}
	row := q.QueryRow(ctx, `
INSERT INTO users (id, username, display_name, password_hash, role, is_active,
    must_change_password, failed_login_count, locked_until, last_login_at,
    created_at, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING *`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.Role, u.IsActive,
		u.MustChangePassword, u.FailedLoginCount, u.LockedUntil, u.LastLoginAt,
		u.CreatedAt, u.UpdatedAt, u.Version)
	return scanInto(row, u)
}

func (s *Store) SaveUser(ctx context.Context, q Querier, u *User) error {
	u.UpdatedAt = time.Now().UTC()
	row := q.QueryRow(ctx, `
UPDATE users SET username=$2, display_name=$3, password_hash=$4, role=$5,
    is_active=$6, must_change_password=$7, failed_login_count=$8,
    locked_until=$9, last_login_at=$10, updated_at=$11, version=$12
WHERE id=$1
RETURNING *`,
		u.ID, u.Username, u.DisplayName, u.PasswordHash, u.Role, u.IsActive,
		u.MustChangePassword, u.FailedLoginCount, u.LockedUntil, u.LastLoginAt,
		u.UpdatedAt, u.Version)
	return scanInto(row, u)
}

func scanInto[T any](row pgx.Row, target *T) error {
	if err := scanRow(row, target); err != nil {
		return err
	}
	normalizeTimes(target)
	return nil
}
