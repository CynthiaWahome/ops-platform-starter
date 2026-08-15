package middleware

import (
	"context"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
)

func RequireRoles(next http.Handler, allowedRoles ...auth.Role) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
			return
		}

		if !principal.HasAnyRole(allowedRoles...) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "insufficient permissions"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ContextWithPrincipal(ctx context.Context, principal auth.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}
