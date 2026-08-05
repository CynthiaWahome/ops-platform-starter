package workitems

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCreateGeneratesWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore()).WithClock(func() time.Time {
		return time.Date(2026, time.July, 31, 17, 30, 0, 0, time.UTC)
	})

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       " Main gate repaint ",
		Description: " Repaint the gate and fix surface cracks. ",
		Priority:    PriorityHigh,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if item.ID == "" || item.ReferenceCode == "" {
		t.Fatal("expected generated id and reference code")
	}

	if item.Status != StatusCreated {
		t.Fatalf("expected status %q, got %q", StatusCreated, item.Status)
	}

	if item.Title != "Main gate repaint" {
		t.Fatalf("expected trimmed title, got %q", item.Title)
	}
}

func TestServiceCreateRejectsInvalidPriority(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore())

	_, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    Priority("urgent-now"),
	})
	if !errors.Is(err, ErrInvalidPriority) {
		t.Fatalf("expected invalid priority error, got %v", err)
	}
}

func TestServiceUpdateChangesEditableFields(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore()).WithClock(func() time.Time {
		return time.Date(2026, time.July, 31, 18, 0, 0, 0, time.UTC)
	})

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	updatedTitle := "Updated gate repaint"
	updatedPriority := PriorityHigh

	updated, err := service.Update(context.Background(), item.ID, UpdateInput{
		Title:    &updatedTitle,
		Priority: &updatedPriority,
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}

	if updated.Title != updatedTitle {
		t.Fatalf("expected title %q, got %q", updatedTitle, updated.Title)
	}

	if updated.Priority != updatedPriority {
		t.Fatalf("expected priority %q, got %q", updatedPriority, updated.Priority)
	}
}

func TestServiceChangeStatusRecordsHistory(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore()).WithClock(func() time.Time {
		return time.Date(2026, time.August, 5, 9, 0, 0, 0, time.UTC)
	})

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	reason := "assigned to on-call crew"

	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-admin-001", ChangeStatusInput{
		ToStatus: StatusAssigned,
		Reason:   &reason,
	})
	if err != nil {
		t.Fatalf("expected status change to succeed, got error: %v", err)
	}

	if updated.Status != StatusAssigned {
		t.Fatalf("expected status %q, got %q", StatusAssigned, updated.Status)
	}

	history, err := service.ListStatusHistory(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("expected history lookup to succeed, got error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}

	entry := history[0]

	if entry.FromStatus == nil || *entry.FromStatus != StatusCreated {
		t.Fatalf("expected from status %q, got %v", StatusCreated, entry.FromStatus)
	}

	if entry.ToStatus != StatusAssigned {
		t.Fatalf("expected to status %q, got %q", StatusAssigned, entry.ToStatus)
	}

	if entry.ChangedByUserID != "user-admin-001" {
		t.Fatalf("expected changed by user-admin-001, got %q", entry.ChangedByUserID)
	}

	if entry.Reason == nil || *entry.Reason != reason {
		t.Fatalf("expected reason %q, got %v", reason, entry.Reason)
	}
}

func TestServiceChangeStatusRejectsIllegalTransition(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.ChangeStatus(context.Background(), item.ID, "user-admin-001", ChangeStatusInput{
		ToStatus: StatusCompleted,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
}
