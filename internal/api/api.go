// Package api serves the JSON API under /api and the Angular SPA everywhere else.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/icons"
)

type Deps struct {
	Version string
	Log     *slog.Logger
	Source  docker.Source
	Icons   *icons.Service // nil when icons are turned off
	Checks  *check.Service // nil in tests that don't need update checks

	// Context for background work started by requests (check runs); it outlives the request.
	Background context.Context
}

type Server struct {
	version    string
	log        *slog.Logger
	source     docker.Source
	icons      *icons.Service
	checks     *check.Service
	background context.Context
}

func New(d Deps) *Server {
	bg := d.Background
	if bg == nil {
		bg = context.Background()
	}
	return &Server{version: d.Version, log: d.Log, source: d.Source, icons: d.Icons, checks: d.Checks, background: bg}
}

// Handler returns the full HTTP handler: API routes plus the SPA served from spa.
func (s *Server) Handler(spa fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/containers", s.containers)
	mux.HandleFunc("GET /api/icons/{ref}", s.icon)
	mux.HandleFunc("POST /api/checks", s.startCheck)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.Handle("/", spaHandler(spa))
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) icon(w http.ResponseWriter, r *http.Request) {
	if s.icons == nil {
		http.NotFound(w, r)
		return
	}
	s.icons.Serve(w, r, r.PathValue("ref"), r.URL.Query().Get("variant"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
