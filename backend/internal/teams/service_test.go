package teams

import (
	"context"
	"testing"
)

func newTestService() Service {
	return NewService(NewMemoryStore(), NewMemoryMembershipStore(), NewMemorySupervisionStore())
}

func TestAddAssigneeMovesBetweenTeams(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTestService()

	teamA, err := svc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}

	teamB, err := svc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := svc.AddAssignee(ctx, teamA.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team A, got error: %v", err)
	}

	if _, err := svc.AddAssignee(ctx, teamB.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to move to team B, got error: %v", err)
	}

	membership, ok, err := svc.membershipStore.GetActiveByUserID(ctx, "user-assignee-001")
	if err != nil {
		t.Fatalf("expected active membership lookup to succeed, got error: %v", err)
	}
	if !ok {
		t.Fatal("expected an active membership after the move")
	}
	if membership.TeamID != teamB.ID {
		t.Fatalf("expected active team to be team B, got %s", membership.TeamID)
	}
}

func TestTeamCanHaveMultipleActiveSupervisors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTestService()

	team, err := svc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}

	if _, err := svc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}

	if _, err := svc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor 1 to be added, got error: %v", err)
	}

	if _, err := svc.AddSupervisor(ctx, team.ID, "user-supervisor-002", "user-admin-001"); err != nil {
		t.Fatalf("expected co-supervisor to be added, got error: %v", err)
	}

	for _, supervisorID := range []string{"user-supervisor-001", "user-supervisor-002"} {
		ok, err := svc.IsActiveSupervisorOf(ctx, supervisorID, "user-assignee-001")
		if err != nil {
			t.Fatalf("expected supervision check to succeed for %s, got error: %v", supervisorID, err)
		}
		if !ok {
			t.Fatalf("expected %s to be an active supervisor of the assignee's team", supervisorID)
		}
	}
}

func TestSupervisorHasNoAuthorityOverAnotherTeam(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTestService()

	teamA, err := svc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team A to create, got error: %v", err)
	}

	teamB, err := svc.CreateTeam(ctx, "Team B")
	if err != nil {
		t.Fatalf("expected team B to create, got error: %v", err)
	}

	if _, err := svc.AddAssignee(ctx, teamA.ID, "user-assignee-a", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team A, got error: %v", err)
	}
	if _, err := svc.AddAssignee(ctx, teamB.ID, "user-assignee-b", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team B, got error: %v", err)
	}

	if _, err := svc.AddSupervisor(ctx, teamA.ID, "user-supervisor-a", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added to team A, got error: %v", err)
	}

	ok, err := svc.IsActiveSupervisorOf(ctx, "user-supervisor-a", "user-assignee-b")
	if err != nil {
		t.Fatalf("expected supervision check to succeed, got error: %v", err)
	}
	if ok {
		t.Fatal("expected team A's supervisor to have no authority over team B's assignee")
	}
}

func TestRemoveSupervisorRevokesButKeepsHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTestService()

	team, err := svc.CreateTeam(ctx, "Team A")
	if err != nil {
		t.Fatalf("expected team to create, got error: %v", err)
	}

	if _, err := svc.AddAssignee(ctx, team.ID, "user-assignee-001", "user-admin-001"); err != nil {
		t.Fatalf("expected assignee to join team, got error: %v", err)
	}

	if _, err := svc.AddSupervisor(ctx, team.ID, "user-supervisor-001", "user-admin-001"); err != nil {
		t.Fatalf("expected supervisor to be added, got error: %v", err)
	}

	if err := svc.RemoveSupervisor(ctx, team.ID, "user-supervisor-001"); err != nil {
		t.Fatalf("expected supervisor removal to succeed, got error: %v", err)
	}

	ok, err := svc.IsActiveSupervisorOf(ctx, "user-supervisor-001", "user-assignee-001")
	if err != nil {
		t.Fatalf("expected supervision check to succeed, got error: %v", err)
	}
	if ok {
		t.Fatal("expected removed supervisor to no longer have authority")
	}

	active, err := svc.supervisionStore.ListActiveByTeamID(ctx, team.ID)
	if err != nil {
		t.Fatalf("expected active-supervisor lookup to succeed, got error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected zero active supervisors after removal, got %d", len(active))
	}
}

func TestIsActiveSupervisorOfFalseWhenAssigneeHasNoTeam(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newTestService()

	ok, err := svc.IsActiveSupervisorOf(ctx, "user-supervisor-001", "user-assignee-unassigned")
	if err != nil {
		t.Fatalf("expected supervision check to succeed, got error: %v", err)
	}
	if ok {
		t.Fatal("expected no authority over an assignee with no active team")
	}
}
