package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testUserPool opens a fresh pool against TEST_DATABASE_URL, migrates it,
// and truncates the users table before the test runs — same "skip if not
// configured" opt-in shape as internal/db's and internal/workitems' own
// Postgres tests, so `go test ./...` never requires a real Postgres
// instance.
func testUserPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping Postgres integration test")
	}

	ctx := context.Background()

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("expected pool to open, got error: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("expected migrate to succeed, got error: %v", err)
	}

	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatalf("expected truncate to succeed, got error: %v", err)
	}

	return pool
}

// TestPostgresUserStoreRoundTripsEmailVerificationAndResetFields is
// exactly the test that should have existed before this slice shipped —
// it would have caught, immediately and automatically, the real bug a
// live manual test against real Postgres caught instead: Update's SET
// clause never touched email_verified, the OTP code hash/expiry fields,
// or the rate-limit counters, so every one of those writes silently
// vanished in Postgres mode while appearing to work fine against
// MemoryUserStore (which copies the whole struct, masking the gap
// completely). This proves the full round trip through Create and
// Update, the same way TestPostgresStoreSurvivesPoolRestart proves
// workitems' actual persistence story.
func TestPostgresUserStoreRoundTripsEmailVerificationAndResetFields(t *testing.T) {
	pool := testUserPool(t)
	store := NewPostgresUserStore(pool)
	ctx := context.Background()

	expiresAt := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	codeHash := hashOTP("123456")

	created, err := store.Create(ctx, User{
		Identifier:                     "postgres-otp-test@gmail.com",
		DisplayName:                    "Postgres OTP Test",
		PasswordHash:                   "irrelevant-hash",
		Roles:                          []Role{RoleRequester},
		IsActive:                       true,
		AuthProvider:                   AuthProviderLocal,
		EmailVerified:                  false,
		EmailVerificationCodeHash:      &codeHash,
		EmailVerificationCodeExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if created.EmailVerified {
		t.Fatal("expected EmailVerified to be false immediately after create")
	}
	if created.EmailVerificationCodeHash == nil || *created.EmailVerificationCodeHash != codeHash {
		t.Fatalf("expected the verification code hash to round-trip through Create, got %v", created.EmailVerificationCodeHash)
	}
	if created.EmailVerificationCodeExpiresAt == nil || !created.EmailVerificationCodeExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected the verification code expiry to round-trip through Create, got %v", created.EmailVerificationCodeExpiresAt)
	}

	// Clear the code and mark verified via Update, then prove it with a
	// separate FindByID rather than only checking Update's own return
	// value — belt and suspenders, since Update's RETURNING already
	// reflects the real row (it would have caught the original bug on
	// its own: SET never mentioning these columns meant RETURNING came
	// back with the row's *old* values, not what was passed in).
	created.EmailVerified = true
	created.EmailVerificationCodeHash = nil
	created.EmailVerificationCodeExpiresAt = nil
	created.FailedOTPAttempts = 2

	if _, err := store.Update(ctx, created); err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}

	refetched, ok := store.FindByID(ctx, created.ID)
	if !ok {
		t.Fatal("expected to find the user after update")
	}

	if !refetched.EmailVerified {
		t.Fatal("expected EmailVerified=true to have actually persisted in Postgres")
	}
	if refetched.EmailVerificationCodeHash != nil {
		t.Fatalf("expected the verification code hash to be cleared in Postgres, got %v", *refetched.EmailVerificationCodeHash)
	}
	if refetched.FailedOTPAttempts != 2 {
		t.Fatalf("expected FailedOTPAttempts=2 to have actually persisted in Postgres, got %d", refetched.FailedOTPAttempts)
	}
}

// TestPostgresUserStoreUpdateAtomicAppliesMutation proves the basic
// contract — not found, mutate's return value persisted, re-fetchable —
// before the concurrency test below proves the actual point of adding
// UpdateAtomic.
func TestPostgresUserStoreUpdateAtomicAppliesMutation(t *testing.T) {
	pool := testUserPool(t)
	store := NewPostgresUserStore(pool)
	ctx := context.Background()

	created, err := store.Create(ctx, User{
		Identifier:   "atomic-basic@gmail.com",
		DisplayName:  "Atomic Basic",
		PasswordHash: "irrelevant-hash",
		Roles:        []Role{RoleRequester},
		IsActive:     true,
		AuthProvider: AuthProviderLocal,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	updated, err := store.UpdateAtomic(ctx, created.ID, func(u User) (User, error) {
		u.FailedOTPAttempts = 7
		return u, nil
	})
	if err != nil {
		t.Fatalf("expected UpdateAtomic to succeed, got error: %v", err)
	}
	if updated.FailedOTPAttempts != 7 {
		t.Fatalf("expected the mutated value to come back from UpdateAtomic, got %d", updated.FailedOTPAttempts)
	}

	refetched, ok := store.FindByID(ctx, created.ID)
	if !ok || refetched.FailedOTPAttempts != 7 {
		t.Fatalf("expected FailedOTPAttempts=7 to have actually persisted, got %+v", refetched)
	}

	_, err = store.UpdateAtomic(ctx, "user-does-not-exist", func(u User) (User, error) { return u, nil })
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound for a missing id, got %v", err)
	}
}

// TestPostgresUserStoreUpdateAtomicSerializesConcurrentMutations is the
// real proof for Postgres specifically: SELECT ... FOR UPDATE inside a
// transaction should make one concurrent caller's find-mutate-write wait
// for another's to fully commit, rather than both reading the same
// pre-mutation row the way two separate, un-transactioned
// SELECT-then-UPDATE statements could. Fires concurrent increments at the
// same counter and asserts none of them were lost — a plain SELECT
// followed by a separate UPDATE (no row lock, no shared transaction)
// would lose some of these under real concurrent load.
func TestPostgresUserStoreUpdateAtomicSerializesConcurrentMutations(t *testing.T) {
	pool := testUserPool(t)
	store := NewPostgresUserStore(pool)
	ctx := context.Background()

	created, err := store.Create(ctx, User{
		Identifier:   "atomic-race@gmail.com",
		DisplayName:  "Atomic Race",
		PasswordHash: "irrelevant-hash",
		Roles:        []Role{RoleRequester},
		IsActive:     true,
		AuthProvider: AuthProviderLocal,
	})
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	const concurrentIncrements = 20

	var wg sync.WaitGroup
	errs := make([]error, concurrentIncrements)

	for i := range concurrentIncrements {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = store.UpdateAtomic(ctx, created.ID, func(u User) (User, error) {
				u.FailedOTPAttempts++
				return u, nil
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("expected increment %d to succeed, got error: %v", i, err)
		}
	}

	refetched, ok := store.FindByID(ctx, created.ID)
	if !ok {
		t.Fatal("expected to find the user after the concurrent increments")
	}
	if refetched.FailedOTPAttempts != concurrentIncrements {
		t.Fatalf("expected all %d concurrent increments to land with none lost, got %d", concurrentIncrements, refetched.FailedOTPAttempts)
	}
}
