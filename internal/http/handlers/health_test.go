package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CynthiaWahome/ops-platform-starter/internal/config"
)

func TestHealthHandlerReturnsOK(t *testing.T) {
	handler := NewHealthHandler(config.Config{
		Port:   "8080",
		AppEnv: "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content type application/json, got %s", got)
	}
}
