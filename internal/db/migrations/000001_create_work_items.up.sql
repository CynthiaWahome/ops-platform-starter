-- work_items.id and reference_code keep the same "<entity>-0004" / "WI-0004"
-- shape the in-memory MemoryStore already generates (workitems/store.go).
-- Nothing anywhere in this codebase parses that format, it's treated as an
-- opaque string everywhere, but keeping it identical avoids surprising
-- anyone diffing behavior between the two backends.
--
-- id and reference_code must share the exact same numeric value (the
-- in-memory store uses one counter for both), so the sequence is read
-- once per insert in application code (a single CTE, one nextval() call)
-- rather than as a per-column DEFAULT — two independent DEFAULT
-- expressions each calling nextval() would allocate two different
-- numbers, not the same one.
CREATE SEQUENCE work_items_seq;

CREATE TABLE work_items (
    id                  TEXT PRIMARY KEY,
    reference_code      TEXT NOT NULL UNIQUE,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL,
    status              TEXT NOT NULL,
    priority            TEXT NOT NULL,
    created_by_user_id  TEXT NOT NULL,
    assigned_to_user_id TEXT,
    location_text       TEXT,
    due_at              TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

-- ListByAssignedToUserID (workitems/store.go) is the one filtered query
-- this table serves outside GetByID — index it directly rather than
-- relying on a sequential scan once a real client has more than a handful
-- of rows.
CREATE INDEX idx_work_items_assigned_to_user_id ON work_items (assigned_to_user_id);
