package auth

import (
	"context"
	"os"
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
