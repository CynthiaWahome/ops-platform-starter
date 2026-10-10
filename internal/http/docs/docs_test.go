package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeStaticReturnsIndexHTML(t *testing.T) {
	t.Parallel()

	handler := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	rec := httptest.NewRecorder()
	handler.ServeStatic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Fatalf("expected index.html to reference the swagger-ui mount point, got: %s", rec.Body.String())
	}
}

func TestServeStaticReturnsVendoredAssets(t *testing.T) {
	t.Parallel()

	handler := NewHandler()

	for _, name := range []string{"swagger-ui-bundle.js", "swagger-ui-standalone-preset.js", "swagger-ui.css"} {
		req := httptest.NewRequest(http.MethodGet, "/docs/"+name, nil)
		rec := httptest.NewRecorder()
		handler.ServeStatic(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s to serve with status %d, got %d", name, http.StatusOK, rec.Code)
		}

		if rec.Body.Len() == 0 {
			t.Fatalf("expected %s to have non-empty content", name)
		}
	}
}

func TestServeStaticMissingFileIs404(t *testing.T) {
	t.Parallel()

	handler := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/this-file-does-not-exist.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeStatic(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestServeSpecReturnsYAMLWithCorrectContentType(t *testing.T) {
	t.Parallel()

	handler := NewHandler()

	req := httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	handler.ServeSpec(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Fatalf("expected Content-Type application/yaml, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") {
		t.Fatalf("expected the served spec to be the real OpenAPI document, got: %q", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "/workitems") {
		t.Fatalf("expected the served spec to document the real routes, got: %q", body[:min(200, len(body))])
	}
}
