package auth

import "golang.org/x/crypto/bcrypt"

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
