// Package docs serves the OpenAPI spec and a Swagger UI to browse it
// (OPS-069) — GET /docs. Both the spec and Swagger UI's static assets
// (bundle JS, standalone preset JS, CSS, favicon) are go:embedded into the
// binary: no external CDN dependency at runtime, the same "minimal
// dependency surface" stance behind net/http-not-Gin and the hand-rolled
// migration runner elsewhere in this codebase.
package docs

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
)

//go:embed static openapi.yaml
var embeddedFiles embed.FS

func init() {
	// .yaml has no MIME type registered in Go's mime package by default
	// on every OS — set it explicitly so the browser's fetch() in
	// static/index.html gets a real Content-Type instead of whatever
	// http.ServeFileFS falls back to guessing.
	_ = mime.AddExtensionType(".yaml", "application/yaml")
}

type Handler struct {
	staticFS http.Handler
}

func NewHandler() Handler {
	static, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		// Only possible if the go:embed directive above is wrong —
		// caught at build/test time, never in production.
		panic(err)
	}

	return Handler{staticFS: http.FileServerFS(static)}
}

// ServeStatic serves index.html and the vendored Swagger UI assets.
func (h Handler) ServeStatic(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/docs/", h.staticFS).ServeHTTP(w, r)
}

// ServeSpec serves the OpenAPI document itself — the one embedded file
// outside static/, since it documents the API, not the UI that renders
// it. static/index.html fetches this at /docs/openapi.yaml.
func (h Handler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFileFS(w, r, embeddedFiles, "openapi.yaml")
}
