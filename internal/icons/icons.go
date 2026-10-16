// Package icons matches containers to app icons from the selfh.st collection
// (https://selfh.st/icons, CC BY 4.0) and serves them from a local disk cache,
// so browsers never contact the CDN and icons keep working offline once fetched.
package icons

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL = "https://cdn.jsdelivr.net/gh/selfhst/icons"
	LabelIcon      = "tidemark.icon" // container label: an icon reference, or "none"

	indexFile    = "index.json"
	indexMaxAge  = 24 * time.Hour
	maxIndexSize = 16 << 20
	maxIconSize  = 2 << 20
)

// Icon is one entry of the selfh.st index.
type Icon struct {
	Ref   string // reference/slug, e.g. "home-assistant"
	Ext   string // "svg" when available, otherwise "png"
	Light bool   // a light (white) variant exists, for dark backgrounds
	Dark  bool   // a dark (black) variant exists, for light backgrounds
}

// Variants of an icon file.
const (
	VariantDefault = ""
	VariantLight   = "light"
	VariantDark    = "dark"
)

type indexEntry struct {
	Name      string `json:"Name"`
	Reference string `json:"Reference"`
	SVG       string `json:"SVG"`
	PNG       string `json:"PNG"`
	Light     string `json:"Light"`
	Dark      string `json:"Dark"`
}

type Service struct {
	base string
	dir  string
	log  *slog.Logger
	http *http.Client

	mu     sync.RWMutex
	byRef  map[string]Icon
	byKey  map[string]string // normalized name or reference -> reference
	sorted []string          // references, sorted, for prefix lookups

	fetchMu  sync.Mutex
	fetching map[string]*sync.Mutex // per icon file, so different icons download in parallel
}

func New(baseURL, cacheDir string, log *slog.Logger) *Service {
	return &Service{
		base: strings.TrimRight(baseURL, "/"),
		dir:  cacheDir,
		log:  log,
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// Start loads the cached index immediately, then refreshes it from the CDN in the
// background when stale and once a day after that.
func (s *Service) Start(ctx context.Context) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		s.log.Warn("icons: can't create cache dir", "dir", s.dir, "err", err)
	}
	fresh := false
	if data, info, err := s.readCachedIndex(); err == nil {
		if err := s.load(data); err == nil {
			fresh = time.Since(info.ModTime()) < indexMaxAge
		}
	}
	go func() {
		if !fresh {
			s.refresh(ctx)
		}
		t := time.NewTicker(indexMaxAge)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.refresh(ctx)
			}
		}
	}()
}

func (s *Service) refresh(ctx context.Context) {
	data, err := s.download(ctx, s.base+"/"+indexFile, maxIndexSize)
	if err != nil {
		s.log.Warn("icons: index download failed, using cache if any", "err", err)
		return
	}
	if err := s.load(data); err != nil {
		s.log.Warn("icons: bad index", "err", err)
		return
	}
	if err := writeFileAtomic(filepath.Join(s.dir, indexFile), data); err != nil {
		s.log.Warn("icons: can't cache index", "err", err)
	}
	s.mu.RLock()
	n := len(s.byRef)
	s.mu.RUnlock()
	s.log.Info("icons: index loaded", "icons", n)
}

func (s *Service) readCachedIndex() ([]byte, os.FileInfo, error) {
	path := filepath.Join(s.dir, indexFile)
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	return data, info, err
}

func (s *Service) load(data []byte) error {
	var entries []indexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	byRef := make(map[string]Icon, len(entries))
	byKey := make(map[string]string, len(entries)*2)
	sorted := make([]string, 0, len(entries))
	for _, e := range entries {
		ext := ""
		switch {
		case strings.EqualFold(e.SVG, "yes"):
			ext = "svg"
		case strings.EqualFold(e.PNG, "yes"):
			ext = "png"
		}
		if e.Reference == "" || ext == "" || !validRef(e.Reference) {
			continue
		}
		byRef[e.Reference] = Icon{
			Ref:   e.Reference,
			Ext:   ext,
			Light: strings.EqualFold(e.Light, "yes"),
			Dark:  strings.EqualFold(e.Dark, "yes"),
		}
		sorted = append(sorted, e.Reference)
		for _, k := range []string{normalize(e.Reference), normalize(e.Name)} {
			if _, taken := byKey[k]; !taken && k != "" {
				byKey[k] = e.Reference
			}
		}
	}
	if len(byRef) == 0 {
		return fmt.Errorf("index has no usable icons")
	}
	sort.Strings(sorted)

	s.mu.Lock()
	s.byRef, s.byKey, s.sorted = byRef, byKey, sorted
	s.mu.Unlock()
	return nil
}

// Lookup returns the icon with exactly this reference.
func (s *Service) Lookup(ref string) (Icon, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ic, ok := s.byRef[ref]
	return ic, ok
}

// Serve writes the icon (or its light/dark variant), fetching it from the CDN into
// the disk cache on first use.
func (s *Service) Serve(w http.ResponseWriter, r *http.Request, ref, variant string) {
	ic, ok := s.Lookup(ref)
	if !ok || (variant == VariantLight && !ic.Light) || (variant == VariantDark && !ic.Dark) ||
		(variant != VariantDefault && variant != VariantLight && variant != VariantDark) {
		http.NotFound(w, r)
		return
	}
	name := ic.Ref
	if variant != VariantDefault {
		name += "-" + variant
	}
	path := filepath.Join(s.dir, name+"."+ic.Ext)
	data, err := os.ReadFile(path)
	if err != nil {
		data, err = s.fetchIcon(r.Context(), ic.Ext, name, path)
		if err != nil {
			s.log.Debug("icons: fetch failed", "ref", ref, "err", err)
			http.NotFound(w, r)
			return
		}
	}

	h := w.Header()
	if ic.Ext == "svg" {
		h.Set("Content-Type", "image/svg+xml")
	} else {
		h.Set("Content-Type", "image/png")
	}
	// Icons come from a third party. <img> never runs SVG scripts, but opening the URL
	// directly would, on our origin, so lock the response down.
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func (s *Service) fetchIcon(ctx context.Context, ext, name, path string) ([]byte, error) {
	s.fetchMu.Lock()
	if s.fetching == nil {
		s.fetching = map[string]*sync.Mutex{}
	}
	m, ok := s.fetching[path]
	if !ok {
		m = &sync.Mutex{}
		s.fetching[path] = m
	}
	s.fetchMu.Unlock()

	m.Lock()
	defer m.Unlock()
	if data, err := os.ReadFile(path); err == nil {
		return data, nil // another request fetched it meanwhile
	}
	data, err := s.download(ctx, fmt.Sprintf("%s/%s/%s.%s", s.base, ext, name, ext), maxIconSize)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(path, data); err != nil {
		s.log.Warn("icons: can't cache icon", "name", name, "err", err)
	}
	return data, nil
}

func (s *Service) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", url, limit)
	}
	return data, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// validRef keeps references safe to use as file names and URL path segments.
func validRef(ref string) bool {
	for _, r := range ref {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return ref != "" && !strings.HasPrefix(ref, ".")
}
