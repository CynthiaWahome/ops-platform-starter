package workitems

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	store        Store
	historyStore StatusHistoryStore
	now          func() time.Time
}

func NewService(store Store, historyStore StatusHistoryStore) Service {
	return Service{
		store:        store,
		historyStore: historyStore,
		now:          time.Now,
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

func (s Service) List(ctx context.Context) ([]WorkItem, error) {
	return s.store.List(ctx)
}

func (s Service) GetByID(ctx context.Context, id string) (WorkItem, error) {
	if strings.TrimSpace(id) == "" {
		return WorkItem{}, ErrInvalidInput
	}

	return s.store.GetByID(ctx, id)
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

	var reason *string
	if input.Reason != nil {
		trimmed := strings.TrimSpace(*input.Reason)
		if trimmed != "" {
			reason = &trimmed
		}
	}

	_, err = s.historyStore.Create(ctx, StatusHistory{
		WorkItemID:      id,
		FromStatus:      &fromStatus,
		ToStatus:        input.ToStatus,
		ChangedByUserID: actorUserID,
		Reason:          reason,
		CreatedAt:       now,
	})
	if err != nil {
		return WorkItem{}, err
	}

	return updated, nil
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

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
