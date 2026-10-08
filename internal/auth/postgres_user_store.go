package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/CynthiaWahome/ops-platform-starter/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresUserStore is UserStore backed by Postgres (OPS-067) — same
// contract as MemoryUserStore, same generated-id shape as every other
// Postgres*Store in this codebase ("user-" + zero-padded sequence value).
// Before this slice, auth never had a Postgres implementation at all: it
// was the one domain still rebuilt from env vars on every process start,
// even when DATABASE_URL was set for everything else.
type PostgresUserStore struct {
	pool *pgxpool.Pool
}

func NewPostgresUserStore(pool *pgxpool.Pool) *PostgresUserStore {
	return &PostgresUserStore{pool: pool}
}

const userColumns = `id, identifier, display_name, password_hash, roles, is_active, requires_password_change, created_by_user_id, auth_provider, google_subject_id, email_verified, email_verification_code_hash, email_verification_code_expires_at, password_reset_code_hash, password_reset_code_expires_at, failed_otp_attempts, otp_locked_until`

func (s *PostgresUserStore) FindByIdentifier(ctx context.Context, identifier string) (User, bool) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE identifier = $1`,
		normalizeIdentifier(identifier),
	)

	user, err := scanUser(row)
	if err != nil {
		return User{}, false
	}

	return user, true
}

func (s *PostgresUserStore) FindByID(ctx context.Context, id string) (User, bool) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`,
		id,
	)

	user, err := scanUser(row)
	if err != nil {
		return User{}, false
	}

	return user, true
}

// FindByGoogleSubjectID backs OPS-068a's repeat-login path — find the
// existing account for a Google identity rather than creating a duplicate
// on every login.
func (s *PostgresUserStore) FindByGoogleSubjectID(ctx context.Context, subjectID string) (User, bool) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE google_subject_id = $1`,
		subjectID,
	)

	user, err := scanUser(row)
	if err != nil {
		return User{}, false
	}

	return user, true
}

func (s *PostgresUserStore) Create(ctx context.Context, user User) (User, error) {
	row := db.Querier(ctx, s.pool).QueryRow(ctx, `
		WITH seq AS (SELECT nextval('users_seq') AS n)
		INSERT INTO users (
			id, identifier, display_name, password_hash, roles, is_active, requires_password_change, created_by_user_id, auth_provider, google_subject_id,
			email_verified, email_verification_code_hash, email_verification_code_expires_at, password_reset_code_hash, password_reset_code_expires_at, failed_otp_attempts, otp_locked_until
		)
		SELECT 'user-' || lpad(n::text, greatest(length(n::text), 4), '0'), $1, $2, $3, $4, $5, $6, nullif($7, ''), $8, $9, $10, $11, $12, $13, $14, $15, $16
		FROM seq
		RETURNING `+userColumns,
		normalizeIdentifier(user.Identifier), user.DisplayName, user.PasswordHash,
		rolesToStrings(user.Roles), user.IsActive, user.RequiresPasswordChange, user.CreatedByUserID,
		string(user.AuthProvider), user.GoogleSubjectID,
		user.EmailVerified, user.EmailVerificationCodeHash, user.EmailVerificationCodeExpiresAt,
		user.PasswordResetCodeHash, user.PasswordResetCodeExpiresAt, user.FailedOTPAttempts, user.OTPLockedUntil,
	)

	created, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrIdentifierTaken
		}

		return User{}, fmt.Errorf("auth: create user: %w", err)
	}

	return created, nil
}

func (s *PostgresUserStore) List(ctx context.Context) ([]User, error) {
	rows, err := db.Querier(ctx, s.pool).Query(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate users: %w", err)
	}

	return users, nil
}

func (s *PostgresUserStore) Update(ctx context.Context, user User) (User, error) {
	return execUpdate(ctx, db.Querier(ctx, s.pool), user)
}

// UpdateAtomic runs the whole find-mutate-write sequence inside one
// transaction, locking the row with SELECT ... FOR UPDATE before handing
// it to mutate — so no other transaction's own find-mutate-write can
// interleave in between (it blocks until this one commits or rolls
// back). See the UserStore interface doc comment for why this matters
// (OPS-068b's OTP consumption race) — a plain SELECT followed by a
// separate UPDATE, each its own round trip, would let two concurrent
// transactions both read the same pre-mutation row.
func (s *PostgresUserStore) UpdateAtomic(ctx context.Context, id string, mutate func(User) (User, error)) (User, error) {
	var result User

	err := db.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		row := db.Querier(txCtx, s.pool).QueryRow(txCtx, `SELECT `+userColumns+` FROM users WHERE id = $1 FOR UPDATE`, id)

		current, err := scanUser(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("auth: select user for update: %w", err)
		}

		updated, err := mutate(current)
		if err != nil {
			return err
		}

		result, err = execUpdate(txCtx, db.Querier(txCtx, s.pool), updated)
		return err
	})

	return result, err
}

// rowQuerier is the one method execUpdate needs from whatever
// db.Querier(ctx, pool) hands back — declared locally because the real
// type db.Querier returns is unexported outside internal/db; Go's
// structural typing means that value satisfies this interface anyway.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// execUpdate is the UPDATE statement Update and UpdateAtomic both run —
// pulled out once so UpdateAtomic's transaction-scoped querier and
// Update's plain pool-scoped one share the exact same SQL rather than two
// copies that could quietly drift apart.
func execUpdate(ctx context.Context, q rowQuerier, user User) (User, error) {
	row := q.QueryRow(ctx, `
		UPDATE users
		SET display_name = $2, password_hash = $3, roles = $4, is_active = $5, requires_password_change = $6,
			email_verified = $7, email_verification_code_hash = $8, email_verification_code_expires_at = $9,
			password_reset_code_hash = $10, password_reset_code_expires_at = $11, failed_otp_attempts = $12, otp_locked_until = $13
		WHERE id = $1
		RETURNING `+userColumns,
		user.ID, user.DisplayName, user.PasswordHash, rolesToStrings(user.Roles), user.IsActive, user.RequiresPasswordChange,
		user.EmailVerified, user.EmailVerificationCodeHash, user.EmailVerificationCodeExpiresAt,
		user.PasswordResetCodeHash, user.PasswordResetCodeExpiresAt, user.FailedOTPAttempts, user.OTPLockedUntil,
	)

	updated, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: update user: %w", err)
	}

	return updated, nil
}

func (s *PostgresUserStore) Seed(ctx context.Context, user User) error {
	_, err := db.Querier(ctx, s.pool).Exec(ctx, `
		INSERT INTO users (
			id, identifier, display_name, password_hash, roles, is_active, requires_password_change, created_by_user_id, auth_provider, google_subject_id,
			email_verified, email_verification_code_hash, email_verification_code_expires_at, password_reset_code_hash, password_reset_code_expires_at, failed_otp_attempts, otp_locked_until
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, ''), $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (id) DO NOTHING
	`, user.ID, normalizeIdentifier(user.Identifier), user.DisplayName, user.PasswordHash,
		rolesToStrings(user.Roles), user.IsActive, user.RequiresPasswordChange, user.CreatedByUserID,
		string(user.AuthProvider), user.GoogleSubjectID,
		user.EmailVerified, user.EmailVerificationCodeHash, user.EmailVerificationCodeExpiresAt,
		user.PasswordResetCodeHash, user.PasswordResetCodeExpiresAt, user.FailedOTPAttempts, user.OTPLockedUntil,
	)
	if err != nil {
		return fmt.Errorf("auth: seed user: %w", err)
	}

	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var (
		user         User
		roles        []string
		createdBy    *string
		authProvider string
	)

	if err := row.Scan(
		&user.ID, &user.Identifier, &user.DisplayName, &user.PasswordHash,
		&roles, &user.IsActive, &user.RequiresPasswordChange, &createdBy,
		&authProvider, &user.GoogleSubjectID,
		&user.EmailVerified, &user.EmailVerificationCodeHash, &user.EmailVerificationCodeExpiresAt,
		&user.PasswordResetCodeHash, &user.PasswordResetCodeExpiresAt, &user.FailedOTPAttempts, &user.OTPLockedUntil,
	); err != nil {
		return User{}, err
	}

	user.Roles = stringsToRoles(roles)
	user.AuthProvider = AuthProvider(authProvider)
	if createdBy != nil {
		user.CreatedByUserID = *createdBy
	}

	return user, nil
}

func rolesToStrings(roles []Role) []string {
	out := make([]string, len(roles))
	for i, role := range roles {
		out[i] = string(role)
	}

	return out
}

func stringsToRoles(values []string) []Role {
	out := make([]Role, len(values))
	for i, value := range values {
		out[i] = Role(value)
	}

	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
