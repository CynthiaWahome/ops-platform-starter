package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager interface {
	Issue(principal Principal) (issuedToken, error)
	Parse(rawToken string) (Principal, time.Time, error)
}

type JWTManager struct {
	issuer string
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

type authClaims struct {
	DisplayName string `json:"displayName"`
	Identifier  string `json:"identifier"`
	Roles       []Role `json:"roles"`
	jwt.RegisteredClaims
}

func NewJWTManager(secret, issuer string, ttl time.Duration) JWTManager {
	return JWTManager{
		issuer: issuer,
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}
}

func (m JWTManager) Issue(principal Principal) (issuedToken, error) {
	issuedAt := m.now()
	expiresAt := issuedAt.Add(m.ttl)

	claims := authClaims{
		DisplayName: principal.DisplayName,
		Identifier:  principal.Identifier,
		Roles:       principal.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   principal.UserID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(m.secret)
	if err != nil {
		return issuedToken{}, err
	}

	return issuedToken{
		Value:     signedToken,
		ExpiresAt: expiresAt,
	}, nil
}

func (m JWTManager) Parse(rawToken string) (Principal, time.Time, error) {
	claims := &authClaims{}
	parser := jwt.NewParser(
		jwt.WithTimeFunc(m.now),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)

	token, err := parser.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		return m.secret, nil
	})
	if err != nil {
		return Principal{}, time.Time{}, ErrInvalidToken
	}

	if !token.Valid {
		return Principal{}, time.Time{}, ErrInvalidToken
	}

	if claims.Subject == "" || claims.ExpiresAt == nil {
		return Principal{}, time.Time{}, ErrInvalidToken
	}

	if claims.Issuer != m.issuer {
		return Principal{}, time.Time{}, ErrInvalidToken
	}

	return Principal{
		UserID:      claims.Subject,
		Identifier:  claims.Identifier,
		DisplayName: claims.DisplayName,
		Roles:       claims.Roles,
	}, claims.ExpiresAt.Time, nil
}

func (m JWTManager) WithClock(now func() time.Time) JWTManager {
	m.now = now
	return m
}
