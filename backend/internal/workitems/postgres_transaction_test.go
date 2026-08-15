package workitems

import (
	"context"
	"errors"
	"testing"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"
)

// failingNotifier always errors — used below to force a failure at the
// very last step of AssignWorkItem's multi-store write sequence, so the
// test can prove everything *before* that point rolled back too.
type failingNotifier struct{}

func (failingNotifier) Notify(ctx context.Context, recipientUserID, workItemID, kind, message string) error {
	return errors.New("simulated notification failure")
}

// TestAssignWorkItemRollsBackEntirelyOnFailure is the direct proof for the
// P1 a review caught on PR #57: AssignWorkItem writes to work_items,
// status_history, assignments, assignment_history, and notifications —
// four separate tables. Before runAtomic existed, a failure on the last
// of those (here, forced via failingNotifier) still left the earlier
// three writes permanently committed: the work item ends up "assigned"
// with no assignment record, and a retry is rejected by
// IsValidTransition because the item isn't in StatusCreated anymore.
//
// This constructs a real workitems.Service against real Postgres stores
// and a real db.PoolTxRunner (not the in-memory stores, which don't have
// a partial-failure mode to prove anything about), forces the failure,
// then reads the work_items row back directly via SQL — bypassing the
// service entirely — to confirm its status is still "created", exactly
// as if AssignWorkItem had never been called at all.
func TestAssignWorkItemRollsBackEntirelyOnFailure(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	service := NewService(
		NewPostgresStore(pool),
		NewPostgresStatusHistoryStore(pool),
		NewPostgresAssignmentStore(pool),
		NewPostgresAssignmentHistoryStore(pool),
		failingNotifier{},
		nil,
		db.PoolTxRunner{Pool: pool},
	)

	item, err := service.Create(ctx, "user-admin-001", CreateInput{
		Title: "Gate repaint", Description: "Repaint the gate", Priority: PriorityMedium,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err = service.AssignWorkItem(ctx, item.ID, "user-admin-001", false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	})
	if err == nil {
		t.Fatal("expected AssignWorkItem to fail because of the forced notification error")
	}

	// Read directly via SQL, bypassing the service, so this doesn't
	// accidentally rely on the same (potentially also-broken) code path
	// under test.
	var status string
	var assignedToUserID *string
	if err := pool.QueryRow(ctx,
		`SELECT status, assigned_to_user_id FROM work_items WHERE id = $1`, item.ID,
	).Scan(&status, &assignedToUserID); err != nil {
		t.Fatalf("expected to read the work item back, got error: %v", err)
	}

	if status != string(StatusCreated) {
		t.Fatalf("expected status to have rolled back to %q, got %q — the failed write left a partial commit", StatusCreated, status)
	}
	if assignedToUserID != nil {
		t.Fatalf("expected assigned_to_user_id to have rolled back to nil, got %v", *assignedToUserID)
	}

	var assignmentCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM assignments WHERE work_item_id = $1`, item.ID,
	).Scan(&assignmentCount); err != nil {
		t.Fatalf("expected assignment count query to succeed, got error: %v", err)
	}
	if assignmentCount != 0 {
		t.Fatalf("expected zero assignment rows after rollback, got %d", assignmentCount)
	}

	var historyCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM status_history WHERE work_item_id = $1`, item.ID,
	).Scan(&historyCount); err != nil {
		t.Fatalf("expected history count query to succeed, got error: %v", err)
	}
	if historyCount != 0 {
		t.Fatalf("expected zero status_history rows after rollback (Create doesn't write one), got %d", historyCount)
	}

	// Confirm the item is genuinely still assignable — a retry after a
	// real transient failure should work, not get stuck rejected by
	// IsValidTransition the way it would have before this fix.
	realService := NewService(
		NewPostgresStore(pool),
		NewPostgresStatusHistoryStore(pool),
		NewPostgresAssignmentStore(pool),
		NewPostgresAssignmentHistoryStore(pool),
		nil, // no notifier at all this time — Notify is a no-op, not a failure
		nil,
		db.PoolTxRunner{Pool: pool},
	)

	if _, err := realService.AssignWorkItem(ctx, item.ID, "user-admin-001", false, AssignInput{
		AssignedToUserID: "user-assignee-001",
	}); err != nil {
		t.Fatalf("expected retry after rollback to succeed, got error: %v", err)
	}
}
