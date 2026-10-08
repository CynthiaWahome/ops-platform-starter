package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// otpLength is a standard 6-digit code — the same shape as nearly every
// email/SMS OTP a user has ever seen (bank 2FA, most SaaS signup flows).
const otpLength = 6

// otpTTL is intentionally short: a code this guessable (1,000,000
// possibilities) should have a narrow window to be guessed in, on top of
// the attempt-lockout in Service.
const otpTTL = 10 * time.Minute

// otpMaxAttempts and otpLockoutDuration implement OPS-068b's rate-limit —
// 5 wrong guesses locks the account's OTP flows (not the account itself,
// login still works) for 15 minutes. Per-account only, not per-IP: this
// starter doesn't track request IPs anywhere else either, and per-account
// is enough to make brute-forcing a specific target impractical, which is
// the actual threat an OTP needs to resist.
const (
	otpMaxAttempts     = 5
	otpLockoutDuration = 15 * time.Minute
)

// GenerateOTP returns a random 6-digit numeric code as a string (e.g.
// "042918" — zero-padded, not reformatted to a smaller number).
func GenerateOTP() (string, error) {
	max := big.NewInt(1)
	for range otpLength {
		max.Mul(max, big.NewInt(10))
	}

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("auth: generate otp: %w", err)
	}

	return fmt.Sprintf("%0*d", otpLength, n.Int64()), nil
}

// hashOTP is a fast cryptographic hash (SHA-256), deliberately not bcrypt
// — an OTP is already a random 6-digit value, not a human-chosen secret,
// so a slow KDF defends against nothing here and only adds latency.
func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// verifyOTP does a constant-time comparison of code's hash against the
// stored hash — same reasoning as any secret comparison (session tokens,
// API keys): a variable-time compare lets a timing attack narrow down the
// correct value one byte at a time.
func verifyOTP(code, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(hashOTP(code)), []byte(storedHash)) == 1
}
