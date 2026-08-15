CREATE SEQUENCE status_history_seq;

CREATE TABLE status_history (
    id                 TEXT PRIMARY KEY,
    work_item_id       TEXT NOT NULL REFERENCES work_items (id),
    from_status        TEXT,
    to_status          TEXT NOT NULL,
    changed_by_user_id TEXT NOT NULL,
    reason             TEXT,
    created_at         TIMESTAMPTZ NOT NULL
);

-- ListByWorkItemID (status_history_store.go) is the only read this table
-- serves.
CREATE INDEX idx_status_history_work_item_id ON status_history (work_item_id);
