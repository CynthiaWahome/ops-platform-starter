package teams

import (
	"context"
	"strings"
	"time"
)

type Service struct {
	teamStore        Store
	membershipStore  MembershipStore
	supervisionStore SupervisionStore
	now              func() time.Time
}

func NewService(teamStore Store, membershipStore MembershipStore, supervisionStore SupervisionStore) Service {
	return Service{
		teamStore:        teamStore,
		membershipStore:  membershipStore,
		supervisionStore: supervisionStore,
		now:              time.Now,
	}
}

func (s Service) CreateTeam(ctx context.Context, name string) (Team, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return Team{}, ErrInvalidInput
	}

	return s.teamStore.Create(ctx, Team{Name: trimmed, CreatedAt: s.now()})
}

func (s Service) ListTeams(ctx context.Context) ([]Team, error) {
	return s.teamStore.List(ctx)
}

// AddAssignee puts an assignee on a team, admin-only in practice (enforced
// at the route layer, same as every other admin-gated action in this
// codebase). If the assignee already has an active membership elsewhere,
// that membership is closed first — this is the "move" operation: an
// assignee always has exactly one active team, never zero once assigned and
// never two, so moving is close-old-then-open-new in the same call rather
// than a separate step callers could forget.
func (s Service) AddAssignee(ctx context.Context, teamID, userID, addedByUserID string) (Membership, error) {
	teamID = strings.TrimSpace(teamID)
	userID = strings.TrimSpace(userID)
	addedByUserID = strings.TrimSpace(addedByUserID)

	if teamID == "" || userID == "" || addedByUserID == "" {
		return Membership{}, ErrInvalidInput
	}

	if _, err := s.teamStore.GetByID(ctx, teamID); err != nil {
		return Membership{}, err
	}

	now := s.now()

	if existing, ok, err := s.membershipStore.GetActiveByUserID(ctx, userID); err != nil {
		return Membership{}, err
	} else if ok {
		if existing.TeamID == teamID {
			// Already on this team — nothing to move.
			return existing, nil
		}

		existing.RemovedAt = ptrTime(now)
		if _, err := s.membershipStore.Update(ctx, existing); err != nil {
			return Membership{}, err
		}
	}

	return s.membershipStore.Create(ctx, Membership{
		TeamID:        teamID,
		UserID:        userID,
		AddedByUserID: addedByUserID,
		AddedAt:       now,
	})
}

// AddSupervisor grants a user active supervision of a team, admin-only in
// practice. Unlike AddAssignee this never closes an existing row first — a
// team can have several concurrently-active supervisors (co-supervision),
// so adding one is purely additive.
func (s Service) AddSupervisor(ctx context.Context, teamID, userID, addedByUserID string) (Supervision, error) {
	teamID = strings.TrimSpace(teamID)
	userID = strings.TrimSpace(userID)
	addedByUserID = strings.TrimSpace(addedByUserID)

	if teamID == "" || userID == "" || addedByUserID == "" {
		return Supervision{}, ErrInvalidInput
	}

	if _, err := s.teamStore.GetByID(ctx, teamID); err != nil {
		return Supervision{}, err
	}

	return s.supervisionStore.Create(ctx, Supervision{
		TeamID:        teamID,
		UserID:        userID,
		AddedByUserID: addedByUserID,
		AddedAt:       s.now(),
	})
}

// RemoveSupervisor revokes one user's active supervision of one team,
// admin-only in practice. Closing the row (not deleting it) keeps the
// "who supervised this team, and when" trail intact.
func (s Service) RemoveSupervisor(ctx context.Context, teamID, userID string) error {
	teamID = strings.TrimSpace(teamID)
	userID = strings.TrimSpace(userID)

	active, err := s.supervisionStore.ListActiveByTeamID(ctx, teamID)
	if err != nil {
		return err
	}

	now := s.now()
	found := false

	for _, supervision := range active {
		if supervision.UserID != userID {
			continue
		}

		found = true
		supervision.RemovedAt = ptrTime(now)
		if _, err := s.supervisionStore.Update(ctx, supervision); err != nil {
			return err
		}
	}

	if !found {
		return ErrNotSupervisor
	}

	return nil
}

// IsActiveSupervisorOf is the one check every team-scoped workitems action
// (assign, verify, flag, complete) reuses: is supervisorUserID currently an
// active supervisor of whatever team assigneeUserID currently belongs to.
// If the assignee has no active team at all, this is false — there is
// nothing to be a supervisor of.
func (s Service) IsActiveSupervisorOf(ctx context.Context, supervisorUserID, assigneeUserID string) (bool, error) {
	supervisorUserID = strings.TrimSpace(supervisorUserID)
	assigneeUserID = strings.TrimSpace(assigneeUserID)

	if supervisorUserID == "" || assigneeUserID == "" {
		return false, nil
	}

	membership, ok, err := s.membershipStore.GetActiveByUserID(ctx, assigneeUserID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	supervisions, err := s.supervisionStore.ListActiveByTeamID(ctx, membership.TeamID)
	if err != nil {
		return false, err
	}

	for _, supervision := range supervisions {
		if supervision.UserID == supervisorUserID {
			return true, nil
		}
	}

	return false, nil
}

// SupervisedAssigneeUserIDs returns every assignee currently on any team
// this user actively supervises — used to scope a supervisor's "list my
// team's work items" view the same way admin sees everything and a plain
// assignee sees only their own assigned work.
func (s Service) SupervisedAssigneeUserIDs(ctx context.Context, supervisorUserID string) ([]string, error) {
	supervisorUserID = strings.TrimSpace(supervisorUserID)
	if supervisorUserID == "" {
		return nil, nil
	}

	supervisions, err := s.supervisionStore.ListActiveByUserID(ctx, supervisorUserID)
	if err != nil {
		return nil, err
	}

	userIDs := make([]string, 0)
	seen := make(map[string]bool)

	for _, supervision := range supervisions {
		members, err := s.membershipStore.ListActiveByTeamID(ctx, supervision.TeamID)
		if err != nil {
			return nil, err
		}

		for _, member := range members {
			if seen[member.UserID] {
				continue
			}

			seen[member.UserID] = true
			userIDs = append(userIDs, member.UserID)
		}
	}

	return userIDs, nil
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
