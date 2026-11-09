package api

import (
	"net/http"
	"time"
)

func (s *Server) healthLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, HealthResponse{Status: "ok", Checks: map[string]map[string]string{}})
}

func (s *Server) healthReady(w http.ResponseWriter, r *http.Request) {
	checks := map[string]map[string]string{}
	status := "ok"
	if err := s.Store.Pool.Ping(r.Context()); err != nil {
		checks["database"] = map[string]string{"status": "unavailable"}
		status = "unavailable"
	} else {
		checks["database"] = map[string]string{"status": "ok"}
	}
	recent, err := s.Store.HasRecentWorker(r.Context(), s.Store.Pool, time.Now().UTC().Add(-90*time.Second))
	if err != nil {
		checks["worker"] = map[string]string{"status": "degraded"}
	} else if recent {
		checks["worker"] = map[string]string{"status": "ok"}
	} else {
		checks["worker"] = map[string]string{"status": "degraded"}
	}
	writeJSON(w, 200, HealthResponse{Status: status, Checks: checks})
}
