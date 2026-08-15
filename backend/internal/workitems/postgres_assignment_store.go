package workitems

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAssignmentStore is AssignmentStore backed by Postgres. One
// active row per work item, matching MemoryAssignmentStore exactly —
// Create upserts on the work_item_id unique constraint (see
// migrations/000003_create_assignments.up.sql) rather than always
// inserting a fresh row, since a reassignment after a decline is supposed
// to replace the current row, not accumulate one.
type PostgresAssignmentStore struct {
	pool *pgxpool.Pool
}

func NewPostgresAssignmentStore(pool *pgxpool.Pool) *PostgresAssignmentStore {
	return &PostgresAssignmentStore{pool: pool}
}

func (s *PostgresAssignmentStore) Create(ctx context.Context, assignment Assignment) (Assignment, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('assignments_seq') AS n)
		INSERT INTO assignments (
			id, work_item_id, assigned_by_user_id, assigned_to_user_id,
			status, assigned_at, responded_at, response_note
		)
		SELECT 'assignment-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2, $3, $4, $5, $6, $7
		FROM seq
		ON CONFLICT (work_item_id) DO UPDATE SET
			assigned_by_user_id = EXCLUDED.assigned_by_user_id,
			assigned_to_user_id = EXCLUDED.assigned_to_user_id,
			status = EXCLUDED.status,
			assigned_at = EXCLUDED.assigned_at,
			responded_at = EXCLUDED.responded_at,
			response_note = EXCLUDED.response_note
		RETURNING id, work_item_id, assigned_by_user_id, assigned_to_user_id,
			status, assigned_at, responded_at, response_note
	`,
		assignment.WorkItemID, assignment.AssignedByUserID, assignment.AssignedToUserID,
		assignment.Status, assignment.AssignedAt, assignment.RespondedAt, assignment.ResponseNote,
	)

	return scanAssignment(row)
}

func (s *PostgresAssignmentStore) GetByWorkItemID(ctx context.Context, workItemID string) (Assignment, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		SELECT id, work_item_id, assigned_by_user_id, assigned_to_user_id,
			status, assigned_at, responded_at, response_note
		FROM assignments
		WHERE work_item_id = $1
	`, workItemID)

	assignment, err := scanAssignment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrAssignmentNotFound
	}

	return assignment, err
}

func (s *PostgresAssignmentStore) Update(ctx context.Context, assignment Assignment) (Assignment, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		UPDATE assignments
		SET status = $2, responded_at = $3, response_note = $4
		WHERE work_item_id = $1
		RETURNING id, work_item_id, assigned_by_user_id, assigned_to_user_id,
			status, assigned_at, responded_at, response_note
	`,
		assignment.WorkItemID, assignment.Status, assignment.RespondedAt, assignment.ResponseNote,
	)

	assignment, err := scanAssignment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrAssignmentNotFound
	}

	return assignment, err
}

func scanAssignment(row rowScanner) (Assignment, error) {
	var assignment Assignment

	if err := row.Scan(
		&assignment.ID, &assignment.WorkItemID, &assignment.AssignedByUserID,
		&assignment.AssignedToUserID, &assignment.Status, &assignment.AssignedAt,
		&assignment.RespondedAt, &assignment.ResponseNote,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Assignment{}, err
		}
		return Assignment{}, fmt.Errorf("workitems: scan assignment: %w", err)
	}

	return assignment, nil
}
