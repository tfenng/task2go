package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body LoginRequest
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.Username) < 3 || len(body.Username) > 64 || len(body.Password) < 1 || len(body.Password) > 256 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	ip, ua := clientMeta(r)
	if !s.Limiter.Allow("login:"+ip+":"+strings.ToLower(body.Username), 10, time.Minute) {
		writeError(w, r, &apiError{Status: 429, Code: "RATE_LIMITED", Message: "too many login attempts", RetryAfter: 60})
		return
	}
	ctx := r.Context()
	var sessionTokens struct {
		session string
		csrf    string
		ttl     time.Duration
	}
	var loggedIn *store.User
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		user, err := s.Store.GetUserByUsernameCI(ctx, tx, body.Username)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if user != nil && user.LockedUntil != nil && user.LockedUntil.After(now) {
			return &apiError{Status: 423, Code: "AUTH_LOCKED", Message: "account is locked"}
		}
		if user == nil || !user.IsActive || !security.VerifyPassword(body.Password, user.PasswordHash) {
			if user != nil {
				user.FailedLoginCount++
				if user.FailedLoginCount >= 5 {
					locked := now.Add(15 * time.Minute)
					user.LockedUntil = &locked
				}
				if err := s.Store.SaveUser(ctx, tx, user); err != nil {
					return err
				}
				event := &store.AuditEvent{
					ActorUserID: &user.ID, Action: "auth.login", ResourceType: "user",
					ResourceID: &user.ID, Outcome: "failure", CorrelationID: correlationID(r),
					IPAddress: optionalPtr(ip), UserAgent: optionalPtr(ua),
				}
				if err := s.Store.InsertAudit(ctx, tx, event); err != nil {
					return err
				}
			}
			return invalidCredentials("invalid username or password")
		}
		user.FailedLoginCount = 0
		user.LockedUntil = nil
		user.LastLoginAt = &now
		if err := s.Store.SaveUser(ctx, tx, user); err != nil {
			return err
		}
		sessionToken := security.RandomToken(48)
		csrf := security.RandomToken(32)
		ttl := security.SessionTTL(body.Remember, s.Config.SessionIdleMinutes, s.Config.SessionRememberDays)
		session := &store.Session{
			ID: store.NewID(), UserID: user.ID, TokenHash: security.TokenHash(sessionToken),
			CSRFTokenHash: security.TokenHash(csrf), IPAddress: optionalPtr(ip),
			UserAgent: optionalPtr(ua), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(ttl),
		}
		if err := s.Store.CreateSession(ctx, tx, session); err != nil {
			return err
		}
		s.audit(ctx, tx, user, "auth.login", "user", user.ID, nil, r)
		sessionTokens.session = sessionToken
		sessionTokens.csrf = csrf
		sessionTokens.ttl = ttl
		loggedIn = user
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	s.setCookies(w, sessionTokens.session, sessionTokens.csrf, sessionTokens.ttl)
	writeJSON(w, 200, LoginResponse{User: userOut(loggedIn), CSRFToken: sessionTokens.csrf})
}

func (s *Server) setCookies(w http.ResponseWriter, sessionToken, csrf string, ttl time.Duration) {
	maxAge := int(ttl.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name: s.Config.SessionCookieName, Value: sessionToken, Path: "/",
		HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name: s.Config.CSRFCookieName, Value: csrf, Path: "/",
		HttpOnly: false, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		MaxAge: maxAge,
	})
}

func (s *Server) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{s.Config.SessionCookieName, s.Config.CSRFCookieName} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if auth.Session != nil {
			if err := s.Store.RevokeSession(ctx, tx, auth.Session.ID); err != nil {
				return err
			}
		}
		resourceID := ""
		if auth.Session != nil {
			resourceID = auth.Session.ID
		}
		s.audit(ctx, tx, auth.User, "auth.logout", "session", resourceID, nil, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	s.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireAuth(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	writeJSON(w, 200, userOut(auth.User))
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body PasswordChange
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.NewPassword) < 6 || len(body.NewPassword) > 256 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	if !security.VerifyPassword(body.CurrentPassword, auth.User.PasswordHash) {
		writeError(w, r, invalidCredentials("current password is incorrect"))
		return
	}
	hash, err := security.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	ctx := r.Context()
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		auth.User.PasswordHash = hash
		auth.User.MustChangePassword = false
		auth.User.Version++
		if err := s.Store.SaveUser(ctx, tx, auth.User); err != nil {
			return err
		}
		keep := ""
		if auth.Session != nil {
			keep = auth.Session.ID
		}
		if err := s.Store.RevokeOtherSessions(ctx, tx, auth.User.ID, keep); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "auth.password_change", "user", auth.User.ID, nil, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireAuth(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	items, err := s.Store.ListSessions(r.Context(), s.Store.Pool, auth.User.ID)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	out := make([]SessionOut, 0, len(items))
	for _, item := range items {
		out = append(out, SessionOut{
			ID: item.ID, IPAddress: item.IPAddress, UserAgent: item.UserAgent,
			CreatedAt: item.CreatedAt, LastSeenAt: item.LastSeenAt, ExpiresAt: item.ExpiresAt,
		})
	}
	writeJSON(w, 200, SessionList{Items: out})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "session_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		session, err := s.Store.GetSession(ctx, tx, id)
		if err != nil {
			return err
		}
		if session == nil || session.UserID != auth.User.ID {
			return notFound("session not found")
		}
		if err := s.Store.RevokeSession(ctx, tx, id); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "auth.session_revoke", "session", id, nil, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireAuth(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	items, err := s.Store.ListAPITokens(r.Context(), s.Store.Pool, auth.User.ID)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	out := make([]APITokenOut, 0, len(items))
	for _, item := range items {
		out = append(out, apiTokenOut(&item))
	}
	writeJSON(w, 200, APITokenList{Items: out})
}

func apiTokenOut(token *store.APIToken) APITokenOut {
	return APITokenOut{
		ID: token.ID, Name: token.Name, TokenPrefix: token.TokenPrefix,
		ExpiresAt: token.ExpiresAt, LastUsedAt: token.LastUsedAt,
		RevokedAt: token.RevokedAt, CreatedAt: token.CreatedAt,
	}
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	var body APITokenCreate
	if err := parseJSON(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	if len(body.Name) < 1 || len(body.Name) > 64 {
		writeError(w, r, validationErr("request validation failed"))
		return
	}
	raw := "tc_" + security.RandomToken(32)
	token := &store.APIToken{
		ID: store.NewID(), UserID: auth.User.ID, Name: body.Name,
		TokenPrefix: raw[:12], TokenHash: security.TokenHash(raw),
		ExpiresAt: body.ExpiresAt, CreatedAt: time.Now().UTC(),
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.Store.CreateAPIToken(ctx, tx, token); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "auth.token_create", "api_token", token.ID,
			map[string]any{"name": body.Name}, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	writeJSON(w, 201, APITokenCreated{APITokenOut: apiTokenOut(token), Token: raw})
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	auth, apiErr := s.requireCSRF(r)
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	id, apiErr := pathID(r, "token_id")
	if apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	ctx := r.Context()
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		token, err := s.Store.GetAPIToken(ctx, tx, id)
		if err != nil {
			return err
		}
		if token == nil || token.UserID != auth.User.ID {
			return notFound("token not found")
		}
		if err := s.Store.RevokeAPIToken(ctx, tx, id); err != nil {
			return err
		}
		s.audit(ctx, tx, auth.User, "auth.token_revoke", "api_token", id, nil, r)
		return nil
	})
	if err != nil {
		writeError(w, r, asAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
