package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withCapturedLogs temporarily replaces slog's default logger with one
// writing JSON into buf, restoring the previous default when the test
// ends — the standard pattern for asserting on slog output without a
// package-level logger dependency to inject. Level is set to Debug
// explicitly: RequestLogger logs a normal 2xx/3xx at Debug (verbose-in-
// dev, quiet-in-prod per ConfigureDefaultLogger), and a handler with no
// options defaults to Info, which would silently swallow exactly the
// success-case assertions below.
func withCapturedLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func TestRequestLoggerLogsMethodPathStatusAndDuration(t *testing.T) {
	buf := withCapturedLogs(t)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/workitems", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var logged map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logged); err != nil {
		t.Fatalf("expected a valid JSON log line, got error: %v (raw: %s)", err, buf.String())
	}

	if logged["method"] != http.MethodPost {
		t.Fatalf("expected method %q, got %v", http.MethodPost, logged["method"])
	}
	if logged["path"] != "/workitems" {
		t.Fatalf("expected path %q, got %v", "/workitems", logged["path"])
	}
	if logged["status"] != float64(http.StatusCreated) {
		t.Fatalf("expected status %v, got %v", http.StatusCreated, logged["status"])
	}
	if _, ok := logged["duration_ms"]; !ok {
		t.Fatal("expected a duration_ms field in the log line")
	}
}

func TestRequestLoggerDefaultsToStatus200WhenWriteHeaderNeverCalled(t *testing.T) {
	buf := withCapturedLogs(t)

	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Never calls WriteHeader — net/http implicitly sends 200 on the
		// first Write, so the logged status must match that, not some
		// unset zero value.
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), `"status":200`) {
		t.Fatalf("expected status 200 in the log line, got: %s", buf.String())
	}
}

func TestRequestLoggerLogsEvenAnUnmatchedRoute(t *testing.T) {
	buf := withCapturedLogs(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLogger(mux)

	req := httptest.NewRequest(http.MethodGet, "/this-route-does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	if !strings.Contains(buf.String(), `"status":404`) {
		t.Fatalf("expected the 404 to be logged too, got: %s", buf.String())
	}
}

func TestRequestLoggerLevelsByStatusCode(t *testing.T) {
	cases := []struct {
		status int
		level  string
	}{
		{http.StatusOK, "DEBUG"},
		{http.StatusCreated, "DEBUG"},
		{http.StatusNotFound, "WARN"},
		{http.StatusForbidden, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
	}

	for _, tc := range cases {
		buf := withCapturedLogs(t)

		handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))

		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		var logged map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logged); err != nil {
			t.Fatalf("status %d: expected a valid JSON log line, got error: %v", tc.status, err)
		}

		if logged["level"] != tc.level {
			t.Fatalf("status %d: expected log level %q, got %v", tc.status, tc.level, logged["level"])
		}
	}
}

func TestConfigureDefaultLoggerDevelopmentIsVerboseTextAtDebug(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	// ConfigureDefaultLogger writes to os.Stdout directly, so (same as
	// the production test below) this inspects the installed handler
	// directly rather than trying to capture its output.
	ConfigureDefaultLogger("development")

	handler, ok := slog.Default().Handler().(*slog.TextHandler)
	if !ok {
		t.Fatalf("expected development to install a *slog.TextHandler, got %T", slog.Default().Handler())
	}

	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected development's handler to allow Debug-level logs")
	}
}

func TestConfigureDefaultLoggerProductionSuppressesDebugAndIsJSON(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	// ConfigureDefaultLogger writes to os.Stdout directly, so this test
	// can't swap in its own buffer first — it proves the two properties
	// that actually matter instead: Debug-level output is suppressed
	// (verified against a handler built the same way, same Level), and
	// production's own handler is JSON-shaped (type-asserted directly,
	// since output capture isn't available here).
	ConfigureDefaultLogger("production")

	handler, ok := slog.Default().Handler().(*slog.JSONHandler)
	if !ok {
		t.Fatalf("expected production to install a *slog.JSONHandler, got %T", slog.Default().Handler())
	}

	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected production's handler to suppress Debug-level logs")
	}
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected production's handler to allow Info-level logs")
	}
}
