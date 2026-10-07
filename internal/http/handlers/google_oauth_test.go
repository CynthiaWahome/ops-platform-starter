package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
)

// fakeGoogleOAuthClient lets these tests prove the handler's own logic
// (state check, wiring into auth.Service) without ever making a real
// network call to Google — the same reason every domain in this codebase
// is built behind an interface instead of a concrete type.
type fakeGoogleOAuthClient struct {
	identity auth.GoogleIdentity
	err      error
}

func (f fakeGoogleOAuthClient) AuthCodeURL(state string) string {
	return "https://accounts.google.com/o/oauth2/auth?state=" + state
}

func (f fakeGoogleOAuthClient) Exchange(_ context.Context, _ string) (auth.GoogleIdentity, error) {
	return f.identity, f.err
}

func TestGoogleLoginSetsStateCookieAndRedirects(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(newTestAuthService(t), fakeGoogleOAuthClient{})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	rec := httptest.NewRecorder()

	handler.GoogleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, rec.Code)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "google_oauth_state" || cookies[0].Value == "" {
		t.Fatalf("expected a non-empty google_oauth_state cookie, got %v", cookies)
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("expected a redirect Location header")
	}
}

func TestGoogleCallbackRejectsMissingState(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(newTestAuthService(t), fakeGoogleOAuthClient{})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=irrelevant", nil)
	rec := httptest.NewRecorder()

	handler.GoogleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGoogleCallbackRejectsStateMismatch(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(newTestAuthService(t), fakeGoogleOAuthClient{})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=irrelevant&state=wrong-value", nil)
	req.AddCookie(&http.Cookie{Name: "google_oauth_state", Value: "correct-value"})
	rec := httptest.NewRecorder()

	handler.GoogleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGoogleCallbackLogsInOnValidExchange(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(newTestAuthService(t), fakeGoogleOAuthClient{
		identity: auth.GoogleIdentity{
			Subject: "google-subject-xyz",
			Email:   "fresh-requester@gmail.com",
			Name:    "Fresh Requester",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=real-code&state=matching-value", nil)
	req.AddCookie(&http.Cookie{Name: "google_oauth_state", Value: "matching-value"})
	rec := httptest.NewRecorder()

	handler.GoogleCallback(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestGoogleCallbackSurfacesExchangeFailureAsBadGateway(t *testing.T) {
	t.Parallel()

	handler := NewAuthHandler(newTestAuthService(t), fakeGoogleOAuthClient{
		err: errors.New("google is unreachable"),
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=real-code&state=matching-value", nil)
	req.AddCookie(&http.Cookie{Name: "google_oauth_state", Value: "matching-value"})
	rec := httptest.NewRecorder()

	handler.GoogleCallback(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rec.Code)
	}
}
