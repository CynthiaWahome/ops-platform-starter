package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLoginWithGoogleCreatesRequesterOnFirstLogin(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	session, err := service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-001",
		Email:   "new-requester@gmail.com",
		Name:    "New Requester",
	})
	if err != nil {
		t.Fatalf("expected google login to succeed, got error: %v", err)
	}

	if !session.Principal.HasRole(RoleRequester) {
		t.Fatalf("expected a new Google login to create a requester, got roles %v", session.Principal.Roles)
	}

	if session.Principal.RequiresPasswordChange {
		t.Fatal("expected a Google-authenticated account to never require a password change")
	}
}

func TestLoginWithGoogleFindsExistingAccountOnRepeatLogin(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	first, err := service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-002",
		Email:   "repeat-requester@gmail.com",
		Name:    "Repeat Requester",
	})
	if err != nil {
		t.Fatalf("expected first google login to succeed, got error: %v", err)
	}

	second, err := service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-002",
		Email:   "repeat-requester@gmail.com",
		Name:    "Repeat Requester",
	})
	if err != nil {
		t.Fatalf("expected second google login to succeed, got error: %v", err)
	}

	if first.Principal.UserID != second.Principal.UserID {
		t.Fatalf("expected repeat login for the same Google subject to find the same account, got %s then %s", first.Principal.UserID, second.Principal.UserID)
	}

	all, err := service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("expected list users to succeed, got error: %v", err)
	}

	matches := 0
	for _, u := range all {
		if u.Identifier == "repeat-requester@gmail.com" {
			matches++
		}
	}

	if matches != 1 {
		t.Fatalf("expected exactly 1 account for the repeat Google identity, got %d", matches)
	}
}

func TestLoginWithGoogleRejectsEmailAlreadyUsedByLocalAccount(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	// admin@ops.local already exists as a local (password) bootstrap
	// account — linking identities is explicitly out of scope (OPS-068a),
	// so this must be rejected, not silently merged or duplicated.
	_, err := service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-003",
		Email:   "admin@ops.local",
		Name:    "Someone Else",
	})
	if !errors.Is(err, ErrGoogleEmailAlreadyRegistered) {
		t.Fatalf("expected ErrGoogleEmailAlreadyRegistered, got %v", err)
	}
}

func TestLoginWithGoogleRejectsInactiveAccount(t *testing.T) {
	t.Parallel()

	service := newTestUserManagementService(t)

	session, err := service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-004",
		Email:   "to-deactivate@gmail.com",
		Name:    "To Deactivate",
	})
	if err != nil {
		t.Fatalf("expected first google login to succeed, got error: %v", err)
	}

	inactive := false
	if _, err := service.UpdateUser(context.Background(), session.Principal.UserID, UpdateUserInput{IsActive: &inactive}); err != nil {
		t.Fatalf("expected deactivation to succeed, got error: %v", err)
	}

	_, err = service.LoginWithGoogle(context.Background(), GoogleIdentity{
		Subject: "google-subject-004",
		Email:   "to-deactivate@gmail.com",
		Name:    "To Deactivate",
	})
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("expected ErrInactiveUser, got %v", err)
	}
}

// TestMemoryUserStoreCreateRejectsDuplicateGoogleSubjectID is a
// defense-in-depth test, not a normal-path one: Service.LoginWithGoogle
// already prevents a duplicate by calling FindByGoogleSubjectID before
// Create, so this calls the store directly to prove it has its own
// uniqueness guarantee too — matching PostgresUserStore's real UNIQUE
// constraint on the column — for the case where two concurrent
// first-logins for the same brand-new identity both pass that check
// before either Create commits.
func TestMemoryUserStoreCreateRejectsDuplicateGoogleSubjectID(t *testing.T) {
	t.Parallel()

	store := NewMemoryUserStore()
	subjectID := "google-subject-race"

	_, err := store.Create(context.Background(), User{
		Identifier:      "first@gmail.com",
		DisplayName:     "First",
		Roles:           []Role{RoleRequester},
		IsActive:        true,
		AuthProvider:    AuthProviderGoogle,
		GoogleSubjectID: &subjectID,
	})
	if err != nil {
		t.Fatalf("expected first create to succeed, got error: %v", err)
	}

	_, err = store.Create(context.Background(), User{
		Identifier:      "second@gmail.com",
		DisplayName:     "Second",
		Roles:           []Role{RoleRequester},
		IsActive:        true,
		AuthProvider:    AuthProviderGoogle,
		GoogleSubjectID: &subjectID,
	})
	if !errors.Is(err, ErrIdentifierTaken) {
		t.Fatalf("expected a second account with the same GoogleSubjectID to be rejected, got %v", err)
	}
}

func TestParseGoogleUserInfoRejectsUnverifiedEmail(t *testing.T) {
	t.Parallel()

	body := strings.NewReader(`{"sub":"12345","email":"someone@gmail.com","email_verified":false,"name":"Someone"}`)

	_, err := parseGoogleUserInfo(body)
	if err == nil {
		t.Fatal("expected an error for an unverified Google email, got nil")
	}
	if !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("expected the error to mention the email isn't verified, got: %v", err)
	}
}

func TestParseGoogleUserInfoRejectsMissingEmailVerifiedField(t *testing.T) {
	t.Parallel()

	// Google's own field, omitted entirely, unmarshals to Go's bool zero
	// value (false) — same rejection as an explicit false, not silently
	// treated as verified just because the field wasn't sent.
	body := strings.NewReader(`{"sub":"12345","email":"someone@gmail.com","name":"Someone"}`)

	_, err := parseGoogleUserInfo(body)
	if err == nil {
		t.Fatal("expected an error when email_verified is absent, got nil")
	}
}

func TestParseGoogleUserInfoAcceptsVerifiedEmail(t *testing.T) {
	t.Parallel()

	body := strings.NewReader(`{"sub":"12345","email":"someone@gmail.com","email_verified":true,"name":"Someone"}`)

	identity, err := parseGoogleUserInfo(body)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if identity.Subject != "12345" || identity.Email != "someone@gmail.com" {
		t.Fatalf("expected the identity fields to be populated correctly, got %+v", identity)
	}
}

func TestParseGoogleUserInfoRejectsMissingSubjectOrEmail(t *testing.T) {
	t.Parallel()

	body := strings.NewReader(`{"email_verified":true,"name":"Someone"}`)

	_, err := parseGoogleUserInfo(body)
	if err == nil {
		t.Fatal("expected an error for a missing sub/email, got nil")
	}
}

func TestGenerateOAuthStateProducesDistinctValues(t *testing.T) {
	t.Parallel()

	first, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("expected state generation to succeed, got error: %v", err)
	}

	second, err := GenerateOAuthState()
	if err != nil {
		t.Fatalf("expected state generation to succeed, got error: %v", err)
	}

	if first == "" || second == "" {
		t.Fatal("expected non-empty state values")
	}

	if first == second {
		t.Fatal("expected two generated state values to differ")
	}
}
