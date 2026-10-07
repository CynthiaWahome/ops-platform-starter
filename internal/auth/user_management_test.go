package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func newTestUserManagementService(t *testing.T) Service {
	t.Helper()

	passwords := NewBcryptPasswordManager(bcrypt.MinCost)
	users := NewMemoryUserStore()

	if err := SeedBootstrapUsers(context.Background(), users, passwords, []BootstrapSeed{
		{
			ID:          "user-admin-001",
			Identifier:  "admin@ops.local",
			DisplayName: "Platform Admin",
			Password:    "ChangeMe123!",
			Roles:       []Role{RoleAdmin},
		},
	}); err != nil {
		t.Fatalf("expected bootstrap seeding to succeed, got error: %v", err)
	}

	tokens := NewJWTManager("test-secret", "ops-platform-starter-backend", time.Hour)

	return NewService(users, passwords, tokens)
}

func TestCreateUserRequiresPasswordChangeAndReturnsTempPassword(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, tempPassword, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "new-assignee@ops.local",
		DisplayName: "New Assignee",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	if tempPassword == "" {
		t.Fatal("expected a temp password to be returned")
	}

	if !user.RequiresPasswordChange {
		t.Fatal("expected new user to require a password change")
	}

	if user.CreatedByUserID != "user-admin-001" {
		t.Fatalf("expected created-by to be user-admin-001, got %s", user.CreatedByUserID)
	}

	session, err := service.Login(context.Background(), "new-assignee@ops.local", tempPassword)
	if err != nil {
		t.Fatalf("expected login with the temp password to succeed, got error: %v", err)
	}

	if !session.Principal.RequiresPasswordChange {
		t.Fatal("expected session principal to carry RequiresPasswordChange")
	}
}

func TestCreateUserRejectsDuplicateIdentifier(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	_, _, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "admin@ops.local",
		DisplayName: "Duplicate",
	})
	if !errors.Is(err, ErrIdentifierTaken) {
		t.Fatalf("expected ErrIdentifierTaken, got %v", err)
	}
}

func TestCreateUserRejectsUnknownRole(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	_, _, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        Role("owner"),
		Identifier:  "someone@ops.local",
		DisplayName: "Someone",
	})
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}
}

func TestChangePasswordClearsRequiresPasswordChangeFlag(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, tempPassword, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "temp-user@ops.local",
		DisplayName: "Temp User",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	if err := service.ChangePassword(context.Background(), user.ID, tempPassword, "a-real-password"); err != nil {
		t.Fatalf("expected change password to succeed, got error: %v", err)
	}

	session, err := service.Login(context.Background(), "temp-user@ops.local", "a-real-password")
	if err != nil {
		t.Fatalf("expected login with the new password to succeed, got error: %v", err)
	}

	if session.Principal.RequiresPasswordChange {
		t.Fatal("expected RequiresPasswordChange to be cleared after a successful change")
	}
}

func TestChangePasswordRejectsWrongOldPassword(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, _, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "temp-user-2@ops.local",
		DisplayName: "Temp User 2",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	err = service.ChangePassword(context.Background(), user.ID, "wrong-temp-password", "a-real-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestResetPasswordIssuesNewTempPasswordAndRequiresChangeAgain(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, tempPassword, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "reset-me@ops.local",
		DisplayName: "Reset Me",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	if err := service.ChangePassword(context.Background(), user.ID, tempPassword, "a-real-password"); err != nil {
		t.Fatalf("expected change password to succeed, got error: %v", err)
	}

	newTempPassword, err := service.ResetPassword(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("expected reset to succeed, got error: %v", err)
	}

	if newTempPassword == "a-real-password" {
		t.Fatal("expected a freshly generated temp password, not the old one")
	}

	// The old, self-chosen password must no longer work.
	if _, err := service.Login(context.Background(), "reset-me@ops.local", "a-real-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password to be rejected after reset, got %v", err)
	}

	session, err := service.Login(context.Background(), "reset-me@ops.local", newTempPassword)
	if err != nil {
		t.Fatalf("expected login with the new temp password to succeed, got error: %v", err)
	}

	if !session.Principal.RequiresPasswordChange {
		t.Fatal("expected RequiresPasswordChange to be true again after a reset")
	}
}

func TestChangePasswordPreservesLeadingAndTrailingWhitespace(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, tempPassword, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "whitespace-password@ops.local",
		DisplayName: "Whitespace Password",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	const chosenPassword = "  has spaces  "

	if err := service.ChangePassword(context.Background(), user.ID, tempPassword, chosenPassword); err != nil {
		t.Fatalf("expected change password to succeed, got error: %v", err)
	}

	// A review caught that trimming the new password before hashing it
	// (while Login never trims what's typed) made a password with
	// leading/trailing spaces impossible to type back in — this proves
	// the exact password chosen, whitespace included, logs back in.
	if _, err := service.Login(context.Background(), "whitespace-password@ops.local", chosenPassword); err != nil {
		t.Fatalf("expected login with the exact chosen password (including whitespace) to succeed, got error: %v", err)
	}
}

func TestChangePasswordRejectsOver72Bytes(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, tempPassword, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "long-password@ops.local",
		DisplayName: "Long Password",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	tooLong := make([]byte, 73)
	for i := range tooLong {
		tooLong[i] = 'a'
	}

	err = service.ChangePassword(context.Background(), user.ID, tempPassword, string(tooLong))
	if !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("expected ErrInvalidUserInput for a password over 72 bytes, got %v", err)
	}
}

func TestUpdateUserDeactivatesAccount(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	user, _, err := service.CreateUser(context.Background(), "user-admin-001", CreateUserInput{
		Role:        RoleAssignee,
		Identifier:  "deactivate-me@ops.local",
		DisplayName: "Deactivate Me",
	})
	if err != nil {
		t.Fatalf("expected user creation to succeed, got error: %v", err)
	}

	inactive := false
	updated, err := service.UpdateUser(context.Background(), user.ID, UpdateUserInput{IsActive: &inactive})
	if err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}

	if updated.IsActive {
		t.Fatal("expected user to be inactive after update")
	}
}

func TestSeedBootstrapUsersIsIdempotent(t *testing.T) {
	t.Parallel()

	users := NewMemoryUserStore()
	passwords := NewBcryptPasswordManager(bcrypt.MinCost)

	seeds := []BootstrapSeed{
		{
			ID:          "user-admin-001",
			Identifier:  "admin@ops.local",
			DisplayName: "Platform Admin",
			Password:    "ChangeMe123!",
			Roles:       []Role{RoleAdmin},
		},
	}

	if err := SeedBootstrapUsers(context.Background(), users, passwords, seeds); err != nil {
		t.Fatalf("expected first seed to succeed, got error: %v", err)
	}

	// Simulate an admin deactivating the bootstrap account before a
	// restart — a second seed call (what happens on every subsequent
	// process start) must not silently reactivate it.
	user, _ := users.FindByID(context.Background(), "user-admin-001")
	user.IsActive = false
	if _, err := users.Update(context.Background(), user); err != nil {
		t.Fatalf("expected deactivation update to succeed, got error: %v", err)
	}

	if err := SeedBootstrapUsers(context.Background(), users, passwords, seeds); err != nil {
		t.Fatalf("expected second seed to succeed, got error: %v", err)
	}

	after, _ := users.FindByID(context.Background(), "user-admin-001")
	if after.IsActive {
		t.Fatal("expected re-seeding to never overwrite an existing user row")
	}
}
