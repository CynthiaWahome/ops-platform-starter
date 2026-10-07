package auth

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("inactive user")
	ErrInvalidToken       = errors.New("invalid token")
	// ErrInvalidUserInput, ErrInvalidRole, ErrIdentifierTaken and
	// ErrUserNotFound back the admin user-management surface added in
	// OPS-067 (CreateUser/ListUsers/UpdateUser/ResetPassword).
	ErrInvalidUserInput = errors.New("invalid user input")
	ErrInvalidRole      = errors.New("invalid role")
	ErrIdentifierTaken  = errors.New("identifier already in use")
	ErrUserNotFound     = errors.New("user not found")
	// ErrGoogleEmailAlreadyRegistered backs OPS-068a's explicitly
	// out-of-scope case: a Google identity whose email already belongs to
	// an existing local (password) account. Linking the two is a
	// deliberately separate, later piece of work, not handled here.
	ErrGoogleEmailAlreadyRegistered = errors.New("an account with this email already exists; log in with your password instead")
	// ErrInvalidOrExpiredCode backs OPS-068b's OTP verification and
	// password-reset flows — one error for "wrong code," "expired code,"
	// and "no code was ever requested," deliberately not distinguished,
	// so a caller probing for valid identifiers/codes learns nothing from
	// the response shape.
	ErrInvalidOrExpiredCode = errors.New("invalid or expired code")
	// ErrTooManyAttempts backs the rate-limit lockout on both OTP flows —
	// a 6-digit code is only 1,000,000 possibilities, brute-forceable
	// without this.
	ErrTooManyAttempts = errors.New("too many attempts, try again later")
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

// AuthProvider distinguishes a password-based account (every account
// before OPS-068a, and still the only path for admin/supervisor/assignee)
// from a Google-authenticated one (requester-only, OPS-068a).
type AuthProvider string

const (
	AuthProviderLocal  AuthProvider = "local"
	AuthProviderGoogle AuthProvider = "google"
)

type User struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"displayName"`
	// PasswordHash is never serialized (OPS-067 made User JSON-visible for
	// the first time via GET/POST/PATCH /users — before that it only ever
	// lived inside the process, so no tag on this field had ever mattered).
	PasswordHash string `json:"-"`
	Roles        []Role `json:"roles"`
	IsActive     bool   `json:"isActive"`
	// RequiresPasswordChange is true for a brand-new admin-created account
	// or right after an admin/supervisor-triggered reset (OPS-067). Login
	// still succeeds, but middleware.RequirePasswordChangeCleared 403s
	// every other route until POST /auth/change-password clears it.
	RequiresPasswordChange bool `json:"requiresPasswordChange"`
	// CreatedByUserID is empty for the 4 bootstrap accounts (created by the
	// system at startup, not by an admin) and set to the acting admin's
	// user ID for every account created via POST /users.
	CreatedByUserID string `json:"createdByUserId,omitempty"`
	// AuthProvider and GoogleSubjectID back OPS-068a's Google signup path.
	// A Google-authenticated user has PasswordHash == "" (no password
	// exists to compare against — Login's password path naturally rejects
	// these via a failing bcrypt compare, no special-casing needed) and
	// RequiresPasswordChange == false (there's no password to force a
	// change on). GoogleSubjectID is Google's stable per-account "sub"
	// claim — nil for every local account, unique when set.
	AuthProvider    AuthProvider `json:"authProvider"`
	GoogleSubjectID *string      `json:"-"`
	// EmailVerified backs OPS-068b. True immediately for every
	// admin-created (#67) and Google-authenticated (#68a — Google already
	// verified that email) account; false on self-service signup until
	// VerifyEmail succeeds. middleware gates POST /workitems (not login,
	// not viewing) on this being true for requesters only.
	EmailVerified bool `json:"emailVerified"`
	// EmailVerificationCodeHash/ExpiresAt and PasswordResetCodeHash/
	// ExpiresAt hold a SHA-256 hash of the current OTP (never the raw
	// code — same reasoning as PasswordHash, though SHA-256 not bcrypt:
	// these are already-random 6-digit codes, not human-chosen secrets,
	// so a slow KDF buys nothing). Nil when no code is outstanding or
	// after one is successfully consumed (single-use).
	EmailVerificationCodeHash      *string    `json:"-"`
	EmailVerificationCodeExpiresAt *time.Time `json:"-"`
	PasswordResetCodeHash          *string    `json:"-"`
	PasswordResetCodeExpiresAt     *time.Time `json:"-"`
	// FailedOTPAttempts/OTPLockedUntil rate-limit both OTP flows
	// (OPS-068b) — shared across verify-email and reset-password since
	// both are the same underlying weakness (a guessable 6-digit code).
	// Reset to 0/nil on every successful verification; incremented on
	// every wrong guess; OTPLockedUntil set once the limit is hit.
	FailedOTPAttempts int        `json:"-"`
	OTPLockedUntil    *time.Time `json:"-"`
}

type Principal struct {
	UserID      string `json:"userId"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"displayName"`
	Roles       []Role `json:"roles"`
	// RequiresPasswordChange mirrors User.RequiresPasswordChange. It's
	// populated fresh from the live user record on every authenticated
	// request (see Service.Authenticate) rather than carried in the JWT
	// itself, so a password change takes effect immediately without
	// needing the caller to log in again for a new token.
	RequiresPasswordChange bool `json:"requiresPasswordChange"`
	// EmailVerified mirrors User.EmailVerified, refreshed from the live
	// user record on every authenticated request (see
	// Service.Authenticate) — same reasoning as RequiresPasswordChange.
	EmailVerified bool `json:"emailVerified"`
}

func (u User) HasRole(role Role) bool {
	for _, assignedRole := range u.Roles {
		if assignedRole == role {
			return true
		}
	}

	return false
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
