CREATE SEQUENCE assignment_history_seq;

CREATE TABLE assignment_history (
    id                   TEXT PRIMARY KEY,
    work_item_id         TEXT NOT NULL REFERENCES work_items (id),
    action               TEXT NOT NULL,
    actor_user_id        TEXT NOT NULL,
    assigned_to_user_id  TEXT NOT NULL,
    note                 TEXT,
    created_at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_assignment_history_work_item_id ON assignment_history (work_item_id);
