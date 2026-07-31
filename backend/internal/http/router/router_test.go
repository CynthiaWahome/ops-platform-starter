package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
)

func TestAdminProbeAllowsAdminAndRejectsAssignee(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)

	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	adminReq := httptest.NewRequest(http.MethodGet, "/access/admin", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)

	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected admin status %d, got %d", http.StatusOK, adminRec.Code)
	}

	assigneeReq := httptest.NewRequest(http.MethodGet, "/access/admin", nil)
	assigneeReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	assigneeRec := httptest.NewRecorder()
	handler.ServeHTTP(assigneeRec, assigneeReq)

	if assigneeRec.Code != http.StatusForbidden {
		t.Fatalf("expected assignee status %d, got %d", http.StatusForbidden, assigneeRec.Code)
	}
}

func TestAssigneeProbeAllowsAssigneeAndRejectsAdmin(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)

	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	assigneeReq := httptest.NewRequest(http.MethodGet, "/access/assignee", nil)
	assigneeReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	assigneeRec := httptest.NewRecorder()
	handler.ServeHTTP(assigneeRec, assigneeReq)

	if assigneeRec.Code != http.StatusOK {
		t.Fatalf("expected assignee status %d, got %d", http.StatusOK, assigneeRec.Code)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/access/assignee", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)

	if adminRec.Code != http.StatusForbidden {
		t.Fatalf("expected admin status %d, got %d", http.StatusForbidden, adminRec.Code)
	}
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	handler, err := New(config.Config{
		Port:                         "8080",
		AppEnv:                       "test",
		AuthTokenSecret:              "test-secret",
		AuthTokenTTL:                 time.Hour,
		BootstrapAdminIdentifier:     "admin@ops.local",
		BootstrapAdminPassword:       "ChangeMe123!",
		BootstrapAdminDisplayName:    "Platform Admin",
		BootstrapAssigneeIdentifier:  "assignee@ops.local",
		BootstrapAssigneePassword:    "ChangeMe123!",
		BootstrapAssigneeDisplayName: "Assigned Worker",
	})
	if err != nil {
		t.Fatalf("expected router to be created, got error: %v", err)
	}

	return handler
}

func loginAndReturnToken(t *testing.T, handler http.Handler, identifier, password string) string {
	t.Helper()

	body := bytes.NewBufferString(`{"identifier":"` + identifier + `","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d", http.StatusOK, rec.Code)
	}

	type sessionResponse struct {
		AccessToken string `json:"accessToken"`
	}

	var response sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("expected login response to decode, got error: %v", err)
	}

	if response.AccessToken == "" {
		t.Fatal("expected access token in login response")
	}

	return response.AccessToken
}
