package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
)

func TestRequireRolesRejectsMissingPrincipal(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	rec := httptest.NewRecorder()

	handler := RequireRoles(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), auth.RoleAdmin)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireRolesRejectsForbiddenRole(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req = req.WithContext(ContextWithPrincipal(req.Context(), auth.Principal{
		UserID:      "user-assignee-001",
		Identifier:  "assignee@ops.local",
		DisplayName: "Assigned Worker",
		Roles:       []auth.Role{auth.RoleAssignee},
	}))
	rec := httptest.NewRecorder()

	handler := RequireRoles(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), auth.RoleAdmin)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequireRolesAllowsMatchingRole(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req = req.WithContext(ContextWithPrincipal(req.Context(), auth.Principal{
		UserID:      "user-admin-001",
		Identifier:  "admin@ops.local",
		DisplayName: "Platform Admin",
		Roles:       []auth.Role{auth.RoleAdmin},
	}))
	rec := httptest.NewRecorder()

	handler := RequireRoles(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), auth.RoleAdmin)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
