package workitems

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("work item not found")
	ErrInvalidInput    = errors.New("invalid work item input")
	ErrInvalidPriority = errors.New("invalid work item priority")
)

type Status string

const (
	StatusCreated Status = "created"
)

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
