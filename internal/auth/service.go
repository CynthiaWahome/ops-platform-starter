package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// UserStore is the persistence seam for user accounts. MemoryUserStore and
// PostgresUserStore both satisfy it — selected at startup by router.New the
// same way every other domain's store is (OPS-048's pattern, extended to
// auth for the first time in OPS-067; before this, auth never participated
// in that in-memory/Postgres split at all).
type UserStore interface {
	FindByIdentifier(ctx context.Context, identifier string) (User, bool)
	FindByID(ctx context.Context, id string) (User, bool)
	// FindByGoogleSubjectID backs OPS-068a's repeat-login path.
	FindByGoogleSubjectID(ctx context.Context, subjectID string) (User, bool)
	// Create inserts a brand-new user with a store-generated ID (sequence
	// in Postgres, incrementing counter in memory) — used for every
	// admin-created account (OPS-067's POST /users).
	Create(ctx context.Context, user User) (User, error)
	List(ctx context.Context) ([]User, error)
	// Update persists an in-place edit (role, active flag, password hash,
	// requires-password-change flag) to an existing row, matched by ID.
	Update(ctx context.Context, user User) (User, error)
	// UpdateAtomic finds the row by id and hands it to mutate, persisting
	// whatever mutate returns — the whole find-mutate-write sequence
	// under one lock (MemoryUserStore) or one row lock inside a
	// transaction (PostgresUserStore), so no other caller's
	// find-mutate-write can interleave in between. Added specifically for
	// OTP consumption (OPS-068b): a plain Find-then-separate-Update left
	// a real race — two concurrent requests could both read the same
	// not-yet-cleared code as valid before either committed, letting a
	// single-use code be consumed twice, and concurrent wrong guesses
	// could each compute "attempts+1" off the same stale read, losing
	// increments and defeating the lockout.
	UpdateAtomic(ctx context.Context, id string, mutate func(User) (User, error)) (User, error)
	// Seed inserts user if no row with its exact ID exists yet, and is a
	// no-op otherwise — see SeedBootstrapUsers. Unlike Create, the caller
	// supplies the ID (the 4 bootstrap accounts need fixed, predictable
	// IDs, not sequence-generated ones), and a second call must never
	// overwrite an admin's later edit (role change, deactivation) to that
	// same row on the next restart.
	Seed(ctx context.Context, user User) error
}

type Service struct {
	users     UserStore
	passwords PasswordManager
	tokens    TokenManager
	// email is nil unless an EmailSender was actually configured
	// (OPS-068b) — SignUp, RequestPasswordReset etc. are only ever called
	// from routes router.go mounts solely when email is configured, so
	// nil here would be a wiring bug, not a real-world state to guard
	// against defensively.
	email EmailSender
	now   func() time.Time
}

func NewService(users UserStore, passwords PasswordManager, tokens TokenManager, email EmailSender) Service {
	return Service{
		users:     users,
		passwords: passwords,
		tokens:    tokens,
		email:     email,
		now:       time.Now,
	}
}

func (s Service) Login(ctx context.Context, identifier, password string) (Session, error) {
	user, ok := s.users.FindByIdentifier(ctx, identifier)
	if !ok {
		return Session{}, ErrInvalidCredentials
	}

	if !user.IsActive {
		return Session{}, ErrInactiveUser
	}

	if err := s.passwords.Compare(user.PasswordHash, password); err != nil {
		return Session{}, ErrInvalidCredentials
	}

	return s.issueSession(user)
}

func (s Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	principal, _, err := s.tokens.Parse(rawToken)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	user, ok := s.users.FindByID(ctx, principal.UserID)
	if !ok || !user.IsActive {
		return Principal{}, ErrInvalidToken
	}

	// Re-derived from the live user record on every request, not just
	// trusted from the token's claims — this is what lets
	// RequiresPasswordChange (and a role change, or deactivation) take
	// effect immediately instead of only after the next login.
	return principalFromUser(user), nil
}

// CreateUserInput is what an admin supplies to provision a new internal
// account (OPS-067). Team assignment (TeamID) is handled by the caller
// (handlers.UsersHandler), which also has teams.Service — auth deliberately
// stays team-agnostic, the same way it stayed persistence-agnostic before
// this slice.
type CreateUserInput struct {
	Role        Role
	Identifier  string
	DisplayName string
}

// CreateUser provisions a new account with a server-generated temporary
// password, returned once (never emailed — ticket is explicit there's no
// email infra for internal users). RequiresPasswordChange starts true;
// every other route 403s for this user until they call ChangePassword.
func (s Service) CreateUser(ctx context.Context, actingUserID string, input CreateUserInput) (User, string, error) {
	identifier := normalizeIdentifier(input.Identifier)
	displayName := strings.TrimSpace(input.DisplayName)

	if identifier == "" || displayName == "" {
		return User{}, "", ErrInvalidUserInput
	}

	if !isKnownRole(input.Role) {
		return User{}, "", ErrInvalidRole
	}

	if _, exists := s.users.FindByIdentifier(ctx, identifier); exists {
		return User{}, "", ErrIdentifierTaken
	}

	tempPassword, err := GenerateTempPassword()
	if err != nil {
		return User{}, "", err
	}

	passwordHash, err := s.passwords.Hash(tempPassword)
	if err != nil {
		return User{}, "", err
	}

	created, err := s.users.Create(ctx, User{
		Identifier:             identifier,
		DisplayName:            displayName,
		PasswordHash:           passwordHash,
		Roles:                  []Role{input.Role},
		IsActive:               true,
		RequiresPasswordChange: true,
		CreatedByUserID:        actingUserID,
		AuthProvider:           AuthProviderLocal,
		EmailVerified:          true,
	})
	if err != nil {
		return User{}, "", err
	}

	return created, tempPassword, nil
}

// GetUser looks up a single account by ID — used by callers (e.g.
// handlers.UsersHandler.ResetPassword) that need to inspect a user's
// current role before acting on it, not just its ID.
func (s Service) GetUser(ctx context.Context, userID string) (User, error) {
	user, ok := s.users.FindByID(ctx, userID)
	if !ok {
		return User{}, ErrUserNotFound
	}

	return user, nil
}

// ListUsers returns every account, unfiltered. Role/team scoping (admin
// sees all, supervisor sees only their own team) is applied by the caller
// (handlers.UsersHandler) via teams.Service — same reason as
// CreateUserInput above.
func (s Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.users.List(ctx)
}

// UpdateUserInput is a partial patch — a nil field is left unchanged. Team
// reassignment goes through teams.Service.AddAssignee/AddSupervisor
// directly (called by handlers.UsersHandler), not through this method.
type UpdateUserInput struct {
	IsActive *bool
	Role     *Role
}

func (s Service) UpdateUser(ctx context.Context, userID string, input UpdateUserInput) (User, error) {
	user, ok := s.users.FindByID(ctx, userID)
	if !ok {
		return User{}, ErrUserNotFound
	}

	if input.Role != nil {
		if !isKnownRole(*input.Role) {
			return User{}, ErrInvalidRole
		}

		user.Roles = []Role{*input.Role}
	}

	if input.IsActive != nil {
		user.IsActive = *input.IsActive
	}

	return s.users.Update(ctx, user)
}

// ResetPassword re-triggers the temp-password flow on an existing user —
// this *is* the password-reset story for internal users (admin-initiated,
// not self-service, since there's no email infra for reset links). Caller
// is responsible for the admin/own-team-supervisor authorization check.
func (s Service) ResetPassword(ctx context.Context, userID string) (string, error) {
	user, ok := s.users.FindByID(ctx, userID)
	if !ok {
		return "", ErrUserNotFound
	}

	tempPassword, err := GenerateTempPassword()
	if err != nil {
		return "", err
	}

	passwordHash, err := s.passwords.Hash(tempPassword)
	if err != nil {
		return "", err
	}

	user.PasswordHash = passwordHash
	user.RequiresPasswordChange = true

	if _, err := s.users.Update(ctx, user); err != nil {
		return "", err
	}

	return tempPassword, nil
}

// ChangePassword is any authenticated user's own escape hatch out of
// RequiresPasswordChange — the one write every account, including one
// currently gated by that flag, is always allowed to make (enforced by
// router.go exempting this route from the gate, not by anything in here).
func (s Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	user, ok := s.users.FindByID(ctx, userID)
	if !ok {
		return ErrUserNotFound
	}

	if err := s.passwords.Compare(user.PasswordHash, oldPassword); err != nil {
		return ErrInvalidCredentials
	}

	// Length is checked against the trimmed value (so a string of nothing
	// but spaces can't pass the >=8 rule), but the password that actually
	// gets hashed is the raw, untrimmed input — a review caught that
	// trimming before Hash here, while Login never trims what's typed,
	// meant a password with leading/trailing spaces could never be typed
	// back in to match. bcrypt silently truncates past 72 bytes, so that's
	// rejected explicitly rather than hashing a shorter password than the
	// one the caller thinks they set.
	if len(strings.TrimSpace(newPassword)) < 8 {
		return ErrInvalidUserInput
	}
	if len(newPassword) > 72 {
		return ErrInvalidUserInput
	}

	passwordHash, err := s.passwords.Hash(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = passwordHash
	user.RequiresPasswordChange = false

	_, err = s.users.Update(ctx, user)
	return err
}

// GoogleIdentity is what the OAuth handler hands the service after
// exchanging a Google authorization code — just the 3 fields OPS-068a
// actually needs, independent of whatever the real Google API response
// shape looks like.
type GoogleIdentity struct {
	Subject string
	Email   string
	Name    string
}

// LoginWithGoogle is the requester self-service signup path (OPS-068a).
// First login for a given Google subject auto-creates a requester-role
// account with no team and no password; every later login for the same
// subject finds that same row rather than creating a duplicate. Linking a
// Google identity to an existing local account is explicitly out of
// scope — if the email already belongs to one, this returns
// ErrGoogleEmailAlreadyRegistered rather than silently merging or
// silently creating a second, confusing account under the same email.
func (s Service) LoginWithGoogle(ctx context.Context, identity GoogleIdentity) (Session, error) {
	if existing, ok := s.users.FindByGoogleSubjectID(ctx, identity.Subject); ok {
		if !existing.IsActive {
			return Session{}, ErrInactiveUser
		}

		return s.issueSession(existing)
	}

	identifier := normalizeIdentifier(identity.Email)

	if _, exists := s.users.FindByIdentifier(ctx, identifier); exists {
		return Session{}, ErrGoogleEmailAlreadyRegistered
	}

	subjectID := identity.Subject

	created, err := s.users.Create(ctx, User{
		Identifier:      identifier,
		DisplayName:     identity.Name,
		Roles:           []Role{RoleRequester},
		IsActive:        true,
		AuthProvider:    AuthProviderGoogle,
		GoogleSubjectID: &subjectID,
		// Google already verified this email as part of its own signup
		// flow — re-verifying it here would be redundant and bad UX.
		EmailVerified: true,
	})
	if err != nil {
		return Session{}, err
	}

	return s.issueSession(created)
}

func (s Service) issueSession(user User) (Session, error) {
	principal := principalFromUser(user)

	token, err := s.tokens.Issue(principal)
	if err != nil {
		return Session{}, err
	}

	return Session{
		AccessToken: token.Value,
		ExpiresAt:   token.ExpiresAt,
		Principal:   principal,
	}, nil
}

func (s Service) WithClock(now func() time.Time) Service {
	s.now = now
	return s
}

// SignUpInput is what a requester supplies to self-service signup
// (OPS-068b) — unlike CreateUserInput, the caller chooses their own
// password and the role is always RoleRequester; the other 3 roles are
// always admin-provisioned (#67), never self-signup.
type SignUpInput struct {
	Identifier  string
	Password    string
	DisplayName string
}

// SignUp creates a requester account with the caller's own chosen
// password, emails a 6-digit verification OTP, and returns a session —
// the account can log in and view immediately, but
// middleware.RequireVerifiedEmailForRequester blocks creating a work
// item until VerifyEmail succeeds.
func (s Service) SignUp(ctx context.Context, input SignUpInput) (Session, error) {
	identifier := normalizeIdentifier(input.Identifier)
	displayName := strings.TrimSpace(input.DisplayName)

	if identifier == "" || displayName == "" {
		return Session{}, ErrInvalidUserInput
	}

	trimmedPassword := strings.TrimSpace(input.Password)
	if len(trimmedPassword) < 8 || len(input.Password) > 72 {
		return Session{}, ErrInvalidUserInput
	}

	if _, exists := s.users.FindByIdentifier(ctx, identifier); exists {
		return Session{}, ErrIdentifierTaken
	}

	passwordHash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return Session{}, err
	}

	// Generate and send the verification code BEFORE persisting the
	// account — a review caught that doing this the other way around
	// (create first, then send) left an orphaned account on any send
	// failure: the row existed, requires EmailVerified forever, and its
	// one-time code — had Update already written it — could never reach
	// anyone, since the email that was supposed to carry it never
	// arrived. Generating the code needs no user ID, so there's no
	// reason to create the row first.
	code, err := GenerateOTP()
	if err != nil {
		return Session{}, err
	}

	if err := s.email.Send(ctx, identifier, "Verify your email",
		fmt.Sprintf("Your verification code is %s. It expires in %d minutes.", code, int(otpTTL.Minutes()))); err != nil {
		return Session{}, err
	}

	hash := hashOTP(code)
	expiresAt := s.now().Add(otpTTL)

	created, err := s.users.Create(ctx, User{
		Identifier:                     identifier,
		DisplayName:                    displayName,
		PasswordHash:                   passwordHash,
		Roles:                          []Role{RoleRequester},
		IsActive:                       true,
		AuthProvider:                   AuthProviderLocal,
		EmailVerified:                  false,
		EmailVerificationCodeHash:      &hash,
		EmailVerificationCodeExpiresAt: &expiresAt,
	})
	if err != nil {
		return Session{}, err
	}

	return s.issueSession(created)
}

// VerifyEmail checks a 6-digit code against the caller's own account
// (authenticated — the handler passes the principal's own user ID, not
// an arbitrary target). Rate-limited the same way as
// CompletePasswordReset: otpMaxAttempts wrong guesses locks OTP flows for
// otpLockoutDuration.
func (s Service) VerifyEmail(ctx context.Context, userID, code string) error {
	return s.consumeOTP(ctx, userID, code,
		func(u User) (*string, *time.Time) {
			return u.EmailVerificationCodeHash, u.EmailVerificationCodeExpiresAt
		},
		func(u *User) {
			u.EmailVerified = true
			u.EmailVerificationCodeHash = nil
			u.EmailVerificationCodeExpiresAt = nil
		},
	)
}

// RequestPasswordReset always succeeds from the caller's point of view —
// it never reveals whether identifier belongs to an account, and only
// ever actually sends an email for an existing *requester* account (the
// other 3 roles use admin-initiated reset, #67, by design: there's no
// email infra story for them, and an external stranger resetting an
// internal account's password isn't a flow this starter wants to enable
// even if email were configured for them too).
func (s Service) RequestPasswordReset(ctx context.Context, identifier string) error {
	user, ok := s.users.FindByIdentifier(ctx, identifier)
	if !ok || !user.HasRole(RoleRequester) || user.AuthProvider != AuthProviderLocal {
		return nil
	}

	code, err := GenerateOTP()
	if err != nil {
		return err
	}

	hash := hashOTP(code)
	expiresAt := s.now().Add(otpTTL)
	user.PasswordResetCodeHash = &hash
	user.PasswordResetCodeExpiresAt = &expiresAt

	if _, err := s.users.Update(ctx, user); err != nil {
		return err
	}

	// Dispatched asynchronously and the Send error deliberately swallowed
	// — a review caught that a synchronous send here made an existing
	// account's response measurably slower than a nonexistent one's,
	// which is itself a timing side-channel leaking account existence
	// even though the response body never does (this method's whole
	// contract). Detached from ctx with its own bounded timeout, since
	// ctx is cancelled the moment the HTTP handler returns — the send
	// still has to be able to complete after that. Once there's
	// server-side logging (issue #72), that's the right place to surface
	// a real delivery failure — silently to an operator, never to the
	// caller.
	recipient := user.Identifier
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()

		_ = s.email.Send(sendCtx, recipient, "Reset your password",
			fmt.Sprintf("Your password reset code is %s. It expires in %d minutes.", code, int(otpTTL.Minutes())))
	}()

	return nil
}

// CompletePasswordReset verifies a 6-digit code against identifier and,
// on success, sets newPassword. Unauthenticated by design (this is the
// one account-recovery path for someone who, by definition, might not be
// able to log in) — the code itself is the proof of identity.
//
// The code is consumed (atomically, via consumeOTP) before newPassword is
// validated, deliberately in that order — not the other way around. A
// review caught that validating the password first would let an attacker
// send unlimited code guesses paired with a deliberately-malformed
// password (always failing fast, before ever touching the attempt
// counter), brute-forcing the code completely free of the lockout this
// slice exists to enforce. The accepted tradeoff: a correct code paired
// with a bad password still gets consumed, so fixing a password typo
// needs a fresh forgot-password call rather than a retry with the same
// code — a minor UX cost for closing a real rate-limit bypass.
func (s Service) CompletePasswordReset(ctx context.Context, identifier, code, newPassword string) error {
	user, ok := s.users.FindByIdentifier(ctx, identifier)
	if !ok {
		return ErrInvalidOrExpiredCode
	}

	if err := s.consumeOTP(ctx, user.ID, code,
		func(u User) (*string, *time.Time) { return u.PasswordResetCodeHash, u.PasswordResetCodeExpiresAt },
		func(u *User) {
			u.PasswordResetCodeHash = nil
			u.PasswordResetCodeExpiresAt = nil
		},
	); err != nil {
		return err
	}

	trimmedNew := strings.TrimSpace(newPassword)
	if len(trimmedNew) < 8 || len(newPassword) > 72 {
		return ErrInvalidUserInput
	}

	passwordHash, err := s.passwords.Hash(newPassword)
	if err != nil {
		return err
	}

	// Re-fetched rather than reusing the User struct from above: that
	// copy predates consumeOTP's atomic mutation (code cleared, attempt
	// counter reset), and a blind Update with the stale copy would
	// silently undo that write.
	current, ok := s.users.FindByID(ctx, user.ID)
	if !ok {
		return ErrUserNotFound
	}

	current.PasswordHash = passwordHash

	_, err = s.users.Update(ctx, current)
	return err
}

// otpCheckResult carries consumeOTP's mutate-closure outcome back to the
// caller — valid is whether the code matched; locked is set when the
// account was already in its lockout window (checked, and left
// unmodified, inside the same atomic operation rather than as a separate
// read beforehand, which itself would have been racy).
type otpCheckResult struct {
	valid  bool
	locked bool
}

// consumeOTP is the one piece of logic both OTP flows share: is the
// account currently locked out, does the code match and hasn't expired —
// and, atomically in the same store operation, either apply onSuccess (on
// a correct code) or record the failed attempt and lock the account out
// once otpMaxAttempts is reached (on a wrong one). Everything happens
// inside UserStore.UpdateAtomic's single find-mutate-write, closing a
// real race a review caught: the previous Find-then-separate-Update
// design let two concurrent requests both read the same not-yet-cleared
// code as valid before either committed (a single-use code consumed
// twice), and let concurrent wrong guesses each compute "attempts+1" off
// the same stale read (losing increments, defeating the lockout).
func (s Service) consumeOTP(ctx context.Context, userID, code string, getCode func(User) (*string, *time.Time), onSuccess func(*User)) error {
	now := s.now()
	var result otpCheckResult

	_, err := s.users.UpdateAtomic(ctx, userID, func(user User) (User, error) {
		if user.OTPLockedUntil != nil && now.Before(*user.OTPLockedUntil) {
			result.locked = true
			return user, nil
		}

		storedHash, expiresAt := getCode(user)
		valid := storedHash != nil && expiresAt != nil && now.Before(*expiresAt) && verifyOTP(code, *storedHash)

		if valid {
			result.valid = true
			onSuccess(&user)
			user.FailedOTPAttempts = 0
			user.OTPLockedUntil = nil
			return user, nil
		}

		user.FailedOTPAttempts++
		if user.FailedOTPAttempts >= otpMaxAttempts {
			lockedUntil := now.Add(otpLockoutDuration)
			user.OTPLockedUntil = &lockedUntil
		}

		return user, nil
	})
	if err != nil {
		return err
	}

	if result.locked {
		return ErrTooManyAttempts
	}
	if !result.valid {
		return ErrInvalidOrExpiredCode
	}

	return nil
}

func isKnownRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleAssignee, RoleSupervisor, RoleRequester:
		return true
	default:
		return false
	}
}

func principalFromUser(user User) Principal {
	return Principal{
		UserID:                 user.ID,
		Identifier:             user.Identifier,
		DisplayName:            user.DisplayName,
		Roles:                  append([]Role(nil), user.Roles...),
		RequiresPasswordChange: user.RequiresPasswordChange,
		EmailVerified:          user.EmailVerified,
	}
}

func normalizeIdentifier(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}
