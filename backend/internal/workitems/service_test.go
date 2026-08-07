package workitems

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCreateGeneratesWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore()).WithClock(func() time.Time {
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore()).WithClock(func() time.Time {
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore()).WithClock(func() time.Time {
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

	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, ChangeStatusInput{
		ToStatus: StatusAssigned,
		Reason:   &reason,
	})
	if err != nil {
		t.Fatalf("expected status change to succeed, got error: %v", err)
	}

	if updated.Status != StatusAssigned {
		t.Fatalf("expected status %q, got %q", StatusAssigned, updated.Status)
	}

	history, err := service.ListStatusHistory(context.Background(), item.ID, "user-admin-001", true)
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.ChangeStatus(context.Background(), item.ID, "user-admin-001", true, ChangeStatusInput{
		ToStatus: StatusCompleted,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error, got %v", err)
	}
}

func TestServiceAssignWorkItemCreatesAssignmentAndMovesStatus(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore()).WithClock(func() time.Time {
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

	assignment, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
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

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got error: %v", err)
	}

	if updated.Status != StatusAssigned {
		t.Fatalf("expected work item status %q, got %q", StatusAssigned, updated.Status)
	}

	if updated.AssignedToUserID == nil || *updated.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected work item assignedToUserId to be set, got %v", updated.AssignedToUserID)
	}

	history, err := service.ListStatusHistory(context.Background(), item.ID, "user-admin-001", true)
	if err != nil {
		t.Fatalf("expected history lookup to succeed, got error: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry from assignment, got %d", len(history))
	}

	if history[0].ToStatus != StatusAssigned {
		t.Fatalf("expected history entry to status %q, got %q", StatusAssigned, history[0].ToStatus)
	}

	fetched, err := service.GetAssignment(context.Background(), item.ID, "user-admin-001", true)
	if err != nil {
		t.Fatalf("expected get assignment to succeed, got error: %v", err)
	}

	if fetched.AssignedToUserID != "user-assignee-001" {
		t.Fatalf("expected fetched assignment assignedToUserId user-assignee-001, got %q", fetched.AssignedToUserID)
	}
}

func TestServiceAssignWorkItemRejectsSecondAssignment(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err != nil {
		t.Fatalf("expected first assign to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-002",
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition error on second assignment, got %v", err)
	}
}

func TestServiceGetAssignmentReturnsNotFoundBeforeAnyAssignment(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.GetAssignment(context.Background(), item.ID, "user-admin-001", true)
	if !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("expected assignment not found error, got %v", err)
	}
}

func TestServiceRespondToAssignmentAccept(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore()).WithClock(func() time.Time {
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

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
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

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true)
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
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

	updated, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true)
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

func TestServiceRespondToAssignmentRejectsWrongUser(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title:       "Gate repaint",
		Description: "Repaint the gate",
		Priority:    PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

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

	if _, err := service.AssignWorkItem(context.Background(), itemA.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	// itemB is deliberately left unassigned, so it should never appear in
	// the assignee's scoped list.

	adminList, err := service.List(context.Background(), "user-admin-001", true)
	if err != nil {
		t.Fatalf("expected admin list to succeed, got error: %v", err)
	}

	if len(adminList) != 2 {
		t.Fatalf("expected admin to see 2 work items, got %d", len(adminList))
	}

	assigneeList, err := service.List(context.Background(), "user-assignee-001", false)
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

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	// The assigned user can see it.
	if _, err := service.GetByID(context.Background(), item.ID, "user-assignee-001", false); err != nil {
		t.Fatalf("expected assigned user to see the work item, got error: %v", err)
	}

	// A different assignee cannot — and gets ErrNotFound, not a forbidden
	// error, so the response doesn't reveal that the id belongs to someone
	// else.
	_, err = service.GetByID(context.Background(), item.ID, "user-assignee-999", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item, got %v", err)
	}

	// An admin can always see it.
	if _, err := service.GetByID(context.Background(), item.ID, "user-admin-001", true); err != nil {
		t.Fatalf("expected admin to see the work item, got error: %v", err)
	}
}

func TestServiceChangeStatusAllowsAssigneeStartWorkAndSubmitForReview(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	// The assignee can start work (accepted -> in_progress) on their own
	// item, as a non-admin caller.
	updated, err := service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	})
	if err != nil {
		t.Fatalf("expected assignee to start work, got error: %v", err)
	}

	if updated.Status != StatusInProgress {
		t.Fatalf("expected status %q, got %q", StatusInProgress, updated.Status)
	}

	// The assignee can then submit for review.
	updated, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, ChangeStatusInput{
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
	_, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-001", false, ChangeStatusInput{
		ToStatus: StatusVerified,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected assignee verify attempt to be rejected, got %v", err)
	}
}

func TestServiceChangeStatusRejectsAssigneeActingOnUnownedWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.RespondToAssignment(context.Background(), item.ID, "user-assignee-001", true, RespondToAssignmentInput{}); err != nil {
		t.Fatalf("expected accept to succeed, got error: %v", err)
	}

	// A different assignee, not the one this item is assigned to, gets
	// ErrNotFound — same "acts as if it doesn't exist" rule as GetByID.
	_, err = service.ChangeStatus(context.Background(), item.ID, "user-assignee-999", false, ChangeStatusInput{
		ToStatus: StatusInProgress,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item, got %v", err)
	}
}

func TestServiceListStatusHistoryScopesToOwnWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.ListStatusHistory(context.Background(), item.ID, "user-assignee-001", false); err != nil {
		t.Fatalf("expected assigned user to see history, got error: %v", err)
	}

	_, err = service.ListStatusHistory(context.Background(), item.ID, "user-assignee-999", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned work item history, got %v", err)
	}
}

func TestServiceGetAssignmentScopesToOwnWorkItem(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryStore(), NewMemoryStatusHistoryStore(), NewMemoryAssignmentStore())

	item, err := service.Create(context.Background(), "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if _, err := service.AssignWorkItem(context.Background(), item.ID, "user-admin-001", AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected assign to succeed, got error: %v", err)
	}

	if _, err := service.GetAssignment(context.Background(), item.ID, "user-assignee-001", false); err != nil {
		t.Fatalf("expected assigned user to see the assignment, got error: %v", err)
	}

	_, err = service.GetAssignment(context.Background(), item.ID, "user-assignee-999", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found error for unowned assignment, got %v", err)
	}
}
