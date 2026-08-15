-- Metadata only, matching attachments.Store's contract exactly — the file
-- bytes themselves stay wherever attachments.Storage points (local disk
-- today, S3 later), completely untouched by this migration. That split
-- was deliberate from OPS-030 on: "swapping to S3 later only touches
-- Storage."
CREATE SEQUENCE attachments_seq;

CREATE TABLE attachments (
    id                   TEXT PRIMARY KEY,
    work_item_id         TEXT NOT NULL REFERENCES work_items (id),
    uploaded_by_user_id  TEXT NOT NULL,
    storage_url          TEXT NOT NULL,
    mime_type            TEXT NOT NULL,
    file_size            BIGINT NOT NULL,
    kind                 TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_attachments_work_item_id ON attachments (work_item_id);
