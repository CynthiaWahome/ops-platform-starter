package notifications

import "time"

// Kind identifies what kind of workflow event a Notification is about.
// These map directly to the "Event Hooks" list in
// notes/OPS_PLATFORM_STARTER_STATUS_MODEL.md — deliberately a small,
// fixed set, not "every status transition." Starting work
// (Accepted -> InProgress) is a real transition but nobody needs a
// notification about it; these six are the checkpoints someone is
// actually waiting on.
type Kind string

const (
	KindAssignmentCreated  Kind = "assignment_created"
	KindAssignmentAccepted Kind = "assignment_accepted"
	KindEvidenceSubmitted  Kind = "evidence_submitted"
	KindWorkFlagged        Kind = "work_flagged"
	KindWorkVerified       Kind = "work_verified"
	KindWorkCompleted      Kind = "work_completed"
)

// Notification is a single record raised as a side effect of a workflow
// event. Unlike StatusHistory or AssignmentHistory, this is not
// append-only — ReadAt starts nil and is set once, in place, when the
// recipient reads it. That's a genuinely different shape from every
// other store this starter has built so far, and it's why Notification
// gets its own Update-capable store instead of being bolted onto one of
// the append-only history stores.
type Notification struct {
	ID              string     `json:"id"`
	RecipientUserID string     `json:"recipientUserId"`
	WorkItemID      string     `json:"workItemId"`
	Kind            Kind       `json:"kind"`
	Message         string     `json:"message"`
	ReadAt          *time.Time `json:"readAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}
