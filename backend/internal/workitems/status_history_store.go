package workitems

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// StatusHistoryStore is append-only: a history entry is never edited or
// removed once written, so there is no Update or Delete method here,
// unlike Store.
type StatusHistoryStore interface {
	Create(ctx context.Context, entry StatusHistory) (StatusHistory, error)
	ListByWorkItemID(ctx context.Context, workItemID string) ([]StatusHistory, error)
}

type MemoryStatusHistoryStore struct {
	mu      sync.RWMutex
	seq     int
	entries []StatusHistory
}

func NewMemoryStatusHistoryStore() *MemoryStatusHistoryStore {
	return &MemoryStatusHistoryStore{}
}

func (s *MemoryStatusHistoryStore) Create(_ context.Context, entry StatusHistory) (StatusHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	entry.ID = fmt.Sprintf("statushistory-%04d", s.seq)

	s.entries = append(s.entries, cloneStatusHistory(entry))

	return cloneStatusHistory(entry), nil
}

func (s *MemoryStatusHistoryStore) ListByWorkItemID(_ context.Context, workItemID string) ([]StatusHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]StatusHistory, 0)
	for _, entry := range s.entries {
		if entry.WorkItemID == workItemID {
			matches = append(matches, cloneStatusHistory(entry))
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].CreatedAt.Before(matches[j].CreatedAt)
	})

	return matches, nil
}

func cloneStatusHistory(entry StatusHistory) StatusHistory {
	cloned := entry

	if entry.FromStatus != nil {
		from := *entry.FromStatus
		cloned.FromStatus = &from
	}

	if entry.Reason != nil {
		reason := *entry.Reason
		cloned.Reason = &reason
	}

	return cloned
}
