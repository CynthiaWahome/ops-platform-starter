package auth

import (
	"context"
	"strings"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
)

type UserStore interface {
	FindByIdentifier(ctx context.Context, identifier string) (User, bool)
	FindByID(ctx context.Context, id string) (User, bool)
}

type StaticUserSeed struct {
	ID          string
	Identifier  string
	DisplayName string
	Password    string
	Roles       []Role
	IsActive    bool
}

type StaticUserStore struct {
	byID         map[string]User
	byIdentifier map[string]User
}

type Service struct {
	users     UserStore
	passwords PasswordManager
	tokens    TokenManager
}

func NewService(users UserStore, passwords PasswordManager, tokens TokenManager) Service {
	return Service{
		users:     users,
		passwords: passwords,
		tokens:    tokens,
	}
}

func NewBootstrapService(cfg config.Config) (Service, error) {
	passwords := NewBcryptPasswordManager(0)

	users, err := NewStaticUserStore(passwords, []StaticUserSeed{
		{
			ID:          "user-admin-001",
			Identifier:  cfg.BootstrapAdminIdentifier,
			DisplayName: cfg.BootstrapAdminDisplayName,
			Password:    cfg.BootstrapAdminPassword,
			Roles:       []Role{RoleAdmin},
			IsActive:    true,
		},
		{
			ID:          "user-assignee-001",
			Identifier:  cfg.BootstrapAssigneeIdentifier,
			DisplayName: cfg.BootstrapAssigneeDisplayName,
			Password:    cfg.BootstrapAssigneePassword,
			Roles:       []Role{RoleAssignee},
			IsActive:    true,
		},
	})
	if err != nil {
		return Service{}, err
	}

	tokens := NewJWTManager(cfg.AuthTokenSecret, "ops-platform-starter-backend", cfg.AuthTokenTTL)

	return NewService(users, passwords, tokens), nil
}

func NewStaticUserStore(passwords PasswordManager, seeds []StaticUserSeed) (StaticUserStore, error) {
	store := StaticUserStore{
		byID:         make(map[string]User, len(seeds)),
		byIdentifier: make(map[string]User, len(seeds)),
	}

	for _, seed := range seeds {
		passwordHash, err := passwords.Hash(seed.Password)
		if err != nil {
			return StaticUserStore{}, err
		}

		user := User{
			ID:           seed.ID,
			Identifier:   normalizeIdentifier(seed.Identifier),
			DisplayName:  seed.DisplayName,
			PasswordHash: passwordHash,
			Roles:        append([]Role(nil), seed.Roles...),
			IsActive:     seed.IsActive,
		}

		store.byID[user.ID] = user
		store.byIdentifier[user.Identifier] = user
	}

	return store, nil
}

func (s StaticUserStore) FindByIdentifier(_ context.Context, identifier string) (User, bool) {
	user, ok := s.byIdentifier[normalizeIdentifier(identifier)]
	return user, ok
}

func (s StaticUserStore) FindByID(_ context.Context, id string) (User, bool) {
	user, ok := s.byID[id]
	return user, ok
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

func (s Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	principal, _, err := s.tokens.Parse(rawToken)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	user, ok := s.users.FindByID(ctx, principal.UserID)
	if !ok || !user.IsActive {
		return Principal{}, ErrInvalidToken
	}

	return principalFromUser(user), nil
}

func principalFromUser(user User) Principal {
	return Principal{
		UserID:      user.ID,
		Identifier:  user.Identifier,
		DisplayName: user.DisplayName,
		Roles:       append([]Role(nil), user.Roles...),
	}
}

func normalizeIdentifier(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}
