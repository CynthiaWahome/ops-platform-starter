package auth

import (
	"crypto/rand"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

// tempPasswordCharset deliberately excludes visually ambiguous characters
// (0/O, 1/l/I) — a temp password is read off an admin's screen and typed in
// by hand by whoever it's for, at least once, before RequiresPasswordChange
// lets them set their own.
const tempPasswordCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%"

const tempPasswordLength = 14

// GenerateTempPassword returns a random, single-use password for a
// newly-created or reset account (OPS-067). It's cryptographically random
// (crypto/rand, not math/rand) because it's returned once in an API
// response and otherwise only ever stored as a bcrypt hash — the random
// source is the only thing standing between it and being guessable.
func GenerateTempPassword() (string, error) {
	result := make([]byte, tempPasswordLength)
	charsetSize := big.NewInt(int64(len(tempPasswordCharset)))

	for i := range result {
		n, err := rand.Int(rand.Reader, charsetSize)
		if err != nil {
			return "", err
		}

		result[i] = tempPasswordCharset[n.Int64()]
	}

	return string(result), nil
}

type PasswordManager interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type BcryptPasswordManager struct {
	cost int
}

func NewBcryptPasswordManager(cost int) BcryptPasswordManager {
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}

	return BcryptPasswordManager{cost: cost}
}

func (m BcryptPasswordManager) Hash(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), m.cost)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}

func (m BcryptPasswordManager) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
