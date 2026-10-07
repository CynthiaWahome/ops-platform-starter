CREATE SEQUENCE users_seq;

CREATE TABLE users (
    id                        TEXT PRIMARY KEY,
    identifier                TEXT NOT NULL UNIQUE,
    display_name              TEXT NOT NULL,
    password_hash             TEXT NOT NULL,
    roles                     TEXT[] NOT NULL,
    is_active                 BOOLEAN NOT NULL DEFAULT true,
    requires_password_change  BOOLEAN NOT NULL DEFAULT false,
    created_by_user_id        TEXT,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);
