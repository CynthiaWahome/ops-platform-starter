package notifications

import (
	"context"
	"errors"
	"fmt"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is Store backed by Postgres (OPS-048) — has an Update
// method for the same reason MemoryStore does: ReadAt changes in place
// after creation, unlike workitems.StatusHistory/AssignmentHistory.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, notification Notification) (Notification, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('notifications_seq') AS n)
		INSERT INTO notifications (
			id, recipient_user_id, work_item_id, kind, message, read_at, created_at
		)
		SELECT 'notification-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2, $3, $4, $5, $6
		FROM seq
		RETURNING id, recipient_user_id, work_item_id, kind, message, read_at, created_at
	`,
		notification.RecipientUserID, notification.WorkItemID, notification.Kind,
		notification.Message, notification.ReadAt, notification.CreatedAt,
	)

	return scanNotification(row)
}

func (s *PostgresStore) GetByID(ctx context.Context, id string) (Notification, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		SELECT id, recipient_user_id, work_item_id, kind, message, read_at, created_at
		FROM notifications
		WHERE id = $1
	`, id)

	notification, err := scanNotification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notification{}, ErrNotFound
	}

	return notification, err
}

func (s *PostgresStore) Update(ctx context.Context, notification Notification) (Notification, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		UPDATE notifications
		SET read_at = $2
		WHERE id = $1
		RETURNING id, recipient_user_id, work_item_id, kind, message, read_at, created_at
	`, notification.ID, notification.ReadAt)

	updated, err := scanNotification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Notification{}, ErrNotFound
	}

	return updated, err
}

func (s *PostgresStore) ListByRecipientUserID(ctx context.Context, recipientUserID string, onlyUnread bool) ([]Notification, error) {
	sql := `
		SELECT id, recipient_user_id, work_item_id, kind, message, read_at, created_at
		FROM notifications
		WHERE recipient_user_id = $1
	`
	if onlyUnread {
		sql += ` AND read_at IS NULL`
	}
	// Oldest first, matching MemoryStore.ListByRecipientUserID exactly —
	// a review on PR #57 caught this returning newest-first here while
	// the in-memory store returns oldest-first, which would silently
	// reverse a caller's notification list depending on which Store the
	// service happened to be built with. workitems' own notification
	// tests (TestServiceNotificationsFireForEventHookedTransitionsOnly)
	// already assert this specific oldest-first order, so that's the
	// one both stores need to agree on.
	sql += ` ORDER BY created_at ASC`

	rows, err := db.Querier(ctx, s.pool).Query(ctx, sql, recipientUserID)
	if err != nil {
		return nil, fmt.Errorf("notifications: list: %w", err)
	}
	defer rows.Close()

	notifications := make([]Notification, 0)
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, notification)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications: iterate rows: %w", err)
	}

	return notifications, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanNotification(row rowScanner) (Notification, error) {
	var notification Notification

	if err := row.Scan(
		&notification.ID, &notification.RecipientUserID, &notification.WorkItemID,
		&notification.Kind, &notification.Message, &notification.ReadAt, &notification.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Notification{}, err
		}
		return Notification{}, fmt.Errorf("notifications: scan: %w", err)
	}

	return notification, nil
}
