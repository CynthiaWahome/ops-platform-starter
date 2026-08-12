package workitems

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// AssignmentHistoryStore is append-only, the same contract as
// StatusHistoryStore: an entry is never edited or removed once written,
// so there is no Update or Delete method here, unlike AssignmentStore.
type AssignmentHistoryStore interface {
	Create(ctx context.Context, entry AssignmentHistory) (AssignmentHistory, error)
	ListByWorkItemID(ctx context.Context, workItemID string) ([]AssignmentHistory, error)
}

type MemoryAssignmentHistoryStore struct {
	mu      sync.RWMutex
	seq     int
	entries []AssignmentHistory
}

func NewMemoryAssignmentHistoryStore() *MemoryAssignmentHistoryStore {
	return &MemoryAssignmentHistoryStore{}
}

func (s *MemoryAssignmentHistoryStore) Create(_ context.Context, entry AssignmentHistory) (AssignmentHistory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	entry.ID = fmt.Sprintf("assignmenthistory-%04d", s.seq)

	s.entries = append(s.entries, cloneAssignmentHistory(entry))

	return cloneAssignmentHistory(entry), nil
}

func (s *MemoryAssignmentHistoryStore) ListByWorkItemID(_ context.Context, workItemID string) ([]AssignmentHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]AssignmentHistory, 0)
	for _, entry := range s.entries {
		if entry.WorkItemID == workItemID {
			matches = append(matches, cloneAssignmentHistory(entry))
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].CreatedAt.Before(matches[j].CreatedAt)
	})

	return matches, nil
}

func cloneAssignmentHistory(entry AssignmentHistory) AssignmentHistory {
	cloned := entry

	if entry.Note != nil {
		note := *entry.Note
		cloned.Note = &note
	}

	return cloned
}
