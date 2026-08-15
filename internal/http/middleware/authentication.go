package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
)

type contextKey string

const principalContextKey contextKey = "principal"

func RequireAuth(service auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := extractBearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
			return
		}

		principal, err := service.Authenticate(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "invalid access token"})
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func PrincipalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(auth.Principal)
	return principal, ok
}

func extractBearerToken(authorizationHeader string) (string, error) {
	headerValue := strings.TrimSpace(authorizationHeader)
	if headerValue == "" {
		return "", errors.New("missing authorization header")
	}

	token, ok := strings.CutPrefix(headerValue, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return "", errors.New("missing bearer token")
	}

	return strings.TrimSpace(token), nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(payload)
}
