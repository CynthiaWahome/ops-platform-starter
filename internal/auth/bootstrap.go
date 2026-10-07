package auth

import (
	"context"

	"github.com/CynthiaWahome/ops-platform-starter/internal/config"
)

// BootstrapSeed is one of the 4 fixed-identity accounts created from env
// vars (see config.Config) — unchanged in spirit by OPS-067: these still
// log in exactly as before. What changed is where they live: a row in
// whichever UserStore is active (memory or Postgres), seeded once and then
// left alone, instead of being rebuilt from scratch — and the only users
// that could ever exist — on every process start.
type BootstrapSeed struct {
	ID          string
	Identifier  string
	DisplayName string
	Password    string
	Roles       []Role
}

// BootstrapSeedsFromConfig builds the 4 bootstrap seeds from cfg — the
// same 4 identities NewBootstrapService used to hardcode directly.
func BootstrapSeedsFromConfig(cfg config.Config) []BootstrapSeed {
	return []BootstrapSeed{
		{
			ID:          "user-admin-001",
			Identifier:  cfg.BootstrapAdminIdentifier,
			DisplayName: cfg.BootstrapAdminDisplayName,
			Password:    cfg.BootstrapAdminPassword,
			Roles:       []Role{RoleAdmin},
		},
		{
			ID:          "user-assignee-001",
			Identifier:  cfg.BootstrapAssigneeIdentifier,
			DisplayName: cfg.BootstrapAssigneeDisplayName,
			Password:    cfg.BootstrapAssigneePassword,
			Roles:       []Role{RoleAssignee},
		},
		{
			ID:          "user-supervisor-001",
			Identifier:  cfg.BootstrapSupervisorIdentifier,
			DisplayName: cfg.BootstrapSupervisorDisplayName,
			Password:    cfg.BootstrapSupervisorPassword,
			Roles:       []Role{RoleSupervisor},
		},
		{
			ID:          "user-requester-001",
			Identifier:  cfg.BootstrapRequesterIdentifier,
			DisplayName: cfg.BootstrapRequesterDisplayName,
			Password:    cfg.BootstrapRequesterPassword,
			Roles:       []Role{RoleRequester},
		},
	}
}

// SeedBootstrapUsers idempotently ensures the 4 bootstrap accounts exist in
// store — via UserStore.Seed, which never overwrites an existing row. An
// admin who's since deactivated or re-roled a bootstrap account (OPS-067
// makes that possible via PATCH /users/:id for the first time) keeps that
// change across a restart, the same as any other user now that accounts
// are persisted instead of rebuilt from scratch every boot.
func SeedBootstrapUsers(ctx context.Context, store UserStore, passwords PasswordManager, seeds []BootstrapSeed) error {
	for _, seed := range seeds {
		passwordHash, err := passwords.Hash(seed.Password)
		if err != nil {
			return err
		}

		if err := store.Seed(ctx, User{
			ID:           seed.ID,
			Identifier:   normalizeIdentifier(seed.Identifier),
			DisplayName:  seed.DisplayName,
			PasswordHash: passwordHash,
			Roles:        append([]Role(nil), seed.Roles...),
			IsActive:     true,
		}); err != nil {
			return err
		}
	}

	return nil
}
