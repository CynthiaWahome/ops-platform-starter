package workitems

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrNotFound        = errors.New("work item not found")
	ErrInvalidInput    = errors.New("invalid work item input")
	ErrInvalidPriority = errors.New("invalid work item priority")
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
var validStatusTransitions = map[Status][]Status{
	StatusCreated:            {StatusAssigned, StatusCancelled},
	StatusAssigned:           {StatusAccepted, StatusCancelled},
	StatusAccepted:           {StatusInProgress, StatusCancelled},
	StatusInProgress:         {StatusSubmittedForReview, StatusCancelled},
	StatusSubmittedForReview: {StatusVerified, StatusFlagged, StatusCancelled},
	StatusVerified:           {StatusCompleted, StatusCancelled},
	StatusFlagged:            {StatusInProgress, StatusCancelled},
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
