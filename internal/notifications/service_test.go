package notifications

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceNotifyCreatesEntryVisibleToRecipient(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore())

	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0001", "assignment_created", "WI-0001 was assigned to you"); err != nil {
		t.Fatalf("expected notify to succeed, got error: %v", err)
	}

	list, err := service.List(context.Background(), "user-assignee-001", false)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(list))
	}

	if list[0].Kind != KindAssignmentCreated {
		t.Fatalf("expected kind %q, got %q", KindAssignmentCreated, list[0].Kind)
	}

	if list[0].ReadAt != nil {
		t.Fatal("expected a freshly created notification to be unread")
	}
}

func TestServiceNotifyRejectsMissingFields(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore())

	if err := service.Notify(context.Background(), "", "workitem-0001", "assignment_created", "message"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error for empty recipient, got %v", err)
	}

	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0001", "assignment_created", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error for empty message, got %v", err)
	}
}

func TestServiceListOnlyUnreadFiltersOutRead(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore())

	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0001", "assignment_created", "first"); err != nil {
		t.Fatalf("expected first notify to succeed, got error: %v", err)
	}
	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0002", "work_verified", "second"); err != nil {
		t.Fatalf("expected second notify to succeed, got error: %v", err)
	}

	all, err := service.List(context.Background(), "user-assignee-001", false)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(all))
	}

	if _, err := service.MarkAsRead(context.Background(), all[0].ID, "user-assignee-001"); err != nil {
		t.Fatalf("expected mark as read to succeed, got error: %v", err)
	}

	unread, err := service.List(context.Background(), "user-assignee-001", true)
	if err != nil {
		t.Fatalf("expected unread-only list to succeed, got error: %v", err)
	}

	if len(unread) != 1 {
		t.Fatalf("expected 1 unread notification remaining, got %d", len(unread))
	}
	if unread[0].ID != all[1].ID {
		t.Fatalf("expected remaining unread notification to be %q, got %q", all[1].ID, unread[0].ID)
	}
}

func TestServiceMarkAsReadRejectsWrongRecipient(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore())

	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0001", "assignment_created", "message"); err != nil {
		t.Fatalf("expected notify to succeed, got error: %v", err)
	}

	list, err := service.List(context.Background(), "user-assignee-001", false)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	// Someone else's notification comes back as ErrNotOwned — the
	// handler maps this to 404, same "acts as if it doesn't exist"
	// shape as everywhere else in this codebase.
	if _, err := service.MarkAsRead(context.Background(), list[0].ID, "user-assignee-002"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("expected not-owned error, got %v", err)
	}
}

func TestServiceMarkAsReadIsIdempotent(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore()).WithClock(func() time.Time {
		return time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	})

	if err := service.Notify(context.Background(), "user-assignee-001", "workitem-0001", "assignment_created", "message"); err != nil {
		t.Fatalf("expected notify to succeed, got error: %v", err)
	}

	list, err := service.List(context.Background(), "user-assignee-001", false)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	first, err := service.MarkAsRead(context.Background(), list[0].ID, "user-assignee-001")
	if err != nil {
		t.Fatalf("expected first mark as read to succeed, got error: %v", err)
	}

	// Marking an already-read notification read again succeeds and
	// leaves ReadAt untouched, rather than erroring on a client retry.
	second, err := service.MarkAsRead(context.Background(), list[0].ID, "user-assignee-001")
	if err != nil {
		t.Fatalf("expected second mark as read to succeed, got error: %v", err)
	}

	if !first.ReadAt.Equal(*second.ReadAt) {
		t.Fatalf("expected ReadAt to stay unchanged, got %v then %v", first.ReadAt, second.ReadAt)
	}
}
