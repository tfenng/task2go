package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

type AuthContext struct {
	User    *store.User
	Session *store.Session
}

func (s *Server) authenticate(r *http.Request) (*AuthContext, *apiError) {
	ctx := r.Context()
	if header := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(header), "bearer ") {
		raw := strings.TrimSpace(header[len("bearer "):])
		token, err := s.Store.GetAPITokenByHash(ctx, s.Store.Pool, security.TokenHash(raw))
		if err != nil {
			return nil, internalError("internal server error")
		}
		if token != nil && token.RevokedAt == nil && (token.ExpiresAt == nil || token.ExpiresAt.After(time.Now().UTC())) {
			user, err := s.Store.GetUser(ctx, s.Store.Pool, token.UserID)
			if err != nil {
				return nil, internalError("internal server error")
			}
			if user != nil && user.IsActive {
				_ = s.Store.TouchAPIToken(ctx, s.Store.Pool, token.ID)
				return &AuthContext{User: user}, nil
			}
		}
	}
	cookie, err := r.Cookie(s.Config.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, authRequired()
	}
	session, err := s.Store.GetSessionByHash(ctx, s.Store.Pool, security.TokenHash(cookie.Value))
	if err != nil {
		return nil, internalError("internal server error")
	}
	if session == nil || session.RevokedAt != nil {
		return nil, authRequired()
	}
	now := time.Now().UTC()
	if !session.ExpiresAt.After(now) {
		_ = s.Store.RevokeSession(ctx, s.Store.Pool, session.ID)
		return nil, authRequired()
	}
	_ = s.Store.TouchSession(ctx, s.Store.Pool, session.ID)
	user, err := s.Store.GetUser(ctx, s.Store.Pool, session.UserID)
	if err != nil {
		return nil, internalError("internal server error")
	}
	if user == nil || !user.IsActive {
		return nil, authRequired()
	}
	return &AuthContext{User: user, Session: session}, nil
}

func (s *Server) requireAuth(r *http.Request, roles ...string) (*AuthContext, *apiError) {
	ctx, err := s.authenticate(r)
	if err != nil {
		return nil, err
	}
	if len(roles) > 0 {
		allowed := false
		for _, role := range roles {
			if ctx.User.Role == role {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, forbidden("insufficient role")
		}
	}
	return ctx, nil
}

func (s *Server) requireCSRF(r *http.Request, roles ...string) (*AuthContext, *apiError) {
	ctx, err := s.requireAuth(r, roles...)
	if err != nil {
		return nil, err
	}
	if !s.Limiter.Allow("write:"+ctx.User.ID, 120, time.Minute) {
		return nil, &apiError{Status: 429, Code: "RATE_LIMITED", Message: "too many write requests", RetryAfter: 60}
	}
	if ctx.Session == nil {
		return ctx, nil
	}
	token := r.Header.Get("X-CSRF-Token")
	if !security.HashEqual(security.TokenHash(token), ctx.Session.CSRFTokenHash) {
		return nil, forbidden("invalid csrf token")
	}
	return ctx, nil
}

func (s *Server) audit(ctx context.Context, tx pgx.Tx, actor *store.User, action, resourceType, resourceID string, details map[string]any, r *http.Request) {
	if tx == nil {
		return
	}
	var actorID *string
	if actor != nil {
		actorID = &actor.ID
	}
	var resID *string
	if resourceID != "" {
		resID = &resourceID
	}
	ip, ua := clientMeta(r)
	var ipPtr, uaPtr *string
	if ip != "" {
		ipPtr = &ip
	}
	if ua != "" {
		uaPtr = &ua
	}
	event := &store.AuditEvent{
		ActorUserID: actorID, Action: action, ResourceType: resourceType,
		ResourceID: resID, Outcome: "success", IPAddress: ipPtr, UserAgent: uaPtr,
		CorrelationID: correlationID(r), Details: store.JSONB(details),
	}
	_ = s.Store.InsertAudit(ctx, tx, event)
}
