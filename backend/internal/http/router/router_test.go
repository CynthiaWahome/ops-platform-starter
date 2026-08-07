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

func TestAdminAssignmentFlow(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")

	createBody := bytes.NewBufferString(`{"title":"Fence repair","description":"Repair the perimeter fence","priority":"medium"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/workitems", createBody)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("expected created work item response to decode, got error: %v", err)
	}

	assignBody := bytes.NewBufferString(`{"assignedToUserId":"user-assignee-001"}`)
	assignReq := httptest.NewRequest(http.MethodPost, "/workitems/"+created.ID+"/assignment", assignBody)
	assignReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignReq.Header.Set("Content-Type", "application/json")
	assignRec := httptest.NewRecorder()
	handler.ServeHTTP(assignRec, assignReq)

	if assignRec.Code != http.StatusCreated {
		t.Fatalf("expected assign status %d, got %d", http.StatusCreated, assignRec.Code)
	}

	type assignmentResponse struct {
		WorkItemID       string `json:"workItemId"`
		AssignedToUserID string `json:"assignedToUserId"`
		Status           string `json:"status"`
	}

	var assigned assignmentResponse
	if err := json.NewDecoder(assignRec.Body).Decode(&assigned); err != nil {
		t.Fatalf("expected assignment response to decode, got error: %v", err)
	}

	if assigned.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected assignedToUserId user-assignee-001, got %q", assigned.AssignedToUserID)
	}

	if assigned.Status != "assigned" {
		t.Fatalf("expected assignment status %q, got %q", "assigned", assigned.Status)
	}

	getAssignmentReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created.ID+"/assignment", nil)
	getAssignmentReq.Header.Set("Authorization", "Bearer "+adminToken)
	getAssignmentRec := httptest.NewRecorder()
	handler.ServeHTTP(getAssignmentRec, getAssignmentReq)

	if getAssignmentRec.Code != http.StatusOK {
		t.Fatalf("expected get assignment status %d, got %d", http.StatusOK, getAssignmentRec.Code)
	}

	var fetched assignmentResponse
	if err := json.NewDecoder(getAssignmentRec.Body).Decode(&fetched); err != nil {
		t.Fatalf("expected fetched assignment response to decode, got error: %v", err)
	}

	if fetched.WorkItemID != created.ID {
		t.Fatalf("expected fetched assignment work item id %q, got %q", created.ID, fetched.WorkItemID)
	}

	getWorkItemReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created.ID, nil)
	getWorkItemReq.Header.Set("Authorization", "Bearer "+adminToken)
	getWorkItemRec := httptest.NewRecorder()
	handler.ServeHTTP(getWorkItemRec, getWorkItemReq)

	var workItemAfterAssign struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(getWorkItemRec.Body).Decode(&workItemAfterAssign); err != nil {
		t.Fatalf("expected work item response to decode, got error: %v", err)
	}

	if workItemAfterAssign.Status != "assigned" {
		t.Fatalf("expected work item status %q after assignment, got %q", "assigned", workItemAfterAssign.Status)
	}
}

func TestAssigneeAcceptAssignmentFlow(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	created := createAndAssignWorkItem(t, handler, adminToken, "user-assignee-001")

	acceptBody := bytes.NewBufferString(`{"note":"on my way"}`)
	acceptReq := httptest.NewRequest(http.MethodPost, "/workitems/"+created+"/assignment/accept", acceptBody)
	acceptReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	handler.ServeHTTP(acceptRec, acceptReq)

	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept status %d, got %d", http.StatusOK, acceptRec.Code)
	}

	var accepted struct {
		Status       string `json:"status"`
		ResponseNote string `json:"responseNote"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&accepted); err != nil {
		t.Fatalf("expected accept response to decode, got error: %v", err)
	}

	if accepted.Status != "accepted" {
		t.Fatalf("expected assignment status %q, got %q", "accepted", accepted.Status)
	}

	getWorkItemReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created, nil)
	getWorkItemReq.Header.Set("Authorization", "Bearer "+adminToken)
	getWorkItemRec := httptest.NewRecorder()
	handler.ServeHTTP(getWorkItemRec, getWorkItemReq)

	var workItem struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(getWorkItemRec.Body).Decode(&workItem); err != nil {
		t.Fatalf("expected work item response to decode, got error: %v", err)
	}

	if workItem.Status != "accepted" {
		t.Fatalf("expected work item status %q after accept, got %q", "accepted", workItem.Status)
	}
}

func TestAssigneeDeclineAssignmentBouncesWorkItemToCreated(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	created := createAndAssignWorkItem(t, handler, adminToken, "user-assignee-001")

	declineReq := httptest.NewRequest(http.MethodPost, "/workitems/"+created+"/assignment/decline", bytes.NewBufferString(`{}`))
	declineReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	declineReq.Header.Set("Content-Type", "application/json")
	declineRec := httptest.NewRecorder()
	handler.ServeHTTP(declineRec, declineReq)

	if declineRec.Code != http.StatusOK {
		t.Fatalf("expected decline status %d, got %d", http.StatusOK, declineRec.Code)
	}

	var declined struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(declineRec.Body).Decode(&declined); err != nil {
		t.Fatalf("expected decline response to decode, got error: %v", err)
	}

	if declined.Status != "declined" {
		t.Fatalf("expected assignment status %q, got %q", "declined", declined.Status)
	}

	getWorkItemReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created, nil)
	getWorkItemReq.Header.Set("Authorization", "Bearer "+adminToken)
	getWorkItemRec := httptest.NewRecorder()
	handler.ServeHTTP(getWorkItemRec, getWorkItemReq)

	var workItem struct {
		Status           string  `json:"status"`
		AssignedToUserID *string `json:"assignedToUserId"`
	}
	if err := json.NewDecoder(getWorkItemRec.Body).Decode(&workItem); err != nil {
		t.Fatalf("expected work item response to decode, got error: %v", err)
	}

	if workItem.Status != "created" {
		t.Fatalf("expected work item status %q after decline, got %q", "created", workItem.Status)
	}

	if workItem.AssignedToUserID != nil {
		t.Fatal("expected assignedToUserId to be cleared after decline")
	}
}

// createAndAssignWorkItem is a small helper shared by the accept and
// decline flow tests: both need a freshly created and assigned work item
// before they can exercise the response endpoints.
func createAndAssignWorkItem(t *testing.T, handler http.Handler, adminToken string, assignedToUserID string) string {
	t.Helper()

	createBody := bytes.NewBufferString(`{"title":"Fence repair","description":"Repair the perimeter fence","priority":"medium"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/workitems", createBody)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, createRec.Code)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("expected created work item response to decode, got error: %v", err)
	}

	assignBody := bytes.NewBufferString(`{"assignedToUserId":"` + assignedToUserID + `"}`)
	assignReq := httptest.NewRequest(http.MethodPost, "/workitems/"+created.ID+"/assignment", assignBody)
	assignReq.Header.Set("Authorization", "Bearer "+adminToken)
	assignReq.Header.Set("Content-Type", "application/json")
	assignRec := httptest.NewRecorder()
	handler.ServeHTTP(assignRec, assignReq)

	if assignRec.Code != http.StatusCreated {
		t.Fatalf("expected assign status %d, got %d", http.StatusCreated, assignRec.Code)
	}

	return created.ID
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

func TestAssigneeOnlySeesOwnAssignedWorkItems(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	assignedID := createAndAssignWorkItem(t, handler, adminToken, "user-assignee-001")

	unassignedBody := bytes.NewBufferString(`{"title":"Unassigned job","description":"Not assigned to anyone","priority":"low"}`)
	unassignedReq := httptest.NewRequest(http.MethodPost, "/workitems", unassignedBody)
	unassignedReq.Header.Set("Authorization", "Bearer "+adminToken)
	unassignedReq.Header.Set("Content-Type", "application/json")
	unassignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unassignedRec, unassignedReq)

	if unassignedRec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d", http.StatusCreated, unassignedRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/workitems", nil)
	listReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRec.Code)
	}

	var listed []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("expected list response to decode, got error: %v", err)
	}

	if len(listed) != 1 || listed[0].ID != assignedID {
		t.Fatalf("expected assignee's list to contain only %q, got %+v", assignedID, listed)
	}

	getOwnReq := httptest.NewRequest(http.MethodGet, "/workitems/"+assignedID, nil)
	getOwnReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	getOwnRec := httptest.NewRecorder()
	handler.ServeHTTP(getOwnRec, getOwnReq)

	if getOwnRec.Code != http.StatusOK {
		t.Fatalf("expected get own work item status %d, got %d", http.StatusOK, getOwnRec.Code)
	}

	var unassignedCreated struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(unassignedRec.Body).Decode(&unassignedCreated); err != nil {
		t.Fatalf("expected unassigned create response to decode, got error: %v", err)
	}

	getUnownedReq := httptest.NewRequest(http.MethodGet, "/workitems/"+unassignedCreated.ID, nil)
	getUnownedReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	getUnownedRec := httptest.NewRecorder()
	handler.ServeHTTP(getUnownedRec, getUnownedReq)

	if getUnownedRec.Code != http.StatusNotFound {
		t.Fatalf("expected get unowned work item status %d, got %d", http.StatusNotFound, getUnownedRec.Code)
	}
}

func TestAssigneeCanStartWorkAndSubmitForReviewOnOwnWorkItem(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	created := createAndAssignWorkItem(t, handler, adminToken, "user-assignee-001")

	acceptReq := httptest.NewRequest(http.MethodPost, "/workitems/"+created+"/assignment/accept", bytes.NewBufferString(`{}`))
	acceptReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptRec := httptest.NewRecorder()
	handler.ServeHTTP(acceptRec, acceptReq)

	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept status %d, got %d", http.StatusOK, acceptRec.Code)
	}

	startReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created+"/status", bytes.NewBufferString(`{"toStatus":"in_progress"}`))
	startReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	startReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	handler.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("expected start work status %d, got %d", http.StatusOK, startRec.Code)
	}

	submitReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created+"/status", bytes.NewBufferString(`{"toStatus":"submitted_for_review"}`))
	submitReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	submitReq.Header.Set("Content-Type", "application/json")
	submitRec := httptest.NewRecorder()
	handler.ServeHTTP(submitRec, submitReq)

	if submitRec.Code != http.StatusOK {
		t.Fatalf("expected submit for review status %d, got %d", http.StatusOK, submitRec.Code)
	}

	// Verifying stays admin-only, even though the assignee just legally
	// moved the item into submitted_for_review.
	verifyReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created+"/status", bytes.NewBufferString(`{"toStatus":"verified"}`))
	verifyReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	handler.ServeHTTP(verifyRec, verifyReq)

	if verifyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected assignee verify attempt status %d, got %d", http.StatusBadRequest, verifyRec.Code)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created+"/history", nil)
	historyReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	historyRec := httptest.NewRecorder()
	handler.ServeHTTP(historyRec, historyReq)

	if historyRec.Code != http.StatusOK {
		t.Fatalf("expected assignee history status %d, got %d", http.StatusOK, historyRec.Code)
	}

	getAssignmentReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created+"/assignment", nil)
	getAssignmentReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	getAssignmentRec := httptest.NewRecorder()
	handler.ServeHTTP(getAssignmentRec, getAssignmentReq)

	if getAssignmentRec.Code != http.StatusOK {
		t.Fatalf("expected assignee assignment lookup status %d, got %d", http.StatusOK, getAssignmentRec.Code)
	}
}

func TestAssigneeCannotActOnOrViewUnownedWorkItem(t *testing.T) {
	t.Parallel()

	handler := newTestRouter(t)
	adminToken := loginAndReturnToken(t, handler, "admin@ops.local", "ChangeMe123!")
	assigneeToken := loginAndReturnToken(t, handler, "assignee@ops.local", "ChangeMe123!")

	// Created but never assigned to anyone, so it belongs to nobody's
	// scope but the admin's.
	createBody := bytes.NewBufferString(`{"title":"Unassigned job","description":"Not assigned to anyone","priority":"low"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/workitems", createBody)
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("expected created work item response to decode, got error: %v", err)
	}

	statusReq := httptest.NewRequest(http.MethodPatch, "/workitems/"+created.ID+"/status", bytes.NewBufferString(`{"toStatus":"in_progress"}`))
	statusReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	statusReq.Header.Set("Content-Type", "application/json")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusNotFound {
		t.Fatalf("expected status change on unowned item status %d, got %d", http.StatusNotFound, statusRec.Code)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/workitems/"+created.ID+"/history", nil)
	historyReq.Header.Set("Authorization", "Bearer "+assigneeToken)
	historyRec := httptest.NewRecorder()
	handler.ServeHTTP(historyRec, historyReq)

	if historyRec.Code != http.StatusNotFound {
		t.Fatalf("expected history on unowned item status %d, got %d", http.StatusNotFound, historyRec.Code)
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
