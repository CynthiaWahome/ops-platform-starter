package workitems

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStatusHistoryStore is StatusHistoryStore backed by Postgres —
// append-only in the schema exactly the way MemoryStatusHistoryStore is
// append-only in code: there is no Update method here, on purpose.
type PostgresStatusHistoryStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStatusHistoryStore(pool *pgxpool.Pool) *PostgresStatusHistoryStore {
	return &PostgresStatusHistoryStore{pool: pool}
}

func (s *PostgresStatusHistoryStore) Create(ctx context.Context, entry StatusHistory) (StatusHistory, error) {
	row := s.pool.QueryRow(ctx, `
		WITH seq AS (SELECT nextval('status_history_seq') AS n)
		INSERT INTO status_history (
			id, work_item_id, from_status, to_status, changed_by_user_id,
			reason, created_at
		)
		SELECT 'statushistory-' || lpad(n::text, 4, '0'), $1, $2, $3, $4, $5, $6
		FROM seq
		RETURNING id, work_item_id, from_status, to_status, changed_by_user_id,
			reason, created_at
	`,
		entry.WorkItemID, entry.FromStatus, entry.ToStatus, entry.ChangedByUserID,
		entry.Reason, entry.CreatedAt,
	)

	return scanStatusHistory(row)
}

func (s *PostgresStatusHistoryStore) ListByWorkItemID(ctx context.Context, workItemID string) ([]StatusHistory, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, work_item_id, from_status, to_status, changed_by_user_id,
			reason, created_at
		FROM status_history
		WHERE work_item_id = $1
		ORDER BY created_at ASC
	`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("workitems: list status history: %w", err)
	}
	defer rows.Close()

	entries := make([]StatusHistory, 0)
	for rows.Next() {
		entry, err := scanStatusHistory(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workitems: iterate status history: %w", err)
	}

	return entries, nil
}

func scanStatusHistory(row rowScanner) (StatusHistory, error) {
	var entry StatusHistory

	if err := row.Scan(
		&entry.ID, &entry.WorkItemID, &entry.FromStatus, &entry.ToStatus,
		&entry.ChangedByUserID, &entry.Reason, &entry.CreatedAt,
	); err != nil {
		return StatusHistory{}, fmt.Errorf("workitems: scan status history: %w", err)
	}

	return entry, nil
}
