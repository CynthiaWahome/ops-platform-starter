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

// RequirePasswordChangeCleared enforces OPS-067's "login succeeds but
// every other endpoint 403s until the password is changed" rule for a
// brand-new admin-created account or one that just went through an
// admin-initiated reset. router.go wraps every route with this except the
// two that must stay reachable to clear the flag in the first place:
// GET /auth/me (so the caller can see their own account) and
// POST /auth/change-password (the only way out).
func RequirePasswordChangeCleared(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
			return
		}

		if principal.RequiresPasswordChange {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "password change required before continuing"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireVerifiedEmailForRequester enforces OPS-068b's narrower gate:
// unlike RequirePasswordChangeCleared (blocks nearly everything), this
// blocks exactly one action — creating a work item — and only for an
// unverified requester. Every other role passes through untouched
// regardless of their EmailVerified value (it's only ever meaningful for
// self-service signup accounts). Applied only to POST /workitems in
// router.go.
func RequireVerifiedEmailForRequester(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
			return
		}

		if principal.HasRole(auth.RoleRequester) && !principal.EmailVerified {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "verify your email before creating a work item"})
			return
		}

		next.ServeHTTP(w, r)
	})
}
