CREATE SEQUENCE notifications_seq;

CREATE TABLE notifications (
    id                 TEXT PRIMARY KEY,
    recipient_user_id  TEXT NOT NULL,
    work_item_id       TEXT NOT NULL REFERENCES work_items (id),
    kind               TEXT NOT NULL,
    message            TEXT NOT NULL,
    read_at            TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL
);

-- List / unread-filter (notifications/store.go) both key off recipient.
CREATE INDEX idx_notifications_recipient_user_id ON notifications (recipient_user_id);
