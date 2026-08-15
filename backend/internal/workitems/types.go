package workitems

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrNotFound             = errors.New("work item not found")
	ErrInvalidInput         = errors.New("invalid work item input")
	ErrInvalidPriority      = errors.New("invalid work item priority")
	ErrInvalidStatus        = errors.New("invalid work item status")
	ErrInvalidTransition    = errors.New("status transition not allowed")
	ErrAssignmentNotFound   = errors.New("assignment not found")
	ErrAssignmentNotOwned   = errors.New("assignment does not belong to this user")
	ErrAssignmentNotPending = errors.New("assignment has already been responded to")
	ErrFeedbackRequired     = errors.New("feedback note is required when flagging a work item")
)

type Status string

const (
	StatusCreated            Status = "created"
	StatusAssigned           Status = "assigned"
	StatusAccepted           Status = "accepted"
	StatusInProgress         Status = "in_progress"
	StatusSubmittedForReview Status = "submitted_for_review"
	StatusVerified           Status = "verified"
	StatusFlagged            Status = "flagged"
	StatusCompleted          Status = "completed"
	StatusCancelled          Status = "cancelled"
)

// validStatusTransitions lists every status a work item may move to from a
// given status. Completed and Cancelled are end states: they have no
// outgoing entries, so nothing can move once a work item lands there.
//
// StatusAssigned -> StatusCreated is the one addition on top of the OPS-003
// backbone: it is the decline path (OPS-023). Declining bounces the work
// item back to unassigned rather than cancelling it, so an admin can
// reassign it to someone else instead of the work item dying outright.
var validStatusTransitions = map[Status][]Status{
	StatusCreated:            {StatusAssigned, StatusCancelled},
	StatusAssigned:           {StatusAccepted, StatusCancelled, StatusCreated},
	StatusAccepted:           {StatusInProgress, StatusCancelled},
	StatusInProgress:         {StatusSubmittedForReview, StatusCancelled},
	StatusSubmittedForReview: {StatusVerified, StatusFlagged, StatusCancelled},
	StatusVerified:           {StatusCompleted, StatusCancelled},
	StatusFlagged:            {StatusInProgress, StatusCancelled},
}

// assigneeAllowedTransitions is the subset of validStatusTransitions an
// assignee may trigger themselves, on a work item assigned to them — the
// "start work" and "submit progress update" rows from the permission
// matrix (notes/OPS_PLATFORM_STARTER_PERMISSION_MATRIX.md), plus the
// rework path (OPS-033): Flagged -> InProgress, so an assignee whose
// submission was sent back can pick the work back up themselves instead
// of waiting on an admin to move it. Triggering Flagged in the first
// place (verify, flag, complete, cancel, assign) stays admin-only. This
// is deliberately a smaller map, not a filtered view of
// validStatusTransitions, so the assignee-safe list is easy to read on its
// own without cross-referencing the full table.
var assigneeAllowedTransitions = map[Status][]Status{
	StatusAccepted:   {StatusInProgress},
	StatusInProgress: {StatusSubmittedForReview},
	StatusFlagged:    {StatusInProgress},
}

// IsAssigneeAllowedTransition reports whether an assignee (as opposed to an
// admin) is permitted to trigger a given status move themselves.
func IsAssigneeAllowedTransition(from, to Status) bool {
	return slices.Contains(assigneeAllowedTransitions[from], to)
}

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

type WorkItem struct {
	ID               string     `json:"id"`
	ReferenceCode    string     `json:"referenceCode"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Status           Status     `json:"status"`
	Priority         Priority   `json:"priority"`
	CreatedByUserID  string     `json:"createdByUserId"`
	AssignedToUserID *string    `json:"assignedToUserId,omitempty"`
	LocationText     *string    `json:"locationText,omitempty"`
	DueAt            *time.Time `json:"dueAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type CreateInput struct {
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Priority     Priority   `json:"priority"`
	LocationText *string    `json:"locationText,omitempty"`
	DueAt        *time.Time `json:"dueAt,omitempty"`
}

type UpdateInput struct {
	Title        *string    `json:"title,omitempty"`
	Description  *string    `json:"description,omitempty"`
	Priority     *Priority  `json:"priority,omitempty"`
	LocationText *string    `json:"locationText,omitempty"`
	DueAt        *time.Time `json:"dueAt,omitempty"`
}

// StatusHistory records one status change on a work item. FromStatus is
// nil for the very first entry, since a brand new work item has no
// previous status to record.
type StatusHistory struct {
	ID              string    `json:"id"`
	WorkItemID      string    `json:"workItemId"`
	FromStatus      *Status   `json:"fromStatus,omitempty"`
	ToStatus        Status    `json:"toStatus"`
	ChangedByUserID string    `json:"changedByUserId"`
	Reason          *string   `json:"reason,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type ChangeStatusInput struct {
	ToStatus Status  `json:"toStatus"`
	Reason   *string `json:"reason,omitempty"`
}

// VerifyInput is the request body for POST /workitems/{id}/verify. Note is
// optional — an admin can verify with no comment at all — and maps
// straight onto ChangeStatusInput.Reason, the same field every other
// status change already records itself under in StatusHistory.
type VerifyInput struct {
	Note *string `json:"note,omitempty"`
}

// FlagInput is the request body for POST /workitems/{id}/flag. Note is a
// plain string, not a pointer like VerifyInput's — unlike verifying,
// flagging without feedback leaves the assignee with nothing to act on,
// so the field being required is part of the type, not just a runtime
// check bolted on afterward.
type FlagInput struct {
	Note string `json:"note"`
}

type AssignmentStatus string

const (
	AssignmentStatusAssigned AssignmentStatus = "assigned"
	AssignmentStatusAccepted AssignmentStatus = "accepted"
	AssignmentStatusDeclined AssignmentStatus = "declined"
)

// Assignment represents the act of giving a work item to an assignee. It is
// kept separate from WorkItem because assignment has its own lifecycle
// (assigned -> accepted/declined) that should not be flattened into the
// work item row. RespondedAt and ResponseNote stay nil until the assignee
// responds via RespondToAssignment.
type Assignment struct {
	ID               string           `json:"id"`
	WorkItemID       string           `json:"workItemId"`
	AssignedByUserID string           `json:"assignedByUserId"`
	AssignedToUserID string           `json:"assignedToUserId"`
	Status           AssignmentStatus `json:"status"`
	AssignedAt       time.Time        `json:"assignedAt"`
	RespondedAt      *time.Time       `json:"respondedAt,omitempty"`
	ResponseNote     *string          `json:"responseNote,omitempty"`
}

// AssignmentHistory records one assignment event on a work item —
// OPS-040's assignment audit trail. AssignmentStore (above) only ever
// keeps the current assignment, overwritten in place as it moves through
// assigned -> accepted/declined; a reassignment after a decline erases
// the previous row entirely. AssignmentHistory is append-only, mirroring
// StatusHistory: nothing here is ever edited, so a full trail of who
// assigned what to whom, and how each assignee responded, survives even
// after the current assignment is overwritten.
//
// Action reuses AssignmentStatus rather than introducing a second enum
// that would just duplicate it — "assigned", "accepted", "declined" mean
// the same thing whether read off the current assignment or a history
// row.
type AssignmentHistory struct {
	ID               string           `json:"id"`
	WorkItemID       string           `json:"workItemId"`
	Action           AssignmentStatus `json:"action"`
	ActorUserID      string           `json:"actorUserId"`
	AssignedToUserID string           `json:"assignedToUserId"`
	Note             *string          `json:"note,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
}

type AssignInput struct {
	AssignedToUserID string `json:"assignedToUserId"`
}

type RespondToAssignmentInput struct {
	Note *string `json:"note,omitempty"`
}

func (p Priority) IsValid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return true
	default:
		return false
	}
}

func (s Status) IsValid() bool {
	switch s {
	case StatusCreated, StatusAssigned, StatusAccepted, StatusInProgress,
		StatusSubmittedForReview, StatusVerified, StatusFlagged,
		StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// IsValidTransition reports whether a work item is allowed to move from one
// status to another. Completed and Cancelled are end states: there is no
// entry for them in validStatusTransitions, so anything checked against them
// as the "from" status correctly comes back false.
func IsValidTransition(from, to Status) bool {
	return slices.Contains(validStatusTransitions[from], to)
}
