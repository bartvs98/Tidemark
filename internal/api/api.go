// Package api serves the JSON API under /api and the Angular SPA everywhere else.
package api

import (
	"context"
	"encoding/json"
	"hash/maphash"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/icons"
	"github.com/bartvs98/tidemark/internal/notify"
	"github.com/bartvs98/tidemark/internal/settings"
	"github.com/bartvs98/tidemark/internal/store"
)

type Deps struct {
	Version string
	Log     *slog.Logger
	Source  docker.Source
	Icons   *icons.Service // nil when icons are turned off; see Server.SetIcons
	Checks  *check.Service // nil in tests that don't need update checks
	// Changelogs serves release notes on the detail endpoint; nil disables them.
	Changelogs *changelog.Service
	// Store keeps review states and update history; nil disables reviews (the endpoints
	// answer 501) and history (always empty).
	Store *store.Store
	// ConfigFile (rules file path) is only shown in settings.
	ConfigFile string

	// Notify sends update messages. Main always sets it (off until configured); nil in
	// tests means off too (the test endpoint answers 501).
	Notify *notify.Service

	// Settings serves and edits the editable settings; nil leaves settings read-only and
	// without per-field state (tests).
	Settings *settings.Manager

	// AI keeps AI changelog summaries; its verdicts raise update levels while a provider is
	// configured. nil means off (the summary endpoint answers 501).
	AI *ai.Service

	// Context for background work started by requests (check runs); it outlives the request.
	Background context.Context

	// Started is when this process started (uptime on the stats page); zero means New's time.
	Started time.Time
}

type Server struct {
	version    string
	log        *slog.Logger
	source     docker.Source
	icons      atomic.Pointer[icons.Service] // nil when icons are off; swapped live
	checks     *check.Service
	changelogs *changelog.Service
	store      *store.Store
	notify     *notify.Service
	settings   *settings.Manager
	ai         *ai.Service
	configFile string
	background context.Context
	started    time.Time

	// Release levels for the stats, by repo and tag (see releaseLevel).
	levelSeed maphash.Seed
	levelsMu  sync.Mutex
	levels    map[string]cachedLevel
}

func New(d Deps) *Server {
	bg := d.Background
	if bg == nil {
		bg = context.Background()
	}
	started := d.Started
	if started.IsZero() {
		started = time.Now()
	}
	s := &Server{version: d.Version, log: d.Log, source: d.Source, checks: d.Checks, changelogs: d.Changelogs, store: d.Store, notify: d.Notify,
		settings: d.Settings, ai: d.AI, configFile: d.ConfigFile, background: bg, started: started,
		levelSeed: maphash.MakeSeed(), levels: map[string]cachedLevel{}}
	s.icons.Store(d.Icons)
	return s
}

// SetIcons swaps the icon service; nil turns icons off.
func (s *Server) SetIcons(svc *icons.Service) { s.icons.Store(svc) }

// Handler returns the full HTTP handler: API routes plus the SPA served from spa.
func (s *Server) Handler(spa fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/containers", s.containers)
	mux.HandleFunc("GET /api/containers/{name}", s.containerDetail)
	mux.HandleFunc("PUT /api/containers/{name}/review", s.putReview)
	mux.HandleFunc("DELETE /api/containers/{name}/review", s.deleteReview)
	mux.HandleFunc("POST /api/containers/{name}/summary", s.postSummary)
	mux.HandleFunc("GET /api/icons/{ref}", s.icon)
	mux.HandleFunc("POST /api/checks", s.startCheck)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PATCH /api/settings", s.patchSettings)
	mux.HandleFunc("POST /api/notifications/test", s.testNotification)
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	mux.Handle("/", spaHandler(spa))
	return hardened(mux)
}

// hardened adds what every response needs and guards writes. A plain cross-site form post
// needs no CORS preflight, so mutating API calls must carry a custom header that only our
// own SPA (or a script) can set. The framing headers keep other sites from embedding the UI.
func hardened(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if r.Header.Get("X-Tidemark") == "" {
					writeError(w, http.StatusForbidden, "missing X-Tidemark header (cross-site requests are refused)")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) icon(w http.ResponseWriter, r *http.Request) {
	svc := s.icons.Load()
	if svc == nil {
		http.NotFound(w, r)
		return
	}
	svc.Serve(w, r, r.PathValue("ref"), r.URL.Query().Get("variant"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
