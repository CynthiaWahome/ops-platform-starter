package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthHandlerLoginReturnsToken(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)
	handler := NewAuthHandler(service)

	body := bytes.NewBufferString(`{"identifier":"admin@ops.local","password":"ChangeMe123!"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response auth.Session
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("expected response body to be valid json, got error: %v", err)
	}

	if response.AccessToken == "" {
		t.Fatal("expected access token in response")
	}
}

func TestAuthHandlerMeReturnsPrincipalFromContext(t *testing.T) {
	t.Parallel()

	service := newTestAuthService(t)
	handler := NewAuthHandler(service)

	session, err := service.Login(t.Context(), "admin@ops.local", "ChangeMe123!")
	if err != nil {
		t.Fatalf("expected login to succeed, got error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()

	protected := httpmiddleware.RequireAuth(service, http.HandlerFunc(handler.Me))
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var principal auth.Principal
	if err := json.NewDecoder(rec.Body).Decode(&principal); err != nil {
		t.Fatalf("expected response body to be valid json, got error: %v", err)
	}

	if principal.UserID != "user-admin-001" {
		t.Fatalf("expected principal user id user-admin-001, got %s", principal.UserID)
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
