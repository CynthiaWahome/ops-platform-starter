package workitems

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool opens a fresh pool against TEST_DATABASE_URL, migrates it, and
// truncates every table this package touches before the test runs — same
// "skip if not configured" opt-in shape as internal/db's own tests, so
// `go test ./...` never requires a real Postgres instance.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping Postgres integration test")
	}

	ctx := context.Background()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("expected pool to open, got error: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("expected migrate to succeed, got error: %v", err)
	}

	// TRUNCATE ... CASCADE, not DELETE: work_items has foreign keys
	// pointing at it from every other table, and each test in this file
	// wants a clean slate rather than accumulating rows across runs.
	if _, err := pool.Exec(ctx, `TRUNCATE work_items CASCADE`); err != nil {
		t.Fatalf("expected truncate to succeed, got error: %v", err)
	}

	return pool
}

// TestPostgresStoreSurvivesPoolRestart is OPS-048's actual proof of
// durability: create data through one pool, close it entirely (the
// closest thing a test can do to simulate a process restart), open a
// brand new pool against the same database, and confirm the data is
// still there. Every other test in this file just proves each Postgres
// store's CRUD contract matches its Memory* counterpart; this one proves
// the actual point of the ticket.
func TestPostgresStoreSurvivesPoolRestart(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping Postgres integration test")
	}

	ctx := context.Background()

	firstPool := testPool(t)
	store := NewPostgresStore(firstPool)

	created, err := store.Create(ctx, WorkItem{
		Title:           "Gate repaint",
		Description:     "Repaint the estate gate",
		Status:          StatusCreated,
		Priority:        PriorityHigh,
		CreatedByUserID: "user-admin-001",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	firstPool.Close()

	secondPool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("expected reopened pool to succeed, got error: %v", err)
	}
	defer secondPool.Close()

	reread, err := NewPostgresStore(secondPool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected work item to survive the pool restart, got error: %v", err)
	}

	if reread.Title != created.Title {
		t.Fatalf("expected title %q to survive, got %q", created.Title, reread.Title)
	}
}

func TestPostgresStoreCRUD(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := NewPostgresStore(pool)

	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := store.Create(ctx, WorkItem{
		Title:           "Fence repair",
		Description:     "Fix the east fence",
		Status:          StatusCreated,
		Priority:        PriorityMedium,
		CreatedByUserID: "user-admin-001",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if created.ID == "" || created.ReferenceCode == "" {
		t.Fatal("expected generated id and reference code")
	}

	fetched, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got error: %v", err)
	}
	if fetched.Title != created.Title {
		t.Fatalf("expected title %q, got %q", created.Title, fetched.Title)
	}

	assignee := "user-assignee-001"
	created.AssignedToUserID = &assignee
	created.Status = StatusAssigned

	updated, err := store.Update(ctx, created)
	if err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}
	if updated.AssignedToUserID == nil || *updated.AssignedToUserID != assignee {
		t.Fatalf("expected assigned_to_user_id %q, got %v", assignee, updated.AssignedToUserID)
	}

	byAssignee, err := store.ListByAssignedToUserID(ctx, assignee)
	if err != nil {
		t.Fatalf("expected list by assignee to succeed, got error: %v", err)
	}
	if len(byAssignee) != 1 || byAssignee[0].ID != created.ID {
		t.Fatalf("expected exactly the one assigned work item, got %+v", byAssignee)
	}

	all, err := store.List(ctx)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 work item total, got %d", len(all))
	}

	if _, err := store.GetByID(ctx, "workitem-9999"); err == nil {
		t.Fatal("expected error for a missing work item id")
	}
}
