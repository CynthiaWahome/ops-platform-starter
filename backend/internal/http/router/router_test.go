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

func TestAdminWorkItemCRUDFlow(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")

	createBody := bytes.NewBufferString(`{"title":"Gate repaint","description":"Repaint estate gate","priority":"high"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/workitems", createBody)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	type workItemResponse struct {
		ID            string `json:"id"`
		ReferenceCode string `json:"referenceCode"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		Priority      string `json:"priority"`
		Status        string `json:"status"`
	}

	var created workItemResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("expected created work item response to decode, got error: %v", err)
	}

	if created.ID == "" {
		t.Fatal("expected created work item id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/workitems", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRec.Code)
	}

	var listed []workItemResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("expected listed work items response to decode, got error: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 listed work item, got %d", len(listed))
	}

	getReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get status %d, got %d", http.StatusOK, getRec.Code)
	}

	updateBody := bytes.NewBufferString(`{"title":"Updated gate repaint","priority":"medium"}`)
	updateReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created.ID, updateBody)
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d", http.StatusOK, updateRec.Code)
	}

	var updated workItemResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("expected updated work item response to decode, got error: %v", err)
	}

	if updated.Title != "Updated gate repaint" {
		t.Fatalf("expected updated title %q, got %q", "Updated gate repaint", updated.Title)
	}

	statusBody := bytes.NewBufferString(`{"toStatus":"assigned","reason":"assigned to on-call crew"}`)
	statusReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created.ID+"/status", statusBody)
	statusReq.Header.Set("Authorization", "Bearer "+adminToken)
	statusReq.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status change status %d, got %d", http.StatusOK, statusRec.Code)
	}

	var statusChanged workItemResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&statusChanged); err != nil {
		t.Fatalf("expected status change response to decode, got error: %v", err)
	}

	if statusChanged.Status != "assigned" {
		t.Fatalf("expected status %q, got %q", "assigned", statusChanged.Status)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created.ID+"/history", nil)
	historyReq.Header.Set("Authorization", "Bearer "+adminToken)
	historyRec := httptest.NewRecorder()
	handler.ServeHTTP(historyRec, historyReq)

	if historyRec.Code != http.StatusOK {
		t.Fatalf("expected history status %d, got %d", http.StatusOK, historyRec.Code)
	}

	type statusHistoryResponse struct {
		FromStatus      *string `json:"fromStatus"`
		ToStatus        string  `json:"toStatus"`
		ChangedByUserID string  `json:"changedByUserId"`
		Reason          *string `json:"reason"`
	}

	var history []statusHistoryResponse
	if err := json.NewDecoder(historyRec.Body).Decode(&history); err != nil {
		t.Fatalf("expected history response to decode, got error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry := history[0]

	if entry.FromStatus == nil || *entry.FromStatus != "created" {
		t.Fatalf("expected from status %q, got %v", "created", entry.FromStatus)
	}

	if entry.ToStatus != "assigned" {
		t.Fatalf("expected to status %q, got %q", "assigned", entry.ToStatus)
	}

	if entry.Reason == nil || *entry.Reason != "assigned to on-call crew" {
		t.Fatalf("expected reason to be recorded, got %v", entry.Reason)
	}
}

func TestAssigneeCannotCreateWorkItem(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	createBody := bytes.NewBufferString(`{"title":"Gate repaint","description":"Repaint estate gate","priority":"high"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/workitems", createBody)
	createReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusForbidden {
		t.Fatalf("expected create status %d, got %d", http.StatusForbidden, createRec.Code)
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
