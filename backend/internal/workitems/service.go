package workitems

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) Service {
	return Service{
		store: store,
		now:   time.Now,
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

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
