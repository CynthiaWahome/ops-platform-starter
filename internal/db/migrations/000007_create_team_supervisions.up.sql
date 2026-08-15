CREATE SEQUENCE team_supervisions_seq;

-- Unlike team_memberships, more than one row can be active for the same
-- team at once — co-supervision/delegation (OPS-045) — so there is no
-- equivalent "one active per X" partial unique index here on purpose.
CREATE TABLE team_supervisions (
    id                TEXT PRIMARY KEY,
    team_id           TEXT NOT NULL REFERENCES teams (id),
    user_id           TEXT NOT NULL,
    added_by_user_id  TEXT NOT NULL,
    added_at          TIMESTAMPTZ NOT NULL,
    removed_at        TIMESTAMPTZ
);

CREATE INDEX idx_team_supervisions_team_id_active
    ON team_supervisions (team_id)
    WHERE removed_at IS NULL;

CREATE INDEX idx_team_supervisions_user_id_active
    ON team_supervisions (user_id)
    WHERE removed_at IS NULL;
