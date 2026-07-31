package workitems

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Store interface {
	Create(ctx context.Context, item WorkItem) (WorkItem, error)
	List(ctx context.Context) ([]WorkItem, error)
	GetByID(ctx context.Context, id string) (WorkItem, error)
	Update(ctx context.Context, item WorkItem) (WorkItem, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	seq     int
	byID    map[string]WorkItem
	ordered []string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID: make(map[string]WorkItem),
	}
}

func (s *MemoryStore) Create(_ context.Context, item WorkItem) (WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	item.ID = fmt.Sprintf("workitem-%04d", s.seq)
	item.ReferenceCode = fmt.Sprintf("WI-%04d", s.seq)

	s.byID[item.ID] = item
	s.ordered = append(s.ordered, item.ID)

	return cloneWorkItem(item), nil
}

func (s *MemoryStore) List(_ context.Context) ([]WorkItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]WorkItem, 0, len(s.ordered))
	for _, id := range s.ordered {
		items = append(items, cloneWorkItem(s.byID[id]))
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	return items, nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (WorkItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.byID[id]
	if !ok {
		return WorkItem{}, ErrNotFound
	}

	return cloneWorkItem(item), nil
}

func (s *MemoryStore) Update(_ context.Context, item WorkItem) (WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[item.ID]; !ok {
		return WorkItem{}, ErrNotFound
	}

	s.byID[item.ID] = item

	return cloneWorkItem(item), nil
}

func cloneWorkItem(item WorkItem) WorkItem {
	cloned := item

	if item.AssignedToUserID != nil {
		assignedTo := *item.AssignedToUserID
		cloned.AssignedToUserID = &assignedTo
	}

	if item.LocationText != nil {
		location := *item.LocationText
		cloned.LocationText = &location
	}

	if item.DueAt != nil {
		cloned.DueAt = ptrTime(*item.DueAt)
	}

	return cloned
}

func ptrTime(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
