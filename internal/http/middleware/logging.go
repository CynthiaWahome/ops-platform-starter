package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

// statusRecorder wraps a ResponseWriter to capture the status code a
// handler actually wrote — net/http gives no way to read that back
// otherwise. Defaults to 200: if a handler never calls WriteHeader
// explicitly (writing directly via Write), net/http implicitly sends 200,
// so that's the correct default here too, not an unknown/zero value.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// ConfigureDefaultLogger sets slog's process-wide default logger based on
// appEnv — the lever cfg.AppEnv (APP_ENV) already provides, so no new env
// var is needed to control verbosity. "development" (the zero-setup
// default) gets human-readable text at Debug level: every request,
// success included. Anything else (production) gets structured JSON at
// Info level — RequestLogger logs successful requests at Debug, so they
// never clear that threshold in production, while 4xx/5xx responses log
// at Warn/Error and always do. Call once at startup, before anything else
// logs.
func ConfigureDefaultLogger(appEnv string) {
	var handler slog.Handler
	if appEnv == "development" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}

	slog.SetDefault(slog.New(handler))
}

// RequestLogger logs method, path, status code, and duration for every
// request (issue #72) — found missing during #68b's live manual testing,
// when a failed signup produced nothing in the server's own output beyond
// the startup line. Wraps the whole mux in router.New, not individual
// routes, so even a request that hits no route at all (net/http's default
// 404) still gets logged — exactly the case that's most useful to see.
//
// Logged at a level that varies with the response: Debug for a normal
// 2xx/3xx (verbose in development, suppressed in production by
// ConfigureDefaultLogger's Info threshold there), Warn for 4xx, Error for
// 5xx — the failures worth seeing regardless of environment.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		level := slog.LevelDebug
		switch {
		case recorder.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case recorder.status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}

		slog.Log(r.Context(), level, "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
