package workitems

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAssignmentHistoryStore is AssignmentHistoryStore backed by
// Postgres — append-only, same contract as
// PostgresStatusHistoryStore/MemoryAssignmentHistoryStore: no Update
// method here, on purpose.
type PostgresAssignmentHistoryStore struct {
	pool *pgxpool.Pool
}

func NewPostgresAssignmentHistoryStore(pool *pgxpool.Pool) *PostgresAssignmentHistoryStore {
	return &PostgresAssignmentHistoryStore{pool: pool}
}

func (s *PostgresAssignmentHistoryStore) Create(ctx context.Context, entry AssignmentHistory) (AssignmentHistory, error) {
	row := s.pool.QueryRow(ctx, `
		WITH seq AS (SELECT nextval('assignment_history_seq') AS n)
		INSERT INTO assignment_history (
			id, work_item_id, action, actor_user_id, assigned_to_user_id,
			note, created_at
		)
		SELECT 'assignmenthistory-' || lpad(n::text, 4, '0'), $1, $2, $3, $4, $5, $6
		FROM seq
		RETURNING id, work_item_id, action, actor_user_id, assigned_to_user_id,
			note, created_at
	`,
		entry.WorkItemID, entry.Action, entry.ActorUserID, entry.AssignedToUserID,
		entry.Note, entry.CreatedAt,
	)

	return scanAssignmentHistory(row)
}

func (s *PostgresAssignmentHistoryStore) ListByWorkItemID(ctx context.Context, workItemID string) ([]AssignmentHistory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, work_item_id, action, actor_user_id, assigned_to_user_id,
			note, created_at
		FROM assignment_history
		WHERE work_item_id = $1
		ORDER BY created_at ASC
	`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("workitems: list assignment history: %w", err)
	}
	defer rows.Close()

	entries := make([]AssignmentHistory, 0)
	for rows.Next() {
		entry, err := scanAssignmentHistory(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workitems: iterate assignment history: %w", err)
	}

	return entries, nil
}

func scanAssignmentHistory(row rowScanner) (AssignmentHistory, error) {
	var entry AssignmentHistory

	if err := row.Scan(
		&entry.ID, &entry.WorkItemID, &entry.Action, &entry.ActorUserID,
		&entry.AssignedToUserID, &entry.Note, &entry.CreatedAt,
	); err != nil {
		return AssignmentHistory{}, fmt.Errorf("workitems: scan assignment history: %w", err)
	}

	return entry, nil
}
