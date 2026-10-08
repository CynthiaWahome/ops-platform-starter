package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestServiceLoginReturnsSession(t *testing.T) {
	t.Parallel()

	service := newTestService(t)

	session, err := service.Login(context.Background(), "admin@ops.local", "ChangeMe123!")
	if err != nil {
		t.Fatalf("expected login to succeed, got error: %v", err)
	}

	if session.AccessToken == "" {
		t.Fatal("expected access token to be returned")
	}

	if !session.Principal.HasRole(RoleAdmin) {
		t.Fatal("expected principal to have admin role")
	}
}

func TestServiceLoginRejectsInvalidPassword(t *testing.T) {
	t.Parallel()

	service := newTestService(t)

	_, err := service.Login(context.Background(), "admin@ops.local", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials error, got %v", err)
	}
}

func TestServiceAuthenticateRejectsInvalidToken(t *testing.T) {
	t.Parallel()

	service := newTestService(t)

	_, err := service.Authenticate(context.Background(), "bad.token.value")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}

func newTestService(t *testing.T) Service {
	t.Helper()

	passwords := NewBcryptPasswordManager(bcrypt.MinCost)
	users := NewMemoryUserStore()

	passwordHash, err := passwords.Hash("ChangeMe123!")
	if err != nil {
		t.Fatalf("expected password hash to succeed, got error: %v", err)
	}

	if err := users.Seed(context.Background(), User{
		ID:           "user-admin-001",
		Identifier:   "admin@ops.local",
		DisplayName:  "Platform Admin",
		PasswordHash: passwordHash,
		Roles:        []Role{RoleAdmin},
		IsActive:     true,
	}); err != nil {
		t.Fatalf("expected user to be seeded, got error: %v", err)
	}

	tokens := NewJWTManager("test-secret", "ops-platform-starter-backend", time.Hour).
		WithClock(func() time.Time {
			return time.Date(2026, time.July, 31, 7, 0, 0, 0, time.UTC)
		})

	return NewService(users, passwords, tokens, nil)
}
