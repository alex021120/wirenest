package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the built frontend. The `all:` prefix includes files that would
// otherwise be ignored (e.g. dotfiles). The frontend build (vite) outputs here.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the embedded single-page app. Unknown, non-API paths fall back
// to index.html so client-side routing (vue-router history mode) works.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			// Not a real asset: serve the SPA shell so the router can take over.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
