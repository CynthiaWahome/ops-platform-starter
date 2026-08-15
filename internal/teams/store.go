package teams

import (
	"context"
	"fmt"
	"sync"
)

type Store interface {
	Create(ctx context.Context, team Team) (Team, error)
	GetByID(ctx context.Context, id string) (Team, error)
	List(ctx context.Context) ([]Team, error)
}

type MembershipStore interface {
	Create(ctx context.Context, membership Membership) (Membership, error)
	// Update persists an in-place edit — used to set RemovedAt when a
	// membership is closed (moved off the team). Unlike
	// workitems.AssignmentHistoryStore this is not append-only: a row's
	// RemovedAt is the one field that legitimately mutates after creation.
	Update(ctx context.Context, membership Membership) (Membership, error)
	// GetActiveByUserID returns the membership row with RemovedAt == nil
	// for this user, if any. An assignee has at most one at a time.
	GetActiveByUserID(ctx context.Context, userID string) (Membership, bool, error)
	// ListActiveByTeamID returns every assignee currently on a team.
	ListActiveByTeamID(ctx context.Context, teamID string) ([]Membership, error)
}

type SupervisionStore interface {
	Create(ctx context.Context, supervision Supervision) (Supervision, error)
	Update(ctx context.Context, supervision Supervision) (Supervision, error)
	// ListActiveByTeamID returns every currently-active supervisor row for
	// a team — unlike membership, more than one can be active at once.
	ListActiveByTeamID(ctx context.Context, teamID string) ([]Supervision, error)
	// ListActiveByUserID returns every team this user currently supervises.
	ListActiveByUserID(ctx context.Context, userID string) ([]Supervision, error)
}

type MemoryStore struct {
	mu    sync.RWMutex
	seq   int
	teams []Team
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Create(_ context.Context, team Team) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	team.ID = fmt.Sprintf("team-%04d", s.seq)

	s.teams = append(s.teams, team)

	return team, nil
}

func (s *MemoryStore) GetByID(_ context.Context, id string) (Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, team := range s.teams {
		if team.ID == id {
			return team, nil
		}
	}

	return Team{}, ErrNotFound
}

func (s *MemoryStore) List(_ context.Context) ([]Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Team, len(s.teams))
	copy(out, s.teams)

	return out, nil
}

type MemoryMembershipStore struct {
	mu      sync.RWMutex
	seq     int
	entries []Membership
}

func NewMemoryMembershipStore() *MemoryMembershipStore {
	return &MemoryMembershipStore{}
}

func (s *MemoryMembershipStore) Create(_ context.Context, membership Membership) (Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	membership.ID = fmt.Sprintf("teammembership-%04d", s.seq)

	s.entries = append(s.entries, membership)

	return membership, nil
}

func (s *MemoryMembershipStore) Update(_ context.Context, membership Membership) (Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, entry := range s.entries {
		if entry.ID == membership.ID {
			s.entries[i] = membership
			return membership, nil
		}
	}

	return Membership{}, ErrNotFound
}

func (s *MemoryMembershipStore) GetActiveByUserID(_ context.Context, userID string) (Membership, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, entry := range s.entries {
		if entry.UserID == userID && entry.RemovedAt == nil {
			return entry, true, nil
		}
	}

	return Membership{}, false, nil
}

func (s *MemoryMembershipStore) ListActiveByTeamID(_ context.Context, teamID string) ([]Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]Membership, 0)
	for _, entry := range s.entries {
		if entry.TeamID == teamID && entry.RemovedAt == nil {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

type MemorySupervisionStore struct {
	mu      sync.RWMutex
	seq     int
	entries []Supervision
}

func NewMemorySupervisionStore() *MemorySupervisionStore {
	return &MemorySupervisionStore{}
}

func (s *MemorySupervisionStore) Create(_ context.Context, supervision Supervision) (Supervision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	supervision.ID = fmt.Sprintf("teamsupervision-%04d", s.seq)

	s.entries = append(s.entries, supervision)

	return supervision, nil
}

func (s *MemorySupervisionStore) Update(_ context.Context, supervision Supervision) (Supervision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, entry := range s.entries {
		if entry.ID == supervision.ID {
			s.entries[i] = supervision
			return supervision, nil
		}
	}

	return Supervision{}, ErrNotFound
}

func (s *MemorySupervisionStore) ListActiveByTeamID(_ context.Context, teamID string) ([]Supervision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]Supervision, 0)
	for _, entry := range s.entries {
		if entry.TeamID == teamID && entry.RemovedAt == nil {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}

func (s *MemorySupervisionStore) ListActiveByUserID(_ context.Context, userID string) ([]Supervision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]Supervision, 0)
	for _, entry := range s.entries {
		if entry.UserID == userID && entry.RemovedAt == nil {
			matches = append(matches, entry)
		}
	}

	return matches, nil
}
