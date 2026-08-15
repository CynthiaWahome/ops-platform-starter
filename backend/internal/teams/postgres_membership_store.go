package teams

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresMembershipStore is MembershipStore backed by Postgres. The
// schema also enforces "at most one active membership per user" via a
// partial unique index (migrations/000006_create_team_memberships.up.sql)
// — a real database-level guarantee MemoryMembershipStore only ever had
// as an application-code convention.
type PostgresMembershipStore struct {
	pool *pgxpool.Pool
}

func NewPostgresMembershipStore(pool *pgxpool.Pool) *PostgresMembershipStore {
	return &PostgresMembershipStore{pool: pool}
}

func (s *PostgresMembershipStore) Create(ctx context.Context, membership Membership) (Membership, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('team_memberships_seq') AS n)
		INSERT INTO team_memberships (id, team_id, user_id, added_by_user_id, added_at, removed_at)
		SELECT 'teammembership-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2, $3, $4, $5
		FROM seq
		RETURNING id, team_id, user_id, added_by_user_id, added_at, removed_at
	`, membership.TeamID, membership.UserID, membership.AddedByUserID, membership.AddedAt, membership.RemovedAt)

	return scanMembership(row)
}

func (s *PostgresMembershipStore) Update(ctx context.Context, membership Membership) (Membership, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		UPDATE team_memberships
		SET removed_at = $2
		WHERE id = $1
		RETURNING id, team_id, user_id, added_by_user_id, added_at, removed_at
	`, membership.ID, membership.RemovedAt)

	updated, err := scanMembership(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrNotFound
	}

	return updated, err
}

func (s *PostgresMembershipStore) GetActiveByUserID(ctx context.Context, userID string) (Membership, bool, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		SELECT id, team_id, user_id, added_by_user_id, added_at, removed_at
		FROM team_memberships
		WHERE user_id = $1 AND removed_at IS NULL
	`, userID)

	membership, err := scanMembership(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, false, nil
	}
	if err != nil {
		return Membership{}, false, err
	}

	return membership, true, nil
}

func (s *PostgresMembershipStore) ListActiveByTeamID(ctx context.Context, teamID string) ([]Membership, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx, `
		SELECT id, team_id, user_id, added_by_user_id, added_at, removed_at
		FROM team_memberships
		WHERE team_id = $1 AND removed_at IS NULL
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("teams: list active memberships: %w", err)
	}
	defer rows.Close()

	memberships := make([]Membership, 0)
	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("teams: iterate memberships: %w", err)
	}

	return memberships, nil
}

func scanMembership(row rowScanner) (Membership, error) {
	var membership Membership

	if err := row.Scan(
		&membership.ID, &membership.TeamID, &membership.UserID,
		&membership.AddedByUserID, &membership.AddedAt, &membership.RemovedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Membership{}, err
		}
		return Membership{}, fmt.Errorf("teams: scan membership: %w", err)
	}

	return membership, nil
}
