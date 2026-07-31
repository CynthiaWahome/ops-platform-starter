package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

func TestWorkItemHandlerCreateReturnsCreatedItem(t *testing.T) {
	t.Parallel()

	service := workitems.NewService(workitems.NewMemoryStore()).WithClock(func() time.Time {
		return time.Date(2026, time.July, 31, 18, 15, 0, 0, time.UTC)
	})
	handler := NewWorkItemHandler(service)

	body := bytes.NewBufferString(`{"title":"Gate repaint","description":"Repaint the estate gate","priority":"high"}`)
	req := httptest.NewRequest(http.MethodPost, "/workitems", body)
	req = req.WithContext(middleware.ContextWithPrincipal(req.Context(), auth.Principal{
		UserID:      "user-admin-001",
		Identifier:  "admin@ops.local",
		DisplayName: "Platform Admin",
		Roles:       []auth.Role{auth.RoleAdmin},
	}))
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var item workitems.WorkItem
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatalf("expected response body to decode, got error: %v", err)
	}

	if item.CreatedByUserID != "user-admin-001" {
		t.Fatalf("expected creator user id user-admin-001, got %s", item.CreatedByUserID)
	}
}
