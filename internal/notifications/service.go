package notifications

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidInput = errors.New("invalid notification input")

// ErrNotOwned mirrors the "acts as if it doesn't exist" shape workitems
// already uses for cross-user access (see workitems.Service.GetByID):
// MarkAsRead maps this to a 404, not a 403, so a caller can't learn
// whether a given notification id belongs to someone else.
var ErrNotOwned = errors.New("notification does not belong to this user")

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) Service {
	return Service{store: store, now: time.Now}
}

// Notify raises one notification as a side effect of a workflow event.
// It satisfies workitems.NotificationSink by signature — kind is a plain
// string here rather than the Kind type, so workitems.Service can call
// this without importing this package at all, only the interface it
// defines for itself. That's the point: workitems depends on a small
// interface it owns, not on this package concretely.
func (s Service) Notify(ctx context.Context, recipientUserID, workItemID, kind, message string) error {
	recipientUserID = strings.TrimSpace(recipientUserID)
	workItemID = strings.TrimSpace(workItemID)
	message = strings.TrimSpace(message)

	if recipientUserID == "" || workItemID == "" || kind == "" || message == "" {
		return ErrInvalidInput
	}

	_, err := s.store.Create(ctx, Notification{
		RecipientUserID: recipientUserID,
		WorkItemID:      workItemID,
		Kind:            Kind(kind),
		Message:         message,
		CreatedAt:       s.now(),
	})

	return err
}

// List returns a recipient's own notifications, newest last (oldest
// first) — same ordering convention as StatusHistory/AssignmentHistory.
// Set onlyUnread to filter to notifications with no ReadAt yet.
func (s Service) List(ctx context.Context, recipientUserID string, onlyUnread bool) ([]Notification, error) {
	if strings.TrimSpace(recipientUserID) == "" {
		return nil, ErrInvalidInput
	}

	return s.store.ListByRecipientUserID(ctx, recipientUserID, onlyUnread)
}

// MarkAsRead sets ReadAt on one notification. Marking an already-read
// notification as read again is a no-op success, not an error — there is
// nothing exceptional about a client retrying or double-clicking.
func (s Service) MarkAsRead(ctx context.Context, id string, recipientUserID string) (Notification, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(recipientUserID) == "" {
		return Notification{}, ErrInvalidInput
	}

	notification, err := s.store.GetByID(ctx, id)
	if err != nil {
		return Notification{}, err
	}

	if notification.RecipientUserID != recipientUserID {
		return Notification{}, ErrNotOwned
	}

	if notification.ReadAt == nil {
		now := s.now()
		notification.ReadAt = &now

		return s.store.Update(ctx, notification)
	}

	return notification, nil
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}
