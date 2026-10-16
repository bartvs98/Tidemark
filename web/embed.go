// Package web embeds the compiled Angular app (web/dist/spa) into the Go binary.
// Run `npm run build` in this directory before `go build` to include it;
// without a build the server still starts and explains what's missing.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// SPA returns the built frontend, rooted at the directory containing index.html.
func SPA() fs.FS {
	sub, err := fs.Sub(dist, "dist/spa")
	if err != nil {
		panic(err) // only possible with an invalid path literal
	}
	return sub
}
