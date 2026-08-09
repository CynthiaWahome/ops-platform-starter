package attachments

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Store persists Attachment metadata — the record of what was uploaded,
// not the file bytes themselves (that's Storage's job). Attachments are
// never edited or removed once created, so this is append-plus-list only,
// same shape as workitems.StatusHistoryStore.
type Store interface {
	Create(ctx context.Context, attachment Attachment) (Attachment, error)
	ListByWorkItemID(ctx context.Context, workItemID string) ([]Attachment, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	seq     int
	entries []Attachment
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Create(_ context.Context, attachment Attachment) (Attachment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	attachment.ID = fmt.Sprintf("attachment-%04d", s.seq)

	s.entries = append(s.entries, attachment)

	return attachment, nil
}

func (s *MemoryStore) ListByWorkItemID(_ context.Context, workItemID string) ([]Attachment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]Attachment, 0)
	for _, entry := range s.entries {
		if entry.WorkItemID == workItemID {
			matches = append(matches, entry)
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].CreatedAt.Before(matches[j].CreatedAt)
	})

	return matches, nil
}
