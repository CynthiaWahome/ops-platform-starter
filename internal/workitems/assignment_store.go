package workitems

import (
	"context"
	"fmt"
	"sync"
)

// AssignmentStore keeps one active assignment per work item. Unlike
// StatusHistoryStore, entries here are expected to be updated in place —
// OPS-023 will set RespondedAt and ResponseNote on the same row when an
// assignee accepts or declines, rather than writing a new row.
type AssignmentStore interface {
	Create(ctx context.Context, assignment Assignment) (Assignment, error)
	GetByWorkItemID(ctx context.Context, workItemID string) (Assignment, error)
	Update(ctx context.Context, assignment Assignment) (Assignment, error)
}

type MemoryAssignmentStore struct {
	mu         sync.RWMutex
	seq        int
	byWorkItem map[string]Assignment
}

func NewMemoryAssignmentStore() *MemoryAssignmentStore {
	return &MemoryAssignmentStore{
		byWorkItem: make(map[string]Assignment),
	}
}

func (s *MemoryAssignmentStore) Create(_ context.Context, assignment Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	assignment.ID = fmt.Sprintf("assignment-%04d", s.seq)

	s.byWorkItem[assignment.WorkItemID] = assignment

	return cloneAssignment(assignment), nil
}

func (s *MemoryAssignmentStore) GetByWorkItemID(_ context.Context, workItemID string) (Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assignment, ok := s.byWorkItem[workItemID]
	if !ok {
		return Assignment{}, ErrAssignmentNotFound
	}

	return cloneAssignment(assignment), nil
}

func (s *MemoryAssignmentStore) Update(_ context.Context, assignment Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byWorkItem[assignment.WorkItemID]; !ok {
		return Assignment{}, ErrAssignmentNotFound
	}

	s.byWorkItem[assignment.WorkItemID] = assignment

	return cloneAssignment(assignment), nil
}

func cloneAssignment(assignment Assignment) Assignment {
	cloned := assignment

	if assignment.RespondedAt != nil {
		cloned.RespondedAt = ptrTime(*assignment.RespondedAt)
	}

	if assignment.ResponseNote != nil {
		note := *assignment.ResponseNote
		cloned.ResponseNote = &note
	}

	return cloned
}
