package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
	"golang.org/x/crypto/bcrypt"
)

// fakeEmailSender records every call instead of making a real call to an
// email provider — the only way these tests can ever see an OTP, since
// the handler never returns one in a response.
type fakeEmailSender struct {
	bodies []string
}

func (f *fakeEmailSender) Send(_ context.Context, _, _, body string) error {
	f.bodies = append(f.bodies, body)
	return nil
}

func (f *fakeEmailSender) last() string {
	return f.bodies[len(f.bodies)-1]
}

func newTestAuthServiceWithEmail(t *testing.T) (auth.Service, *fakeEmailSender) {
	t.Helper()

	passwords := auth.NewBcryptPasswordManager(bcrypt.MinCost)
	users := auth.NewMemoryUserStore()
	sender := &fakeEmailSender{}

	tokens := auth.NewJWTManager("test-secret", "ops-platform-starter-backend", time.Hour)

	return auth.NewService(users, passwords, tokens, sender), sender
}

func TestSignUpHandlerReturnsSessionAndSendsEmail(t *testing.T) {
	t.Parallel()

	service, sender := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	body := bytes.NewBufferString(`{"identifier":"new-requester@gmail.com","password":"a-real-password","displayName":"New Requester"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", body)
	rec := httptest.NewRecorder()

	handler.SignUp(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	if len(sender.bodies) != 1 {
		t.Fatalf("expected exactly 1 email sent, got %d", len(sender.bodies))
	}
}

func TestSignUpHandlerRejectsShortPassword(t *testing.T) {
	t.Parallel()

	service, _ := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	body := bytes.NewBufferString(`{"identifier":"short@gmail.com","password":"short","displayName":"Short"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", body)
	rec := httptest.NewRecorder()

	handler.SignUp(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestVerifyEmailHandlerSucceedsWithCorrectCode(t *testing.T) {
	t.Parallel()

	service, sender := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	session, err := service.SignUp(t.Context(), auth.SignUpInput{
		Identifier:  "verify-handler@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Verify Handler",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	code := extractOTPFromBody(sender.last())

	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email", bytes.NewBufferString(`{"code":"`+code+`"}`))
	req = req.WithContext(httpmiddleware.ContextWithPrincipal(req.Context(), session.Principal))
	rec := httptest.NewRecorder()

	handler.VerifyEmail(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}

func TestVerifyEmailHandlerRejectsWrongCode(t *testing.T) {
	t.Parallel()

	service, _ := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	session, err := service.SignUp(t.Context(), auth.SignUpInput{
		Identifier:  "verify-handler-2@gmail.com",
		Password:    "a-real-password",
		DisplayName: "Verify Handler 2",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/verify-email", bytes.NewBufferString(`{"code":"000000"}`))
	req = req.WithContext(httpmiddleware.ContextWithPrincipal(req.Context(), session.Principal))
	rec := httptest.NewRecorder()

	handler.VerifyEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestForgotPasswordHandlerAlwaysReturnsGenericResponse(t *testing.T) {
	t.Parallel()

	service, _ := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(`{"identifier":"nobody@gmail.com"}`))
	rec := httptest.NewRecorder()

	handler.ForgotPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestResetPasswordHandlerSucceedsWithCorrectCode(t *testing.T) {
	t.Parallel()

	service, sender := newTestAuthServiceWithEmail(t)
	handler := NewAuthHandler(service, nil)

	_, err := service.SignUp(t.Context(), auth.SignUpInput{
		Identifier:  "reset-handler@gmail.com",
		Password:    "original-password",
		DisplayName: "Reset Handler",
	})
	if err != nil {
		t.Fatalf("expected signup to succeed, got error: %v", err)
	}

	if err := service.RequestPasswordReset(t.Context(), "reset-handler@gmail.com"); err != nil {
		t.Fatalf("expected reset request to succeed, got error: %v", err)
	}

	code := extractOTPFromBody(sender.last())

	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password",
		bytes.NewBufferString(`{"identifier":"reset-handler@gmail.com","code":"`+code+`","newPassword":"a-brand-new-password"}`))
	rec := httptest.NewRecorder()

	handler.ResetPassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, rec.Code, rec.Body.String())
	}
}

func extractOTPFromBody(body string) string {
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
