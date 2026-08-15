package workitems

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/notifications"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/teams"
)

func TestServiceCreateGeneratesWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil).WithClock(func() time.Time {
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil).WithClock(func() time.Time {
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

	updated, err := service.Update(context.Background(), item.ID, "user-admin-001", true, false, UpdateInput{
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil).WithClock(func() time.Time {
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

	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, false, ChangeStatusInput{
		ToStatus: StatusAssigned,
		Reason:   &reason,
	})
	if err != nil {
		t.Fatalf("expected status change to succeed, got error: %v", err)
	}

	if updated.Status != StatusAssigned {
		t.Fatalf("expected status %q, got %q", StatusAssigned, updated.Status)
	}

	history, err := service.ListStatusHistory(context.Background(), item.ID, "user-admin-001", true, false)
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, false, ChangeStatusInput{
		ToStatus: StatusCompleted,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
}

func TestServiceAssignWorkItemCreatesAssignmentAndMovesStatus(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil).WithClock(func() time.Time {
		return time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
	})

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	assignment, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if assignment.WorkItemID != item.ID {
		t.Fatalf("expected assignment work item id %q, got %q", item.ID, assignment.WorkItemID)
	}

	if assignment.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected assigned to user-assignee-001, got %q", assignment.AssignedToUserID)
	}

	if assignment.Status != AssignmentStatusAssigned {
		t.Fatalf("expected assignment status %q, got %q", AssignmentStatusAssigned, assignment.Status)
	}

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true, false, false)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got error: %v", err)
	}

	if updated.Status != StatusAssigned {
		t.Fatalf("expected work item status %q, got %q", StatusAssigned, updated.Status)
	}

	if updated.AssignedToUserID == nil || *updated.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected work item assignedToUserId to be set, got %v", updated.AssignedToUserID)
	}

	history, err := service.ListStatusHistory(context.Background(), item.ID, "user-admin-001", true, false)
	if err != nil {
		t.Fatalf("expected history lookup to succeed, got error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry from assignment, got %d", len(history))
	}

	if history[0].ToStatus != StatusAssigned {
		t.Fatalf("expected history entry to status %q, got %q", StatusAssigned, history[0].ToStatus)
	}

	fetched, err := service.GetAssignment(context.Background(), item.ID, "user-admin-001", true, false)
	if err != nil {
		t.Fatalf("expected get assignment to succeed, got error: %v", err)
	}

	if fetched.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected fetched assignment assignedToUserId user-assignee-001, got %q", fetched.AssignedToUserID)
	}
}

func TestServiceAssignWorkItemRejectsSecondAssignment(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected first assign to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-002",
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error on second assignment, got %v", err)
	}
}

func TestServiceGetAssignmentReturnsNotFoundBeforeAnyAssignment(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.GetAssignment(context.Background(), item.ID, "user-admin-001", true, false)
	if !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("expected assignment not found error, got %v", err)
	}
}

func TestServiceRespondToAssignmentAccept(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil).WithClock(func() time.Time {
		return time.Date(2026, time.August, 6, 9, 0, 0, 0, time.UTC)
	})

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	note := "on my way"

	assignment, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{
		Note: &note,
	})
	if err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	if assignment.Status != AssignmentStatusAccepted {
		t.Fatalf("expected assignment status %q, got %q", AssignmentStatusAccepted, assignment.Status)
	}

	if assignment.RespondedAt == nil {
		t.Fatal("expected respondedAt to be set")
	}

	if assignment.ResponseNote == nil || *assignment.ResponseNote != note {
		t.Fatalf("expected response note %q, got %v", note, assignment.ResponseNote)
	}

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true, false, false)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got error: %v", err)
	}

	if updated.Status != StatusAccepted {
		t.Fatalf("expected work item status %q, got %q", StatusAccepted, updated.Status)
	}

	if updated.AssignedToUserID == nil || *updated.AssignedToUserID != "user-assignee-001" {
		t.Fatal("expected assignedToUserId to remain set after accept")
	}
}

func TestServiceRespondToAssignmentDeclineBouncesWorkItemToCreated(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	assignment, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", false, RespondToAssignmentInput{})
	if err != nil {
		t.Fatalf("expected decline to succeed, got error: %v", err)
	}

	if assignment.Status != AssignmentStatusDeclined {
		t.Fatalf("expected assignment status %q, got %q", AssignmentStatusDeclined, assignment.Status)
	}

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true, false, false)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got error: %v", err)
	}

	if updated.Status != StatusCreated {
		t.Fatalf("expected work item status %q after decline, got %q", StatusCreated, updated.Status)
	}

	if updated.AssignedToUserID != nil {
		t.Fatal("expected assignedToUserId to be cleared after decline")
	}
}

func TestServiceListAssignmentHistorySurvivesReassignment(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	// First assignee declines...
	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected first assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", false, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected decline to succeed, got error: %v", err)
	}

	// ...so the admin reassigns to someone else, who accepts. AssignmentStore
	// (the current-state store) now only knows about the second assignee —
	// this is exactly the gap OPS-040 closes.
	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-002",
	}); err != nil {
		t.Fatalf("expected reassign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-002", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected second accept to succeed, got error: %v", err)
	}

	// The current assignment only shows the second assignee.
	current, err := service.GetAssignment(context.Background(), item.ID, "user-admin-001", true, false)
	if err != nil {
		t.Fatalf("expected get assignment to succeed, got error: %v", err)
	}
	if current.AssignedToUserID != "user-assignee-002" {
		t.Fatalf("expected current assignment to user-assignee-002, got %q", current.AssignedToUserID)
	}

	// But the assignment history has the full trail — all four events,
	// including the first assignee who is no longer referenced anywhere
	// else in the system.
	history, err := service.ListAssignmentHistory(context.Background(), item.ID, "user-admin-001", true, false)
	if err != nil {
		t.Fatalf("expected list assignment history to succeed, got error: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("expected 4 assignment history entries, got %d", len(history))
	}

	wantActions := []AssignmentStatus{AssignmentStatusAssigned, AssignmentStatusDeclined, AssignmentStatusAssigned, AssignmentStatusAccepted}
	wantAssignees := []string{"user-assignee-001", "user-assignee-001", "user-assignee-002", "user-assignee-002"}
	for i, entry := range history {
		if entry.Action != wantActions[i] {
			t.Fatalf("entry %d: expected action %q, got %q", i, wantActions[i], entry.Action)
		}
		if entry.AssignedToUserID != wantAssignees[i] {
			t.Fatalf("entry %d: expected assignedToUserId %q, got %q", i, wantAssignees[i], entry.AssignedToUserID)
		}
	}
}

func TestServiceRespondToAssignmentRejectsWrongUser(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	_, err = service.RespondToAssignment(context.Background(), item.ID, "user-assignee-999", true, RespondToAssignmentInput{})
	if !errors.Is(err, ErrAssignmentNotOwned) {
		t.Fatalf("expected assignment not owned error, got %v", err)
	}
}

func TestServiceRespondToAssignmentRejectsAlreadyRespondedTo(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	_, err = service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{})
	if err != nil {
		t.Fatalf("expected first accept to succeed, got error: %v", err)
	}

	_, err = service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", false, RespondToAssignmentInput{})
	if !errors.Is(err, ErrAssignmentNotPending) {
		t.Fatalf("expected assignment not pending error, got %v", err)
	}
}

func TestServiceListScopesToAdminSeesAllAssigneeSeesOwn(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	itemA, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Fence repair", Description: "Repair the fence", Priority: PriorityLow,
	}); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), itemA.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	// itemB is deliberately left unassigned, so it should never appear in
	// the assignee's scoped list.

	adminList, err := service.List(context.Background(), "user-admin-001", true, false, false)
	if err != nil {
		t.Fatalf("expected admin list to succeed, got error: %v", err)
	}

	if len(adminList) != 2 {
		t.Fatalf("expected admin to see 2 work items, got %d", len(adminList))
	}

	assigneeList, err := service.List(context.Background(), "user-assignee-001", false, false, false)
	if err != nil {
		t.Fatalf("expected assignee list to succeed, got error: %v", err)
	}

	if len(assigneeList) != 1 {
		t.Fatalf("expected assignee to see 1 work item, got %d", len(assigneeList))
	}

	if assigneeList[0].ID != itemA.ID {
		t.Fatalf("expected assignee's list to contain %q, got %q", itemA.ID, assigneeList[0].ID)
	}
}

func TestServiceGetByIDHidesUnownedWorkItemFromAssignee(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	// The assigned user can see it.
	if _, err := service.GetByID(context.Background(), item.ID, "user-assignee-001", false, false, false); err != nil {
		t.Fatalf("expected assigned user to see the work item, got error: %v", err)
	}

	// A different assignee cannot — and gets ErrNotFound, not a forbidden
	// error, so the response doesn't reveal that the id belongs to someone
	// else.
	_, err = service.GetByID(context.Background(), item.ID, "user-assignee-999", false, false, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item, got %v", err)
	}

	// An admin can always see it.
	if _, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true, false, false); err != nil {
		t.Fatalf("expected admin to see the work item, got error: %v", err)
	}
}

func TestServiceChangeStatusAllowsAssigneeStartWorkAndSubmitForReview(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	// The assignee can start work (accepted -> in_progress) on their own
	// item, as a non-admin caller.
	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	})
	if err != nil {
		t.Fatalf("expected assignee to start work, got error: %v", err)
	}

	if updated.Status != StatusInProgress {
		t.Fatalf("expected status %q, got %q", StatusInProgress, updated.Status)
	}

	// The assignee can then submit for review.
	updated, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusSubmittedForReview,
	})
	if err != nil {
		t.Fatalf("expected assignee to submit for review, got error: %v", err)
	}

	if updated.Status != StatusSubmittedForReview {
		t.Fatalf("expected status %q, got %q", StatusSubmittedForReview, updated.Status)
	}

	// But the assignee cannot verify their own submitted work — that stays
	// admin-only, even though it is a legal transition in the full table.
	_, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusVerified,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected assignee verify attempt to be rejected, got %v", err)
	}
}

func TestServiceNotificationsFireForEventHookedTransitionsOnly(t *testing.T) {
	t.Parallel()

	notificationService := notifications.NewService(notifications.NewMemoryStore())
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notificationService, nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	// assignment_created -> the assignee.
	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	// assignment_accepted -> the admin who assigned it.
	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	// Starting work is a real transition but not in the Event Hooks
	// list — no notification should fire for it.
	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	}); err != nil {
		t.Fatalf("expected start work to succeed, got error: %v", err)
	}

	// evidence_submitted -> the creator.
	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusSubmittedForReview,
	}); err != nil {
		t.Fatalf("expected submit for review to succeed, got error: %v", err)
	}

	// work_flagged -> the assignee.
	feedback := "retake the photo"
	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, false, ChangeStatusInput{
		ToStatus: StatusFlagged,
		Reason:   &feedback,
	}); err != nil {
		t.Fatalf("expected flag to succeed, got error: %v", err)
	}

	assigneeNotifications, err := notificationService.List(context.Background(), "user-assignee-001", false)
	if err != nil {
		t.Fatalf("expected assignee notification list to succeed, got error: %v", err)
	}

	wantAssigneeKinds := []notifications.Kind{notifications.KindAssignmentCreated, notifications.KindWorkFlagged}
	if len(assigneeNotifications) != len(wantAssigneeKinds) {
		t.Fatalf("expected %d assignee notifications (assignment_created, work_flagged — no notification for starting work), got %d: %+v", len(wantAssigneeKinds), len(assigneeNotifications), assigneeNotifications)
	}
	for i, want := range wantAssigneeKinds {
		if assigneeNotifications[i].Kind != want {
			t.Fatalf("assignee notification %d: expected kind %q, got %q", i, want, assigneeNotifications[i].Kind)
		}
	}

	adminNotifications, err := notificationService.List(context.Background(), "user-admin-001", false)
	if err != nil {
		t.Fatalf("expected admin notification list to succeed, got error: %v", err)
	}

	wantAdminKinds := []notifications.Kind{notifications.KindAssignmentAccepted, notifications.KindEvidenceSubmitted}
	if len(adminNotifications) != len(wantAdminKinds) {
		t.Fatalf("expected %d admin (creator) notifications, got %d: %+v", len(wantAdminKinds), len(adminNotifications), adminNotifications)
	}
	for i, want := range wantAdminKinds {
		if adminNotifications[i].Kind != want {
			t.Fatalf("admin notification %d: expected kind %q, got %q", i, want, adminNotifications[i].Kind)
		}
	}
}

func TestServiceNotificationsDoNotFireOnDecline(t *testing.T) {
	t.Parallel()

	notificationService := notifications.NewService(notifications.NewMemoryStore())
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notificationService, nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", false, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected decline to succeed, got error: %v", err)
	}

	// Declining is not in the Event Hooks list — the admin only got the
	// assignment_created notification's mirror image (nothing), and
	// still has zero notifications after the decline.
	adminNotifications, err := notificationService.List(context.Background(), "user-admin-001", false)
	if err != nil {
		t.Fatalf("expected admin notification list to succeed, got error: %v", err)
	}

	if len(adminNotifications) != 0 {
		t.Fatalf("expected 0 admin notifications after a decline, got %d: %+v", len(adminNotifications), adminNotifications)
	}
}

func TestServiceChangeStatusAllowsAssigneeToReworkFlaggedWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	}); err != nil {
		t.Fatalf("expected assignee to start work, got error: %v", err)
	}

	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusSubmittedForReview,
	}); err != nil {
		t.Fatalf("expected assignee to submit for review, got error: %v", err)
	}

	// Admin flags it back — this is the one leg of the flag flow that
	// stays admin-only, exercised here as plain ChangeStatus since the
	// handler-level "note is required" rule lives in the HTTP layer, not
	// the service.
	feedback := "Photo is blurry, retake before resubmitting"
	if _, err := service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, false, ChangeStatusInput{
		ToStatus: StatusFlagged,
		Reason:   &feedback,
	}); err != nil {
		t.Fatalf("expected admin to flag, got error: %v", err)
	}

	// OPS-033: the assignee can now pick the rework back up themselves —
	// Flagged -> InProgress — without waiting on an admin to move it,
	// closing the gap between the design doc and the code.
	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	})
	if err != nil {
		t.Fatalf("expected assignee to rework flagged item, got error: %v", err)
	}

	if updated.Status != StatusInProgress {
		t.Fatalf("expected status %q, got %q", StatusInProgress, updated.Status)
	}
}

func TestServiceChangeStatusRejectsAssigneeActingOnUnownedWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	// A different assignee, not the one this item is assigned to, gets
	// ErrNotFound — same "acts as if it doesn't exist" rule as GetByID.
	_, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-999", false, false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item, got %v", err)
	}
}

func TestServiceListStatusHistoryScopesToOwnWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.ListStatusHistory(context.Background(), item.ID, "user-assignee-001", false, false); err != nil {
		t.Fatalf("expected assigned user to see history, got error: %v", err)
	}

	_, err = service.ListStatusHistory(context.Background(), item.ID, "user-assignee-999", false, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item history, got %v", err)
	}
}

func TestServiceGetAssignmentScopesToOwnWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), notifications.NewService(notifications.NewMemoryStore()), nil, nil)

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.GetAssignment(context.Background(), item.ID, "user-assignee-001", false, false); err != nil {
		t.Fatalf("expected assigned user to see the assignment, got error: %v", err)
	}

	_, err = service.GetAssignment(context.Background(), item.ID, "user-assignee-999", false, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned assignment, got %v", err)
	}
}

// --- OPS-045: supervisor + team scoping ---

// newSupervisorTestService wires a real teams.Service in as TeamAuthority
// (not a hand-rolled mock) — same "construct the real collaborator" choice
// OPS-041's notification tests made for notifications.Service.
func newSupervisorTestService(t *testing.T) (Service, teams.Service) {
	t.Helper()

	teamSvc := teams.NewService(teams.NewMemoryStore(), teams.NewMemoryMembershipStore(), teams.NewMemorySupervisionStore())
	workSvc := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, teamSvc, nil)

	return workSvc, teamSvc
}

func TestSupervisorCanRunFullLifecycleForOwnTeam(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	team, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}
	if _, err := teamSvc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-supervisor-001", false, true, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected supervisor to assign within own team, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(ctx, item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected assignee to accept, got error: %v", err)
	}
	for _, toStatus := range []Status{StatusInProgress, StatusSubmittedForReview} {
		if _, err := service.ChangeStatus(ctx, item.ID, "user-assignee-001", false, false, ChangeStatusInput{ToStatus: toStatus}); err != nil {
			t.Fatalf("expected assignee move to %q to succeed, got error: %v", toStatus, err)
		}
	}

	if _, err := service.ChangeStatus(ctx, item.ID, "user-supervisor-001", false, true, ChangeStatusInput{ToStatus: StatusVerified}); err != nil {
		t.Fatalf("expected supervisor to verify own team's work, got error: %v", err)
	}

	updated, err := service.ChangeStatus(ctx, item.ID, "user-supervisor-001", false, true, ChangeStatusInput{ToStatus: StatusCompleted})
	if err != nil {
		t.Fatalf("expected supervisor to mark own team's work completed, got error: %v", err)
	}

	if updated.Status != StatusCompleted {
		t.Fatalf("expected status %q, got %q", StatusCompleted, updated.Status)
	}
}

func TestSupervisorCannotAssignOutsideOwnTeam(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	teamA, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}
	teamB, err := teamSvc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := teamSvc.AddAssignee(ctx, teamB.ID, "user-assignee-b", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team B, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamA.ID, "user-supervisor-a", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added to team A, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-a", CreateInput{
		Title: "Fence repair", Description: "Fix the east fence", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(ctx, item.ID, "user-supervisor-a", false, true, AssignInput{
		AssignedToUserID: "user-assignee-b",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected team A's supervisor to be rejected assigning to team B's assignee, got %v", err)
	}
}

func TestSupervisorCannotVerifyAnotherTeamsWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	teamA, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}
	teamB, err := teamSvc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := teamSvc.AddAssignee(ctx, teamB.ID, "user-assignee-b", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team B, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamB.ID, "user-supervisor-b", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor B to be added, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamA.ID, "user-supervisor-a", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor A to be added, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-b", CreateInput{
		Title: "Fence repair", Description: "Fix the east fence", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-supervisor-b", false, true, AssignInput{
		AssignedToUserID: "user-assignee-b",
	}); err != nil {
		t.Fatalf("expected team B's supervisor to assign within own team, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(ctx, item.ID, "user-assignee-b", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected assignee to accept, got error: %v", err)
	}
	for _, toStatus := range []Status{StatusInProgress, StatusSubmittedForReview} {
		if _, err := service.ChangeStatus(ctx, item.ID, "user-assignee-b", false, false, ChangeStatusInput{ToStatus: toStatus}); err != nil {
			t.Fatalf("expected assignee move to %q to succeed, got error: %v", toStatus, err)
		}
	}

	_, err = service.ChangeStatus(ctx, item.ID, "user-supervisor-a", false, true, ChangeStatusInput{ToStatus: StatusVerified})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected team A's supervisor to be rejected verifying team B's work, got %v", err)
	}
}

func TestAdminRetainsGlobalAuthorityAcrossAllTeams(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	team, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}
	if _, err := teamSvc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}
	// Deliberately no supervisor added to this team — admin should still
	// be able to act on it, unaffected by team structure. This is the
	// "both supervisors unavailable" edge case resolved architecturally:
	// admin is a standing fallback, not a special-cased one.

	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected admin to assign despite no team supervisor, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(ctx, item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected assignee to accept, got error: %v", err)
	}
	for _, toStatus := range []Status{StatusInProgress, StatusSubmittedForReview} {
		if _, err := service.ChangeStatus(ctx, item.ID, "user-assignee-001", false, false, ChangeStatusInput{ToStatus: toStatus}); err != nil {
			t.Fatalf("expected assignee move to %q to succeed, got error: %v", toStatus, err)
		}
	}

	if _, err := service.ChangeStatus(ctx, item.ID, "user-admin-001", true, false, ChangeStatusInput{ToStatus: StatusVerified}); err != nil {
		t.Fatalf("expected admin to verify despite no team supervisor, got error: %v", err)
	}
}

func TestSupervisorListSeesOnlyOwnTeamWork(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	teamA, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}
	teamB, err := teamSvc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := teamSvc.AddAssignee(ctx, teamA.ID, "user-assignee-a", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team A, got error: %v", err)
	}
	if _, err := teamSvc.AddAssignee(ctx, teamB.ID, "user-assignee-b", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team B, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamA.ID, "user-supervisor-a", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added to team A, got error: %v", err)
	}

	itemA, err := service.Create(ctx, "user-supervisor-a", CreateInput{
		Title: "Team A task", Description: "Belongs to team A", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}
	if _, err := service.AssignWorkItem(ctx, itemA.ID, "user-supervisor-a", false, true, AssignInput{
		AssignedToUserID: "user-assignee-a",
	}); err != nil {
		t.Fatalf("expected assign within team A to succeed, got error: %v", err)
	}

	itemB, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Team B task", Description: "Belongs to team B", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}
	if _, err := service.AssignWorkItem(ctx, itemB.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-b",
	}); err != nil {
		t.Fatalf("expected assign within team B to succeed, got error: %v", err)
	}

	visible, err := service.List(ctx, "user-supervisor-a", false, true, false)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if len(visible) != 1 || visible[0].ID != itemA.ID {
		t.Fatalf("expected supervisor to see only team A's work item, got %+v", visible)
	}
}

// TestSupervisorCannotAdoptAnotherUsersUnassignedWorkItem closes a real gap
// a review caught on PR #52: the original AssignWorkItem check only
// verified the *destination* assignee was on the caller's team, never that
// the caller had any authority over the work item itself. That let a
// supervisor "adopt" an admin's (or another supervisor's) hidden,
// not-yet-assigned item just by knowing its id and pointing it at their own
// team, even though List/GetByID correctly hid it from them beforehand.
func TestSupervisorCannotAdoptAnotherUsersUnassignedWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	team, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}
	if _, err := teamSvc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added, got error: %v", err)
	}

	// Admin creates a work item — the supervisor above has no authority
	// over it (they didn't create it, and it isn't assigned to anyone on
	// their team yet).
	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Admin's own task", Description: "Not the supervisor's to give away", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(ctx, item.ID, "user-supervisor-001", false, true, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected supervisor to be rejected assigning an item they have no authority over, got %v", err)
	}
}

func TestChangeStatusToFlaggedRequiresFeedbackRegardlessOfEntryPoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}
	if _, err := service.RespondToAssignment(ctx, item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}
	if _, err := service.ChangeStatus(ctx, item.ID, "user-assignee-001", false, false, ChangeStatusInput{ToStatus: StatusInProgress}); err != nil {
		t.Fatalf("expected start work to succeed, got error: %v", err)
	}
	if _, err := service.ChangeStatus(ctx, item.ID, "user-assignee-001", false, false, ChangeStatusInput{ToStatus: StatusSubmittedForReview}); err != nil {
		t.Fatalf("expected submit for review to succeed, got error: %v", err)
	}

	// Even an admin, going through the generic ChangeStatus entry point
	// (not the dedicated POST .../flag route, which validates this
	// itself before ever calling ChangeStatus), cannot flag with no
	// reason and no note.
	_, err = service.ChangeStatus(ctx, item.ID, "user-admin-001", true, false, ChangeStatusInput{ToStatus: StatusFlagged})
	if !errors.Is(err, ErrFeedbackRequired) {
		t.Fatalf("expected feedback-required error flagging with no reason, got %v", err)
	}

	whitespace := "   "
	_, err = service.ChangeStatus(ctx, item.ID, "user-admin-001", true, false, ChangeStatusInput{ToStatus: StatusFlagged, Reason: &whitespace})
	if !errors.Is(err, ErrFeedbackRequired) {
		t.Fatalf("expected feedback-required error flagging with whitespace-only reason, got %v", err)
	}

	reason := "retake the photo"
	updated, err := service.ChangeStatus(ctx, item.ID, "user-admin-001", true, false, ChangeStatusInput{ToStatus: StatusFlagged, Reason: &reason})
	if err != nil {
		t.Fatalf("expected flag with a real reason to succeed, got error: %v", err)
	}
	if updated.Status != StatusFlagged {
		t.Fatalf("expected status %q, got %q", StatusFlagged, updated.Status)
	}
}

// --- OPS-046: Update team scoping ---

func TestSupervisorCanEditOwnUnassignedWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	team, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	// Editable before it's even assigned — the supervisor created it.
	updatedTitle := "Repaint the main gate"
	updated, err := service.Update(ctx, item.ID, "user-supervisor-001", false, true, UpdateInput{Title: &updatedTitle})
	if err != nil {
		t.Fatalf("expected supervisor to edit their own unassigned item, got error: %v", err)
	}
	if updated.Title != updatedTitle {
		t.Fatalf("expected title %q, got %q", updatedTitle, updated.Title)
	}
}

// TestSupervisorCannotEditOnceAssigned closes a real gap a review caught on
// PR #54: the permission matrix's action is literally "Edit unassigned work
// item" — supervisorMayActOn alone checks *who* may act on a team's work,
// not *when*, so without this extra gate a supervisor could keep rewriting
// a work item's core fields straight through verified/completed. Admin is
// unaffected — admin's edit latitude stays unrestricted at any status, same
// as everywhere else in this package.
func TestSupervisorCannotEditOnceAssigned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	team, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}
	if _, err := teamSvc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-supervisor-001", false, true, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	updatedDescription := "Repaint the main gate, front and back"
	_, err = service.Update(ctx, item.ID, "user-supervisor-001", false, true, UpdateInput{Description: &updatedDescription})
	if !errors.Is(err, ErrAlreadyAssigned) {
		t.Fatalf("expected supervisor edit of an assigned item to be rejected, got %v", err)
	}
}

// TestAdminCanEditWorkItemAtAnyStatus confirms OPS-046/the PR #54 review fix
// only narrows the supervisor path — admin keeps unrestricted edit
// latitude regardless of the work item's current status, matching admin's
// behavior everywhere else in this package.
func TestAdminCanEditWorkItemAtAnyStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(ctx, item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	updatedTitle := "Still editable by admin"
	updated, err := service.Update(ctx, item.ID, "user-admin-001", true, false, UpdateInput{Title: &updatedTitle})
	if err != nil {
		t.Fatalf("expected admin to edit an assigned item, got error: %v", err)
	}
	if updated.Title != updatedTitle {
		t.Fatalf("expected title %q, got %q", updatedTitle, updated.Title)
	}
}

func TestSupervisorCannotEditAnotherTeamsWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, teamSvc := newSupervisorTestService(t)

	teamA, err := teamSvc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}
	teamB, err := teamSvc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := teamSvc.AddAssignee(ctx, teamB.ID, "user-assignee-b", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team B, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamB.ID, "user-supervisor-b", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor B to be added, got error: %v", err)
	}
	if _, err := teamSvc.AddSupervisor(ctx, teamA.ID, "user-supervisor-a", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor A to be added, got error: %v", err)
	}

	item, err := service.Create(ctx, "user-supervisor-b", CreateInput{
		Title: "Fence repair", Description: "Fix the east fence", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}
	if _, err := service.AssignWorkItem(ctx, item.ID, "user-supervisor-b", false, true, AssignInput{
		AssignedToUserID: "user-assignee-b",
	}); err != nil {
		t.Fatalf("expected team B's supervisor to assign within own team, got error: %v", err)
	}

	unwantedTitle := "Hijacked title"
	_, err = service.Update(ctx, item.ID, "user-supervisor-a", false, true, UpdateInput{Title: &unwantedTitle})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected team A's supervisor to be rejected editing team B's work item, got %v", err)
	}
}

func TestAssigneeCannotEditWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}
	if _, err := service.AssignWorkItem(ctx, item.ID, "user-admin-001", true, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	unwantedTitle := "Not the assignee's to change"
	_, err = service.Update(ctx, item.ID, "user-assignee-001", false, false, UpdateInput{Title: &unwantedTitle})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected assignee to be rejected editing a work item, got %v", err)
	}
}

// --- OPS-047: requester role ---

func TestRequesterSeesOnlyOwnCreatedWorkItems(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	own, err := service.Create(ctx, "user-requester-001", CreateInput{
		Title: "Fix leaking tap", Description: "Kitchen tap won't stop dripping", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.Create(ctx, "user-requester-999", CreateInput{
		Title: "Someone else's request", Description: "Not user-requester-001's business", Priority: PriorityLow,
	}); err != nil {
		t.Fatalf("expected second create to succeed, got error: %v", err)
	}

	visible, err := service.List(ctx, "user-requester-001", false, false, true)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if len(visible) != 1 || visible[0].ID != own.ID {
		t.Fatalf("expected requester to see only their own work item, got %+v", visible)
	}

	if _, err := service.GetByID(ctx, own.ID, "user-requester-001", false, false, true); err != nil {
		t.Fatalf("expected requester to view their own work item, got error: %v", err)
	}
}

func TestRequesterCannotSeeAnotherRequestersWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	item, err := service.Create(ctx, "user-requester-001", CreateInput{
		Title: "Fix leaking tap", Description: "Kitchen tap won't stop dripping", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.GetByID(ctx, item.ID, "user-requester-999", false, false, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected another requester to be rejected viewing this work item, got %v", err)
	}
}

// TestRequesterCannotActOnWorkItem is a defense-in-depth test, same shape
// as TestAssigneeCannotEditWorkItem — the router never opens
// assign/verify/flag/status/update routes to RoleRequester at all, but
// this confirms the service layer itself has no requester path into any
// of them, in case that role check were ever accidentally loosened.
func TestRequesterCannotActOnWorkItem(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore(), NewMemoryAssignmentHistoryStore(), nil, nil, nil)

	item, err := service.Create(ctx, "user-requester-001", CreateInput{
		Title: "Fix leaking tap", Description: "Kitchen tap won't stop dripping", Priority: PriorityLow,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(ctx, item.ID, "user-requester-001", false, false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected requester assign attempt to be rejected, got %v", err)
	}

	_, err = service.ChangeStatus(ctx, item.ID, "user-requester-001", false, false, ChangeStatusInput{ToStatus: StatusCancelled})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected requester status-change attempt to be rejected, got %v", err)
	}
}
