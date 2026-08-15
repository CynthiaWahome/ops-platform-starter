package teams

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is Store backed by Postgres (OPS-048) — same contract as
// MemoryStore, same generated id shape.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, team Team) (Team, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('teams_seq') AS n)
		INSERT INTO teams (id, name, created_at)
		SELECT 'team-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2
		FROM seq
		RETURNING id, name, created_at
	`, team.Name, team.CreatedAt)

	return scanTeam(row)
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (Team, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `SELECT id, name, created_at FROM teams WHERE id = $1`, id)

	team, err := scanTeam(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, ErrNotFound
	}

	return team, err
}

func (s *PostgresStore) List(ctx context.Context) ([]Team, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx, `SELECT id, name, created_at FROM teams ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("teams: list: %w", err)
	}
	defer rows.Close()

	teams := make([]Team, 0)
	for rows.Next() {
		team, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("teams: iterate rows: %w", err)
	}

	return teams, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTeam(row rowScanner) (Team, error) {
	var team Team

	if err := row.Scan(&team.ID, &team.Name, &team.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Team{}, err
		}
		return Team{}, fmt.Errorf("teams: scan: %w", err)
	}

	return team, nil
}
