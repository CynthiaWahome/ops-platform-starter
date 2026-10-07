package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// fakeEmailSender records every call instead of making a real call to
// Resend or SMTP — lets these tests inspect the actual OTP sent, since
// the service only ever hands it to EmailSender, never returns it in an
// API response.
type fakeEmailSender struct {
	mu   sync.Mutex
	sent []sentEmail
	err  error
}

type sentEmail struct {
	To      string
	Subject string
	Body    string
}

func (f *fakeEmailSender) Send(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	f.sent = append(f.sent, sentEmail{To: to, Subject: subject, Body: body})
	return nil
}

func (f *fakeEmailSender) last() sentEmail {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.sent[len(f.sent)-1]
}

func newTestSignupService(t *testing.T) (Service, *fakeEmailSender) {
	t.Helper()

	passwords := NewBcryptPasswordManager(bcrypt.MinCost)
	users := NewMemoryUserStore()
	sender := &fakeEmailSender{}

	tokens := NewJWTManager("test-secret", "ops-platform-starter-backend", time.Hour)

	return NewService(users, passwords, tokens, sender), sender
}

// extractOTP pulls the 6-digit code out of a sentEmail's body — the
// fakeEmailSender is the only place a test can observe the code at all,
// since SignUp/RequestPasswordReset never return it directly.
func extractOTP(body string) string {
	start := -1
	for i, r := range body {
		if r >= '0' && r <= '9' {
			start = i
			break
		}
	}
	if start == -1 || start+6 > len(body) {
		return ""
	}
	return body[start : start+6]
}

func TestSignUpCreatesUnverifiedRequesterAndSendsCode(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	session, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "new-requester@gmail.com",
		Password:    "a-real-password",
		DisplayName: "New Requester",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	if !session.Principal.HasRole(RoleRequester) {
		t.Fatalf("expected a requester account, got roles %v", session.Principal.Roles)
	}

	if session.Principal.EmailVerified {
		t.Fatal("expected a brand-new signup to be unverified")
	}

	if len(sender.sent) != 1 || sender.last().To != "new-requester@gmail.com" {
		t.Fatalf("expected exactly 1 verification email sent to the new account, got %+v", sender.sent)
	}
}

func TestSignUpRejectsShortPassword(t *testing.T) {
	t.Parallel()

	service, _ := newTestSignupService(t)

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "short-password@gmail.com",
		Password:    "short",
		DisplayName: "Short Password",
	})
	if !errors.Is(err, ErrInvalidUserInput) {
		t.Fatalf("expected ErrInvalidUserInput, got %v", err)
	}
}

// TestSignUpNeverCreatesAccountWhenEmailSendFails proves the ordering fix
// a live manual test caught: a failed send (e.g. Resend rejecting an
// unverified "from" domain) must never leave an orphaned account behind
// — one with a stored OTP hash nobody can ever retrieve, because the
// email that was supposed to carry the code never arrived.
func TestSignUpNeverCreatesAccountWhenEmailSendFails(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)
	sender.err = errors.New("resend: domain not verified")

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "never-created@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Never Created",
	})
	if err == nil {
		t.Fatal("expected signup to fail when the email send fails")
	}

	if _, exists := service.users.FindByIdentifier(context.Background(), "never-created@gmail.com"); exists {
		t.Fatal("expected no account to have been created when the verification email failed to send")
	}

	// Retrying with a working sender must succeed — proving the first
	// attempt didn't leave the identifier permanently squatted.
	sender.err = nil
	if _, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "never-created@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Never Created",
	}); err != nil {
		t.Fatalf("expected retry after a fixed sender to succeed, got error: %v", err)
	}
}

func TestSignUpRejectsDuplicateIdentifier(t *testing.T) {
	t.Parallel()

	service, _ := newTestSignupService(t)

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "dup@gmail.com",
		Password:    "a-real-password",
		DisplayName: "First",
	})
	if err != nil {
		t.Fatalf("expected first signup to succeed, got error: %v", err)
	}

	_, err = service.SignUp(context.Background(), SignUpInput{
		Identifier:  "dup@gmail.com",
		Password:    "another-password",
		DisplayName: "Second",
	})
	if !errors.Is(err, ErrIdentifierTaken) {
		t.Fatalf("expected ErrIdentifierTaken, got %v", err)
	}
}

func TestVerifyEmailWithCorrectCodeSucceeds(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	session, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "verify-me@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Verify Me",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	code := extractOTP(sender.last().Body)
	if code == "" {
		t.Fatalf("expected to extract a code from the sent email body: %q", sender.last().Body)
	}

	if err := service.VerifyEmail(context.Background(), session.Principal.UserID, code); err != nil {
		t.Fatalf("expected verification to succeed, got error: %v", err)
	}

	user, ok := service.users.FindByID(context.Background(), session.Principal.UserID)
	if !ok || !user.EmailVerified {
		t.Fatal("expected the account to be marked verified")
	}
}

func TestVerifyEmailRejectsWrongCode(t *testing.T) {
	t.Parallel()

	service, _ := newTestSignupService(t)

	session, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "wrong-code@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Wrong Code",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	err = service.VerifyEmail(context.Background(), session.Principal.UserID, "000000")
	if !errors.Is(err, ErrInvalidOrExpiredCode) {
		t.Fatalf("expected ErrInvalidOrExpiredCode, got %v", err)
	}
}

func TestVerifyEmailRejectsExpiredCode(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	service = service.WithClock(func() time.Time { return now })

	session, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "expired-code@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Expired Code",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	code := extractOTP(sender.last().Body)

	service = service.WithClock(func() time.Time { return now.Add(11 * time.Minute) })

	err = service.VerifyEmail(context.Background(), session.Principal.UserID, code)
	if !errors.Is(err, ErrInvalidOrExpiredCode) {
		t.Fatalf("expected ErrInvalidOrExpiredCode for an expired code, got %v", err)
	}
}

func TestVerifyEmailLocksOutAfterTooManyAttempts(t *testing.T) {
	t.Parallel()

	service, _ := newTestSignupService(t)

	session, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "lockout@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Lockout",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	for range otpMaxAttempts {
		_ = service.VerifyEmail(context.Background(), session.Principal.UserID, "000000")
	}

	err = service.VerifyEmail(context.Background(), session.Principal.UserID, "000000")
	if !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected ErrTooManyAttempts after %d wrong guesses, got %v", otpMaxAttempts+1, err)
	}
}

func TestRequestPasswordResetIsSilentForNonexistentAccount(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	if err := service.RequestPasswordReset(context.Background(), "nobody@gmail.com"); err != nil {
		t.Fatalf("expected no error for a nonexistent account, got %v", err)
	}

	if len(sender.sent) != 0 {
		t.Fatalf("expected no email sent for a nonexistent account, got %+v", sender.sent)
	}
}

// TestRequestPasswordResetNeverLeaksEmailSendFailureAsAnError proves
// RequestPasswordReset's whole contract (never reveal whether identifier
// belongs to an account) survives an actual send failure — before the
// fix, an existing account whose email happened to fail got a different
// error than a nonexistent one, which is itself an enumeration oracle.
func TestRequestPasswordResetNeverLeaksEmailSendFailureAsAnError(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "send-fails@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Send Fails",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	sender.err = errors.New("resend: domain not verified")

	if err := service.RequestPasswordReset(context.Background(), "send-fails@gmail.com"); err != nil {
		t.Fatalf("expected no error even though the underlying send failed, got %v", err)
	}
}

func TestRequestPasswordResetOnlySendsForRequesterAccounts(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	if err := SeedBootstrapUsers(context.Background(), service.users, service.passwords, []BootstrapSeed{
		{
			ID:          "user-admin-001",
			Identifier:  "admin@ops.local",
			DisplayName: "Platform Admin",
			Password:    "ChangeMe123!",
			Roles:       []Role{RoleAdmin},
		},
	}); err != nil {
		t.Fatalf("expected seeding to succeed, got error: %v", err)
	}

	if err := service.RequestPasswordReset(context.Background(), "admin@ops.local"); err != nil {
		t.Fatalf("expected no error for an internal account, got %v", err)
	}

	if len(sender.sent) != 0 {
		t.Fatalf("expected no reset email sent for a non-requester account, got %+v", sender.sent)
	}
}

func TestCompletePasswordResetWithCorrectCodeSucceeds(t *testing.T) {
	t.Parallel()

	service, sender := newTestSignupService(t)

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "reset-me@gmail.com",
		Password:    "original-password",
		DisplayName: "Reset Me",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	if err := service.RequestPasswordReset(context.Background(), "reset-me@gmail.com"); err != nil {
		t.Fatalf("expected reset request to succeed, got error: %v", err)
	}

	code := extractOTP(sender.last().Body)

	if err := service.CompletePasswordReset(context.Background(), "reset-me@gmail.com", code, "a-brand-new-password"); err != nil {
		t.Fatalf("expected reset completion to succeed, got error: %v", err)
	}

	if _, err := service.Login(context.Background(), "reset-me@gmail.com", "a-brand-new-password"); err != nil {
		t.Fatalf("expected login with the new password to succeed, got error: %v", err)
	}

	if _, err := service.Login(context.Background(), "reset-me@gmail.com", "original-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected the old password to be rejected, got %v", err)
	}
}

func TestCompletePasswordResetLocksOutAfterTooManyAttempts(t *testing.T) {
	t.Parallel()

	service, _ := newTestSignupService(t)

	_, err := service.SignUp(context.Background(), SignUpInput{
		Identifier:  "reset-lockout@gmail.com",
		Password:    "original-password",
		DisplayName: "Reset Lockout",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	if err := service.RequestPasswordReset(context.Background(), "reset-lockout@gmail.com"); err != nil {
		t.Fatalf("expected reset request to succeed, got error: %v", err)
	}

	for range otpMaxAttempts {
		_ = service.CompletePasswordReset(context.Background(), "reset-lockout@gmail.com", "000000", "irrelevant-password")
	}

	err = service.CompletePasswordReset(context.Background(), "reset-lockout@gmail.com", "000000", "irrelevant-password")
	if !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected ErrTooManyAttempts, got %v", err)
	}
}
