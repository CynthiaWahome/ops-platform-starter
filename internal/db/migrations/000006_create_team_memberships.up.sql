CREATE SEQUENCE team_memberships_seq;

CREATE TABLE team_memberships (
    id                TEXT PRIMARY KEY,
    team_id           TEXT NOT NULL REFERENCES teams (id),
    user_id           TEXT NOT NULL,
    added_by_user_id  TEXT NOT NULL,
    added_at          TIMESTAMPTZ NOT NULL,
    removed_at        TIMESTAMPTZ
);

CREATE INDEX idx_team_memberships_team_id ON team_memberships (team_id);

-- "An assignee has at most one active team" (teams/store.go) is only ever
-- enforced in application code by MembershipStore today — the service
-- closes the old row before opening a new one, but nothing stops two
-- active rows if that ever got bypassed. A partial unique index makes it
-- a real database-level guarantee instead: at most one row per user_id
-- where removed_at is still null.
CREATE UNIQUE INDEX idx_team_memberships_one_active_per_user
    ON team_memberships (user_id)
    WHERE removed_at IS NULL;
