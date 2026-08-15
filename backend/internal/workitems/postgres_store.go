package workitems

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is Store backed by a real Postgres table (OPS-048) instead
// of the in-memory map MemoryStore uses. Every method has the exact same
// contract as MemoryStore's — same errors, same ordering, same generated
// id/reference_code shape — so workitems.Service and every test that
// exercises it through the Store interface never needs to know or care
// which one it's talking to.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Create generates id and reference_code from the same sequence in a
// single round trip (see migrations/000001_create_work_items.up.sql for
// why this can't be two independent column DEFAULTs) via a CTE, then
// inserts the row and returns it with those generated fields filled in —
// the same shape MemoryStore.Create already returns.
func (s *PostgresStore) Create(ctx context.Context, item WorkItem) (WorkItem, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('work_items_seq') AS n)
		INSERT INTO work_items (
			id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
		)
		SELECT
			'workitem-' || lpad(n::text, greatest(length(n::text), 4), '0'),
			'WI-' || lpad(n::text, greatest(length(n::text), 4), '0'),
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		FROM seq
		RETURNING id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
	`,
		item.Title, item.Description, item.Status, item.Priority,
		item.CreatedByUserID, item.AssignedToUserID, item.LocationText, item.DueAt,
		item.CreatedAt, item.UpdatedAt,
	)

	return scanWorkItem(row)
}

func (s *PostgresStore) List(ctx context.Context) ([]WorkItem, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx, `
		SELECT id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
		FROM work_items
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("workitems: list: %w", err)
	}
	defer rows.Close()

	return scanWorkItems(rows)
}

// ListByAssignedToUserID is the WHERE clause MemoryStore's own doc comment
// on this method already predicted a real database-backed Store would
// use, instead of the full scan + filter it has to do in-process.
func (s *PostgresStore) ListByAssignedToUserID(ctx context.Context, userID string) ([]WorkItem, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx, `
		SELECT id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
		FROM work_items
		WHERE assigned_to_user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("workitems: list by assignee: %w", err)
	}
	defer rows.Close()

	return scanWorkItems(rows)
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (WorkItem, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		SELECT id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
		FROM work_items
		WHERE id = $1
	`, id)

	return scanWorkItem(row)
}

func (s *PostgresStore) Update(ctx context.Context, item WorkItem) (WorkItem, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		UPDATE work_items
		SET title = $2, description = $3, status = $4, priority = $5,
			assigned_to_user_id = $6, location_text = $7, due_at = $8,
			updated_at = $9
		WHERE id = $1
		RETURNING id, reference_code, title, description, status, priority,
			created_by_user_id, assigned_to_user_id, location_text, due_at,
			created_at, updated_at
	`,
		item.ID, item.Title, item.Description, item.Status, item.Priority,
		item.AssignedToUserID, item.LocationText, item.DueAt, item.UpdatedAt,
	)

	return scanWorkItem(row)
}

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query,
// per-row via Scan) — letting scanWorkItem be shared by both single-row
// and multi-row callers instead of duplicating the same 12-column Scan
// call in four places.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(row rowScanner) (WorkItem, error) {
	var item WorkItem

	err := row.Scan(
		&item.ID, &item.ReferenceCode, &item.Title, &item.Description,
		&item.Status, &item.Priority, &item.CreatedByUserID,
		&item.AssignedToUserID, &item.LocationText, &item.DueAt,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkItem{}, ErrNotFound
	}
	if err != nil {
		return WorkItem{}, fmt.Errorf("workitems: scan: %w", err)
	}

	return item, nil
}

func scanWorkItems(rows pgx.Rows) ([]WorkItem, error) {
	items := make([]WorkItem, 0)

	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workitems: iterate rows: %w", err)
	}

	return items, nil
}
