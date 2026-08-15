// Package teams is the enforced boundary a supervisor's authority is scoped
// to (OPS-045). An assignee belongs to exactly one active team at a time; a
// team can have more than one active supervisor at once (co-supervision).
// Both relationships use the same never-hard-delete shape as
// workitems.AssignmentHistory: nothing is edited in place, a change closes
// the old row (RemovedAt) and opens a new one, so "who supervised this team,
// and when" survives every reshuffle.
package teams

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput  = errors.New("invalid team input")
	ErrNotFound      = errors.New("team not found")
	ErrNotSupervisor = errors.New("user is not an active supervisor of this team")
)

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Membership records one assignee's tenure on one team. RemovedAt is nil
// while the membership is active. Only admin creates/closes these — see
// Service.AddAssignee.
type Membership struct {
	ID            string     `json:"id"`
	TeamID        string     `json:"teamId"`
	UserID        string     `json:"userId"`
	AddedByUserID string     `json:"addedByUserId"`
	AddedAt       time.Time  `json:"addedAt"`
	RemovedAt     *time.Time `json:"removedAt,omitempty"`
}

// Supervision records one supervisor's tenure on one team. Unlike
// Membership, more than one Supervision row for the same team can be active
// at once — that's co-supervision/delegation (e.g. temporary cover while a
// supervisor is on leave).
type Supervision struct {
	ID            string     `json:"id"`
	TeamID        string     `json:"teamId"`
	UserID        string     `json:"userId"`
	AddedByUserID string     `json:"addedByUserId"`
	AddedAt       time.Time  `json:"addedAt"`
	RemovedAt     *time.Time `json:"removedAt,omitempty"`
}
