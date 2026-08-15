-- OPS-047: requester visibility is scoped by CreatedByUserID (ListByCreatedByUserID),
-- the same way assignee visibility already relies on the
-- idx_work_items_assigned_to_user_id index from migration 000001.
CREATE INDEX idx_work_items_created_by_user_id ON work_items (created_by_user_id);
