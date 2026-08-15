package notifications

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var ErrNotFound = errors.New("notification not found")

// Store persists Notifications. Unlike attachments.Store or
// workitems.StatusHistoryStore, this has an Update method — a
// notification's ReadAt field changes once, in place, after creation.
type Store interface {
	Create(ctx context.Context, notification Notification) (Notification, error)
	GetByID(ctx context.Context, id string) (Notification, error)
	Update(ctx context.Context, notification Notification) (Notification, error)
	ListByRecipientUserID(ctx context.Context, recipientUserID string, onlyUnread bool) ([]Notification, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	seq     int
	entries map[string]Notification
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]Notification),
	}
}

func (s *MemoryStore) Create(_ context.Context, notification Notification) (Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	notification.ID = fmt.Sprintf("notification-%04d", s.seq)

	s.entries[notification.ID] = notification

	return notification, nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	notification, ok := s.entries[id]
	if !ok {
		return Notification{}, ErrNotFound
	}

	return notification, nil
}

func (s *MemoryStore) Update(_ context.Context, notification Notification) (Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.entries[notification.ID]; !ok {
		return Notification{}, ErrNotFound
	}

	s.entries[notification.ID] = notification

	return notification, nil
}

func (s *MemoryStore) ListByRecipientUserID(_ context.Context, recipientUserID string, onlyUnread bool) ([]Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]Notification, 0)
	for _, entry := range s.entries {
		if entry.RecipientUserID != recipientUserID {
			continue
		}
		if onlyUnread && entry.ReadAt != nil {
			continue
		}
		matches = append(matches, entry)
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].CreatedAt.Before(matches[j].CreatedAt)
	})

	return matches, nil
}
