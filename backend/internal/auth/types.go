package auth

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidToken       = errors.New("invalid token")
)

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAssignee   Role = "assignee"
	RoleSupervisor Role = "supervisor"
	// RoleRequester is the external, low-trust customer role restored in
	// OPS-047 — create a work item, track its own status, nothing else.
	// Not to be confused with the pre-OPS-045 "requester" that got
	// renamed to RoleSupervisor; this is a genuinely new, narrower role,
	// not a revival of the old one.
	RoleRequester Role = "requester"
)

type User struct {
	ID           string
	Identifier   string
	DisplayName  string
	PasswordHash string
	Roles        []Role
	IsActive     bool
}

type Principal struct {
	UserID      string `json:"userId"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"displayName"`
	Roles       []Role `json:"roles"`
}

func (p Principal) HasRole(role Role) bool {
	for _, assignedRole := range p.Roles {
		if assignedRole == role {
			return true
		}
	}

	return false
}

func (p Principal) HasAnyRole(roles ...Role) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}

	return false
}

type Session struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Principal   Principal `json:"user"`
}

type issuedToken struct {
	Value     string
	ExpiresAt time.Time
}
