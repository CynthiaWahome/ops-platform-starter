package db

import (
	"context"
	"os"
	"testing"
)

// testDSN returns the connection string for the local Postgres instance
// used to verify this package, skipping the test entirely if
// TEST_DATABASE_URL isn't set — `go test ./...` never requires a real
// Postgres instance to be running (every other package's tests use the
// Memory* stores exactly as before OPS-048); this is the one opt-in
// exception, for the thing that can only be proven against a real
// database.
func testDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping Postgres integration test")
	}

	return dsn
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()

	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("expected Open to succeed, got error: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("expected first Migrate to succeed, got error: %v", err)
	}

	// Running it again should be a no-op, not an error — every startup
	// of a real deployed instance calls Migrate unconditionally.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("expected second Migrate (no-op) to succeed, got error: %v", err)
	}

	var tableCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name != 'schema_migrations'
	`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("expected table count query to succeed, got error: %v", err)
	}

	const wantTables = 9 // work_items, status_history, assignments, assignment_history, teams, team_memberships, team_supervisions, notifications, attachments
	if tableCount != wantTables {
		t.Fatalf("expected %d tables after migration, got %d", wantTables, tableCount)
	}
}
