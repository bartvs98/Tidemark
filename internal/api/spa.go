package api

import (
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// hashedAsset matches Angular's content-hashed output (main-JEKI5GUH.js,
// media/geist-latin-wght-normal-XEQ56JUF.woff2); those files never change.
var hashedAsset = regexp.MustCompile(`-[A-Z0-9]{8}\.[a-z0-9]+$`)

const notBuilt = `<!doctype html><title>Tidemark</title>
<p>The frontend isn't built into this binary. Run <code>npm run build</code> in <code>web/</code> and rebuild,
or use <code>npm start</code> there for the dev server (it proxies <code>/api</code> to this process).</p>`

// spaHandler serves static files from spa and falls back to index.html for
// client-side routes, so deep links like /containers/foo work on reload.
func spaHandler(spa fs.FS) http.Handler {
	files := http.FileServerFS(spa)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(spa, name); err == nil && !info.IsDir() {
				if hashedAsset.MatchString(path.Base(name)) {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			if path.Ext(name) != "" {
				http.NotFound(w, r) // a missing asset, not a client route
				return
			}
		}

		index, err := fs.ReadFile(spa, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notBuilt))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
