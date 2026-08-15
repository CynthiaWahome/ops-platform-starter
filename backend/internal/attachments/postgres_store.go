package attachments

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is Store backed by Postgres (OPS-048) — metadata only,
// same split kept since OPS-030: file bytes live wherever Storage points
// (local disk today, S3 later), completely untouched by this store.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, attachment Attachment) (Attachment, error) {
	row := s.pool.QueryRow(ctx, `
		WITH seq AS (SELECT nextval('attachments_seq') AS n)
		INSERT INTO attachments (
			id, work_item_id, uploaded_by_user_id, storage_url, mime_type,
			file_size, kind, created_at
		)
		SELECT 'attachment-' || lpad(n::text, 4, '0'), $1, $2, $3, $4, $5, $6, $7
		FROM seq
		RETURNING id, work_item_id, uploaded_by_user_id, storage_url, mime_type,
			file_size, kind, created_at
	`,
		attachment.WorkItemID, attachment.UploadedByUserID, attachment.StorageURL,
		attachment.MimeType, attachment.FileSize, attachment.Kind, attachment.CreatedAt,
	)

	return scanAttachment(row)
}

func (s *PostgresStore) ListByWorkItemID(ctx context.Context, workItemID string) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, work_item_id, uploaded_by_user_id, storage_url, mime_type,
			file_size, kind, created_at
		FROM attachments
		WHERE work_item_id = $1
		ORDER BY created_at ASC
	`, workItemID)
	if err != nil {
		return nil, fmt.Errorf("attachments: list: %w", err)
	}
	defer rows.Close()

	attachments := make([]Attachment, 0)
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("attachments: iterate rows: %w", err)
	}

	return attachments, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAttachment(row rowScanner) (Attachment, error) {
	var attachment Attachment

	if err := row.Scan(
		&attachment.ID, &attachment.WorkItemID, &attachment.UploadedByUserID,
		&attachment.StorageURL, &attachment.MimeType, &attachment.FileSize,
		&attachment.Kind, &attachment.CreatedAt,
	); err != nil {
		return Attachment{}, fmt.Errorf("attachments: scan: %w", err)
	}

	return attachment, nil
}
