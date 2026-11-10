package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tfenng/tasks/task2go/internal/config"
	"github.com/tfenng/tasks/task2go/internal/security"
	"github.com/tfenng/tasks/task2go/internal/store"
)

type Server struct {
	Config  *config.Config
	Store   *store.Store
	Box     *security.SecretBox
	Limiter *Limiter
	Web     http.FileSystem
	Index   []byte
}

func NewServer(cfg *config.Config, st *store.Store, box *security.SecretBox, webFS http.FileSystem, index []byte) *Server {
	return &Server{
		Config: cfg, Store: st, Box: box, Limiter: NewLimiter(),
		Web: webFS, Index: index,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	var handler http.Handler = mux
	handler = s.recoverer(handler)
	handler = s.cors(handler)
	handler = s.securityHeaders(handler)
	handler = s.correlation(handler)
	return handler
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	mux.HandleFunc("POST /api/v1/auth/password", s.changePassword)
	mux.HandleFunc("GET /api/v1/auth/sessions", s.listSessions)
	mux.HandleFunc("DELETE /api/v1/auth/sessions/{session_id}", s.revokeSession)
	mux.HandleFunc("GET /api/v1/auth/tokens", s.listTokens)
	mux.HandleFunc("POST /api/v1/auth/tokens", s.createToken)
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{token_id}", s.revokeToken)

	mux.HandleFunc("GET /api/v1/task-definitions", s.listDefinitions)
	mux.HandleFunc("POST /api/v1/task-definitions", s.createDefinition)
	mux.HandleFunc("GET /api/v1/task-definitions/{definition_id}", s.getDefinition)
	mux.HandleFunc("PATCH /api/v1/task-definitions/{definition_id}", s.updateDefinition)
	mux.HandleFunc("POST /api/v1/task-definitions/{definition_id}/enable", s.enableDefinition)
	mux.HandleFunc("POST /api/v1/task-definitions/{definition_id}/disable", s.disableDefinition)
	mux.HandleFunc("POST /api/v1/task-definitions/{definition_id}/archive", s.archiveDefinition)
	mux.HandleFunc("DELETE /api/v1/task-definitions/{definition_id}", s.deleteDefinition)

	mux.HandleFunc("GET /api/v1/runs", s.listRuns)
	mux.HandleFunc("POST /api/v1/runs", s.createRun)
	mux.HandleFunc("GET /api/v1/runs/{run_id}", s.getRun)
	mux.HandleFunc("DELETE /api/v1/runs/{run_id}", s.deleteRun)
	mux.HandleFunc("GET /api/v1/runs/{run_id}/live-output", s.getLiveOutput)
	mux.HandleFunc("POST /api/v1/runs/{run_id}/retry", s.retryRun)
	mux.HandleFunc("POST /api/v1/runs/{run_id}/cancel", s.cancelRun)

	mux.HandleFunc("GET /api/v1/schedules", s.listSchedules)
	mux.HandleFunc("POST /api/v1/schedules", s.createSchedule)
	mux.HandleFunc("DELETE /api/v1/schedules", s.deleteAllSchedules)
	mux.HandleFunc("GET /api/v1/schedules/{schedule_id}", s.getSchedule)
	mux.HandleFunc("PATCH /api/v1/schedules/{schedule_id}", s.updateSchedule)
	mux.HandleFunc("DELETE /api/v1/schedules/{schedule_id}", s.deleteSchedule)
	mux.HandleFunc("POST /api/v1/schedules/{schedule_id}/pause", s.pauseSchedule)
	mux.HandleFunc("POST /api/v1/schedules/{schedule_id}/resume", s.resumeSchedule)
	mux.HandleFunc("POST /api/v1/schedules/{schedule_id}/cancel", s.cancelSchedule)

	mux.HandleFunc("GET /api/v1/execution-targets", s.listTargets)
	mux.HandleFunc("POST /api/v1/execution-targets", s.createTarget)
	mux.HandleFunc("PATCH /api/v1/execution-targets/{target_id}", s.updateTarget)
	mux.HandleFunc("DELETE /api/v1/execution-targets/{target_id}", s.deleteTarget)
	mux.HandleFunc("GET /api/v1/credentials", s.listCredentials)
	mux.HandleFunc("POST /api/v1/credentials", s.createCredential)
	mux.HandleFunc("PATCH /api/v1/credentials/{credential_id}", s.updateCredential)
	mux.HandleFunc("DELETE /api/v1/credentials/{credential_id}", s.deleteCredential)

	mux.HandleFunc("GET /api/v1/stats/runs", s.runStats)
	mux.HandleFunc("GET /api/v1/stats/attempts", s.attemptStats)
	mux.HandleFunc("GET /api/v1/users", s.listUsers)
	mux.HandleFunc("POST /api/v1/users", s.createUser)
	mux.HandleFunc("PATCH /api/v1/users/{user_id}", s.updateUser)
	mux.HandleFunc("POST /api/v1/users/{user_id}/reset-password", s.resetUserPassword)
	mux.HandleFunc("GET /api/v1/audit-events", s.listAudit)
	mux.HandleFunc("GET /api/v1/scripts", s.listScripts)
	mux.HandleFunc("GET /api/v1/health/live", s.healthLive)
	mux.HandleFunc("GET /api/v1/health/ready", s.healthReady)

	mux.HandleFunc("GET /api/openapi.json", s.openapiJSON)
	mux.HandleFunc("GET /api/docs", s.apiDocs)
	mux.Handle("GET /assets/", http.FileServer(s.Web))
	mux.HandleFunc("/", s.serveSPA)
}

type contextKey string

const (
	correlationKey contextKey = "correlation_id"
	authKey        contextKey = "auth"
)

func correlationID(r *http.Request) string {
	if value, ok := r.Context().Value(correlationKey).(string); ok {
		return value
	}
	return "-"
}

func (s *Server) correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = randomHex(16)
		}
		w.Header().Set("X-Correlation-ID", id)
		ctx := context.WithValue(r.Context(), correlationKey, id)
		started := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r.WithContext(ctx))
		slog.Info("request completed",
			"correlation_id", id, "method", r.Method, "path", r.URL.Path,
			"status", statusOf(sw), "duration_ms", time.Since(started).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("request panic", "error", recovered, "path", r.URL.Path)
				writeError(w, r, internalError("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		if s.Config.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range s.Config.CORSOriginsList() {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "*")
				w.Header().Set("Access-Control-Allow-Headers", "*")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(path, "api/") || path == "api" {
		writeError(w, r, notFound("not found"))
		return
	}
	if len(s.Index) == 0 {
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.Index)
}

func clientMeta(r *http.Request) (string, string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host, r.Header.Get("User-Agent")
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(buf)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

func statusOf(w http.ResponseWriter) int {
	if sw, ok := w.(*statusWriter); ok && sw.status != 0 {
		return sw.status
	}
	return http.StatusOK
}
