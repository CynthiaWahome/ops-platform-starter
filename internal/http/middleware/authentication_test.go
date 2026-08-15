package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

// OPS-042: before this file, RequireAuth (the piece that actually
// extracts and validates the bearer token) had zero direct tests — every
// existing test that touched it went through a full login + protected
// request round trip (e.g. TestAuthHandlerMeReturnsPrincipalFromContext),
// which proves the happy path works but never isolated the specific
// rejection cases below. authorization_test.go, by contrast, already
// tests RequireRoles directly the same way this file now tests
// RequireAuth.

func TestRequireAuthRejectsMissingAuthorizationHeader(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()

	handler := RequireAuth(service, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthRejectsHeaderWithoutBearerPrefix(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Token some-value")
	rec := httptest.NewRecorder()

	handler := RequireAuth(service, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthRejectsBearerWithEmptyToken(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	handler := RequireAuth(service, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthRejectsInvalidToken(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()

	handler := RequireAuth(service, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthAllowsValidTokenAndSetsPrincipal(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)

	session, err := service.Login(t.Context(), "admin@ops.local", "ChangeMe123!")
	if err != nil {
		t.Fatalf("expected login to succeed, got error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()

	var sawPrincipal auth.Principal
	var sawOK bool

	handler := RequireAuth(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPrincipal, sawOK = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !sawOK {
		t.Fatal("expected principal to be set in request context")
	}

	if sawPrincipal.UserID != "user-admin-001" {
		t.Fatalf("expected principal user id user-admin-001, got %s", sawPrincipal.UserID)
	}
}

func newTestAuthService(t *testing.T) auth.Service {
	t.Helper()

	passwords := auth.NewBcryptPasswordManager(bcrypt.MinCost)
	users, err := auth.NewStaticUserStore(passwords, []auth.StaticUserSeed{
		{
			ID:          "user-admin-001",
			Identifier:  "admin@ops.local",
			DisplayName: "Platform Admin",
			Password:    "ChangeMe123!",
			Roles:       []auth.Role{auth.RoleAdmin},
			IsActive:    true,
		},
	})
	if err != nil {
		t.Fatalf("expected static store to be created, got error: %v", err)
	}

	tokens := auth.NewJWTManager("test-secret", "ops-platform-starter-backend", time.Hour).
		WithClock(func() time.Time {
			return time.Date(2026, time.July, 31, 7, 0, 0, 0, time.UTC)
		})

	return auth.NewService(users, passwords, tokens)
}
