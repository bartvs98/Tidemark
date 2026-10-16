// Package api serves the JSON API under /api and the Angular SPA everywhere else.
package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
)

type Server struct {
	version string
	log     *slog.Logger
}

func New(version string, log *slog.Logger) *Server {
	return &Server{version: version, log: log}
}

// Handler returns the full HTTP handler: API routes plus the SPA served from spa.
func (s *Server) Handler(spa fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.Handle("/", spaHandler(spa))
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
