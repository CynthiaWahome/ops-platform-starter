CREATE SEQUENCE assignments_seq;

-- One active row per work item, upserted in place — matches
-- MemoryAssignmentStore exactly (Create overwrites, Update overwrites the
-- same row keyed by work_item_id). AssignmentHistory (below) is the
-- append-only trail; this table is deliberately not.
CREATE TABLE assignments (
    id                  TEXT PRIMARY KEY,
    work_item_id        TEXT NOT NULL UNIQUE REFERENCES work_items (id),
    assigned_by_user_id TEXT NOT NULL,
    assigned_to_user_id TEXT NOT NULL,
    status              TEXT NOT NULL,
    assigned_at         TIMESTAMPTZ NOT NULL,
    responded_at        TIMESTAMPTZ,
    response_note       TEXT
);
