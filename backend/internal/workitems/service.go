package workitems

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	store           Store
	historyStore    StatusHistoryStore
	assignmentStore AssignmentStore
	now             func() time.Time
}

func NewService(store Store, historyStore StatusHistoryStore, assignmentStore AssignmentStore) Service {
	return Service{
		store:           store,
		historyStore:    historyStore,
		assignmentStore: assignmentStore,
		now:             time.Now,
	}
}

func (s Service) Create(ctx context.Context, createdByUserID string, input CreateInput) (WorkItem, error) {
	if strings.TrimSpace(createdByUserID) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	title := strings.TrimSpace(input.Title)
	description := strings.TrimSpace(input.Description)

	if title == "" || description == "" {
		return WorkItem{}, ErrInvalidInput
	}

	if !input.Priority.IsValid() {
		return WorkItem{}, ErrInvalidPriority
	}

	now := s.now()

	item := WorkItem{
		Title:           title,
		Description:     description,
		Status:          StatusCreated,
		Priority:        input.Priority,
		CreatedByUserID: createdByUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if input.LocationText != nil {
		location := strings.TrimSpace(*input.LocationText)
		if location != "" {
			item.LocationText = &location
		}
	}

	if input.DueAt != nil {
		item.DueAt = ptrTime(*input.DueAt)
	}

	return s.store.Create(ctx, item)
}

// List returns work items scoped to the caller. An admin sees every work
// item. A non-admin caller only sees work items currently assigned to
// them — the "assignee sees own assigned work only" rule from the
// starter's permission matrix. The workitems package deliberately doesn't
// import the auth package, so the caller (the HTTP handler, which does
// know about roles) resolves "is this an admin" into a plain bool before
// calling this method.
func (s Service) List(ctx context.Context, callerUserID string, callerIsAdmin bool) ([]WorkItem, error) {
	if callerIsAdmin {
		return s.store.List(ctx)
	}

	if strings.TrimSpace(callerUserID) == "" {
		return nil, ErrInvalidInput
	}

	return s.store.ListByAssignedToUserID(ctx, callerUserID)
}

// GetByID returns one work item, scoped the same way as List. A non-admin
// caller asking for a work item that isn't assigned to them gets
// ErrNotFound, not a "forbidden" error — from their point of view the work
// item does not exist, rather than existing-but-hidden. This avoids
// revealing that a given id belongs to someone else.
func (s Service) GetByID(ctx context.Context, id string, callerUserID string, callerIsAdmin bool) (WorkItem, error) {
	if strings.TrimSpace(id) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}

	if callerIsAdmin {
		return item, nil
	}

	if item.AssignedToUserID == nil || *item.AssignedToUserID != callerUserID {
		return WorkItem{}, ErrNotFound
	}

	return item, nil
}

func (s Service) Update(ctx context.Context, id string, input UpdateInput) (WorkItem, error) {
	if strings.TrimSpace(id) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return WorkItem{}, ErrInvalidInput
		}

		item.Title = title
	}

	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if description == "" {
			return WorkItem{}, ErrInvalidInput
		}

		item.Description = description
	}

	if input.Priority != nil {
		if !input.Priority.IsValid() {
			return WorkItem{}, ErrInvalidPriority
		}

		item.Priority = *input.Priority
	}

	if input.LocationText != nil {
		location := strings.TrimSpace(*input.LocationText)
		if location == "" {
			item.LocationText = nil
		} else {
			item.LocationText = &location
		}
	}

	if input.DueAt != nil {
		item.DueAt = ptrTime(*input.DueAt)
	}

	item.UpdatedAt = s.now()

	return s.store.Update(ctx, item)
}

// ChangeStatus moves a work item to a new status, checks the move is legal,
// saves the new status on the work item, and writes a StatusHistory entry
// recording what changed, who changed it, and why.
func (s Service) ChangeStatus(ctx context.Context, id string, actorUserID string, input ChangeStatusInput) (WorkItem, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(actorUserID) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	if !input.ToStatus.IsValid() {
		return WorkItem{}, ErrInvalidStatus
	}

	item, err := s.store.GetByID(ctx, id)
	if err != nil {
		return WorkItem{}, err
	}

	if !IsValidTransition(item.Status, input.ToStatus) {
		return WorkItem{}, ErrInvalidTransition
	}

	fromStatus := item.Status
	now := s.now()

	item.Status = input.ToStatus
	item.UpdatedAt = now

	updated, err := s.store.Update(ctx, item)
	if err != nil {
		return WorkItem{}, err
	}

	if err := s.recordStatusChange(ctx, id, fromStatus, input.ToStatus, actorUserID, input.Reason, now); err != nil {
		return WorkItem{}, err
	}

	return updated, nil
}

// recordStatusChange writes one StatusHistory entry. It is the single place
// that turns a status move into a history row, so ChangeStatus and
// AssignWorkItem (which also moves a status, from Created to Assigned)
// don't each keep their own copy of this logic.
func (s Service) recordStatusChange(ctx context.Context, workItemID string, from, to Status, actorUserID string, rawReason *string, at time.Time) error {
	var reason *string
	if rawReason != nil {
		trimmed := strings.TrimSpace(*rawReason)
		if trimmed != "" {
			reason = &trimmed
		}
	}

	_, err := s.historyStore.Create(ctx, StatusHistory{
		WorkItemID:      workItemID,
		FromStatus:      &from,
		ToStatus:        to,
		ChangedByUserID: actorUserID,
		Reason:          reason,
		CreatedAt:       at,
	})

	return err
}

// ListStatusHistory returns every status change recorded for a work item,
// oldest first. It confirms the work item exists before checking history so
// callers get a consistent ErrNotFound instead of an empty list for a
// missing id.
func (s Service) ListStatusHistory(ctx context.Context, workItemID string) ([]StatusHistory, error) {
	if strings.TrimSpace(workItemID) == "" {
		return nil, ErrInvalidInput
	}

	if _, err := s.store.GetByID(ctx, workItemID); err != nil {
		return nil, err
	}

	return s.historyStore.ListByWorkItemID(ctx, workItemID)
}

// AssignWorkItem gives a work item to an assignee. It reuses the same
// status-transition rules as ChangeStatus: a work item can only move into
// StatusAssigned from a status that already allows it (StatusCreated), so
// a work item that has already been assigned, or moved further along, is
// rejected by the same IsValidTransition check rather than a new rule.
func (s Service) AssignWorkItem(ctx context.Context, workItemID string, assignedByUserID string, input AssignInput) (Assignment, error) {
	assignedToUserID := strings.TrimSpace(input.AssignedToUserID)

	if strings.TrimSpace(workItemID) == "" || strings.TrimSpace(assignedByUserID) == "" || assignedToUserID == "" {
		return Assignment{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, workItemID)
	if err != nil {
		return Assignment{}, err
	}

	if !IsValidTransition(item.Status, StatusAssigned) {
		return Assignment{}, ErrInvalidTransition
	}

	fromStatus := item.Status
	now := s.now()

	item.Status = StatusAssigned
	item.AssignedToUserID = &assignedToUserID
	item.UpdatedAt = now

	if _, err := s.store.Update(ctx, item); err != nil {
		return Assignment{}, err
	}

	if err := s.recordStatusChange(ctx, workItemID, fromStatus, StatusAssigned, assignedByUserID, nil, now); err != nil {
		return Assignment{}, err
	}

	return s.assignmentStore.Create(ctx, Assignment{
		WorkItemID:       workItemID,
		AssignedByUserID: assignedByUserID,
		AssignedToUserID: assignedToUserID,
		Status:           AssignmentStatusAssigned,
		AssignedAt:       now,
	})
}

// GetAssignment returns the current assignment for a work item. It confirms
// the work item exists first, so a bad work item id and a work item that
// has never been assigned come back as two distinguishable errors instead
// of both looking like "not found" for the same reason.
func (s Service) GetAssignment(ctx context.Context, workItemID string) (Assignment, error) {
	if strings.TrimSpace(workItemID) == "" {
		return Assignment{}, ErrInvalidInput
	}

	if _, err := s.store.GetByID(ctx, workItemID); err != nil {
		return Assignment{}, err
	}

	return s.assignmentStore.GetByWorkItemID(ctx, workItemID)
}

// RespondToAssignment lets the assignee a work item is assigned to accept
// or decline it. accept controls which path is taken:
//
//   - accept: work item moves Assigned -> Accepted, assignment recorded as
//     accepted
//   - decline: work item moves Assigned -> Created (unassigned again, so
//     an admin can reassign it), assignment recorded as declined
//
// respondingUserID must match the assignment's AssignedToUserID — this is
// the ownership check that role middleware alone cannot make, since
// middleware only knows the caller's role, not which specific work item
// they were assigned.
func (s Service) RespondToAssignment(ctx context.Context, workItemID string, respondingUserID string, accept bool, input RespondToAssignmentInput) (Assignment, error) {
	if strings.TrimSpace(workItemID) == "" || strings.TrimSpace(respondingUserID) == "" {
		return Assignment{}, ErrInvalidInput
	}

	item, err := s.store.GetByID(ctx, workItemID)
	if err != nil {
		return Assignment{}, err
	}

	assignment, err := s.assignmentStore.GetByWorkItemID(ctx, workItemID)
	if err != nil {
		return Assignment{}, err
	}

	if assignment.AssignedToUserID != respondingUserID {
		return Assignment{}, ErrAssignmentNotOwned
	}

	if assignment.Status != AssignmentStatusAssigned {
		return Assignment{}, ErrAssignmentNotPending
	}

	toStatus := StatusCreated
	assignmentStatus := AssignmentStatusDeclined
	if accept {
		toStatus = StatusAccepted
		assignmentStatus = AssignmentStatusAccepted
	}

	if !IsValidTransition(item.Status, toStatus) {
		return Assignment{}, ErrInvalidTransition
	}

	fromStatus := item.Status
	now := s.now()

	item.Status = toStatus
	item.UpdatedAt = now

	if !accept {
		item.AssignedToUserID = nil
	}

	if _, err := s.store.Update(ctx, item); err != nil {
		return Assignment{}, err
	}

	if err := s.recordStatusChange(ctx, workItemID, fromStatus, toStatus, respondingUserID, input.Note, now); err != nil {
		return Assignment{}, err
	}

	var note *string
	if input.Note != nil {
		trimmed := strings.TrimSpace(*input.Note)
		if trimmed != "" {
			note = &trimmed
		}
	}

	assignment.Status = assignmentStatus
	assignment.RespondedAt = ptrTime(now)
	assignment.ResponseNote = note

	return s.assignmentStore.Update(ctx, assignment)
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
