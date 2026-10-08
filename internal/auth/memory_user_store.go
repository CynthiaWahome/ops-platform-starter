package auth

import (
	"context"
	"fmt"
	"sync"
)

// MemoryUserStore is UserStore backed by an in-process slice — the
// zero-setup default, same role in this package as teams.MemoryStore plays
// in internal/teams. Before OPS-067 this didn't exist: the old
// StaticUserStore was built once from env vars at startup and never
// mutated again. This one is a real, mutable store — accounts can be
// created, listed, and updated at runtime, exactly like Postgres mode.
type MemoryUserStore struct {
	mu    sync.RWMutex
	seq   int
	users []User
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{}
}

func (s *MemoryUserStore) FindByIdentifier(_ context.Context, identifier string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := normalizeIdentifier(identifier)
	for _, user := range s.users {
		if user.Identifier == normalized {
			return user, true
		}
	}

	return User{}, false
}

func (s *MemoryUserStore) FindByID(_ context.Context, id string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.ID == id {
			return user, true
		}
	}

	return User{}, false
}

// FindByGoogleSubjectID backs OPS-068a's repeat-login path — find the
// existing account for a Google identity rather than creating a duplicate
// on every login.
func (s *MemoryUserStore) FindByGoogleSubjectID(_ context.Context, subjectID string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.GoogleSubjectID != nil && *user.GoogleSubjectID == subjectID {
			return user, true
		}
	}

	return User{}, false
}

func (s *MemoryUserStore) Create(_ context.Context, user User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := normalizeIdentifier(user.Identifier)
	for _, existing := range s.users {
		if existing.Identifier == normalized {
			return User{}, ErrIdentifierTaken
		}
		// A review caught that this store never checked GoogleSubjectID
		// uniqueness — PostgresUserStore has a real UNIQUE constraint on
		// the column, but this one had no equivalent, so two concurrent
		// first-logins for the same brand-new Google identity (both
		// passing FindByGoogleSubjectID before either Create commits)
		// could create two accounts sharing one Google subject, breaking
		// FindByGoogleSubjectID's single-result assumption.
		if existing.GoogleSubjectID != nil && user.GoogleSubjectID != nil && *existing.GoogleSubjectID == *user.GoogleSubjectID {
			return User{}, ErrIdentifierTaken
		}
	}

	s.seq++
	user.ID = fmt.Sprintf("user-%04d", s.seq)
	user.Identifier = normalized

	s.users = append(s.users, user)

	return user, nil
}

func (s *MemoryUserStore) List(_ context.Context) ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]User, len(s.users))
	copy(out, s.users)

	return out, nil
}

func (s *MemoryUserStore) Update(_ context.Context, user User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.users {
		if existing.ID == user.ID {
			s.users[i] = user
			return user, nil
		}
	}

	return User{}, ErrUserNotFound
}

// UpdateAtomic holds the store-wide lock for the entire find-mutate-write
// sequence, unlike calling FindByID and Update separately — see the
// UserStore interface doc comment for why that matters (OPS-068b's OTP
// consumption race).
func (s *MemoryUserStore) UpdateAtomic(_ context.Context, id string, mutate func(User) (User, error)) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.users {
		if existing.ID == id {
			updated, err := mutate(existing)
			if err != nil {
				return User{}, err
			}

			s.users[i] = updated

			return updated, nil
		}
	}

	return User{}, ErrUserNotFound
}

func (s *MemoryUserStore) Seed(_ context.Context, user User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.users {
		if existing.ID == user.ID {
			return nil
		}
	}

	user.Identifier = normalizeIdentifier(user.Identifier)
	s.users = append(s.users, user)

	return nil
}
