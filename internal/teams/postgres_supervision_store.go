package teams

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSupervisionStore is SupervisionStore backed by Postgres. Unlike
// PostgresMembershipStore there is no "one active row" constraint in the
// schema — co-supervision means several rows can be active for the same
// team at once, on purpose.
type PostgresSupervisionStore struct {
	pool *pgxpool.Pool
}

func NewPostgresSupervisionStore(pool *pgxpool.Pool) *PostgresSupervisionStore {
	return &PostgresSupervisionStore{pool: pool}
}

func (s *PostgresSupervisionStore) Create(ctx context.Context, supervision Supervision) (Supervision, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('team_supervisions_seq') AS n)
		INSERT INTO team_supervisions (id, team_id, user_id, added_by_user_id, added_at, removed_at)
		SELECT 'teamsupervision-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2, $3, $4, $5
		FROM seq
		RETURNING id, team_id, user_id, added_by_user_id, added_at, removed_at
	`, supervision.TeamID, supervision.UserID, supervision.AddedByUserID, supervision.AddedAt, supervision.RemovedAt)

	return scanSupervision(row)
}

func (s *PostgresSupervisionStore) Update(ctx context.Context, supervision Supervision) (Supervision, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		UPDATE team_supervisions
		SET removed_at = $2
		WHERE id = $1
		RETURNING id, team_id, user_id, added_by_user_id, added_at, removed_at
	`, supervision.ID, supervision.RemovedAt)

	updated, err := scanSupervision(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Supervision{}, ErrNotFound
	}

	return updated, err
}

func (s *PostgresSupervisionStore) ListActiveByTeamID(ctx context.Context, teamID string) ([]Supervision, error) {
	return s.queryActive(ctx, `
		SELECT id, team_id, user_id, added_by_user_id, added_at, removed_at
		FROM team_supervisions
		WHERE team_id = $1 AND removed_at IS NULL
	`, teamID)
}

func (s *PostgresSupervisionStore) ListActiveByUserID(ctx context.Context, userID string) ([]Supervision, error) {
	return s.queryActive(ctx, `
		SELECT id, team_id, user_id, added_by_user_id, added_at, removed_at
		FROM team_supervisions
		WHERE user_id = $1 AND removed_at IS NULL
	`, userID)
}

func (s *PostgresSupervisionStore) queryActive(ctx context.Context, sql string, arg string) ([]Supervision, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx, sql, arg)
	if err != nil {
		return nil, fmt.Errorf("teams: list active supervisions: %w", err)
	}
	defer rows.Close()

	supervisions := make([]Supervision, 0)
	for rows.Next() {
		supervision, err := scanSupervision(rows)
		if err != nil {
			return nil, err
		}
		supervisions = append(supervisions, supervision)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("teams: iterate supervisions: %w", err)
	}

	return supervisions, nil
}

func scanSupervision(row rowScanner) (Supervision, error) {
	var supervision Supervision

	if err := row.Scan(
		&supervision.ID, &supervision.TeamID, &supervision.UserID,
		&supervision.AddedByUserID, &supervision.AddedAt, &supervision.RemovedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Supervision{}, err
		}
		return Supervision{}, fmt.Errorf("teams: scan supervision: %w", err)
	}

	return supervision, nil
}
