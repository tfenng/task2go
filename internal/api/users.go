package api

import (
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	p, apiErr := parsePage(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	items, total, err := s.Store.ListUsers(r.Context(), s.Store.Pool, p.Limit, p.Offset)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	p.setTotal(total)
	out := make([]UserOut, 0, len(items))
	for i := range items {
		out = append(out, userOut(&items[i]))
	}
	writeJSON(w, 200, UserList{Items: out, Page: p.Page, PageSize: p.PageSize, Total: p.Total, TotalPages: p.TotalPages})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body UserCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.Username) < 3 || len(body.Username) > 64 || !usernamePattern.MatchString(body.Username) {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if len(body.DisplayName) < 1 || len(body.DisplayName) > 128 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if body.Role != "admin" && body.Role != "operator" && body.Role != "viewer" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if len(body.Password) < 6 || len(body.Password) > 256 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	hash, err := security.HashPassword(body.Password)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	now := time.Now().UTC()
	user := &store.User{
		ID: store.NewID(), Username: body.Username, DisplayName: body.DisplayName,
		PasswordHash: hash, Role: body.Role, IsActive: true, MustChangePassword: true,
		CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	ctx := r.Context()
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		existing, err := s.Store.GetUserByUsernameCI(ctx, tx, body.Username)
		if err != nil {
			return err
		}
		if existing != nil {
			return conflict("username already exists")
		}
		if err := s.Store.CreateUser(ctx, tx, user); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "user.create", "user", user.ID,
			map[string]any{"role": user.Role}, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, userOut(user))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "user_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	expected, apiErr := ifMatch(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body UserUpdate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if body.Role != nil && *body.Role != "admin" && *body.Role != "operator" && *body.Role != "viewer" {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	ctx := r.Context()
	var updated *store.User
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		user, err := s.Store.GetUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return notFound("user not found")
		}
		if user.Version != expected {
			return conflict("user version conflict")
		}
		if body.DisplayName != nil {
			if len(*body.DisplayName) < 1 || len(*body.DisplayName) > 128 {
				return validationErr("request validation failed")
			}
			user.DisplayName = *body.DisplayName
		}
		if body.Role != nil {
			user.Role = *body.Role
		}
		if body.IsActive != nil {
			user.IsActive = *body.IsActive
		}
		user.Version++
		if err := s.Store.SaveUser(ctx, tx, user); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "user.update", "user", user.ID, nil, r)
		updated = user
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, userOut(updated))
}

func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r, "admin")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "user_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	temporary := security.RandomToken(12)
	hash, err := security.HashPassword(temporary)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	ctx := r.Context()
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		user, err := s.Store.GetUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return notFound("user not found")
		}
		user.PasswordHash = hash
		user.MustChangePassword = true
		user.FailedLoginCount = 0
		user.LockedUntil = nil
		user.Version++
		if err := s.Store.SaveUser(ctx, tx, user); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "user.reset_password", "user", user.ID, nil, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 200, PasswordResetResponse{TemporaryPassword: temporary})
}
