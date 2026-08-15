// Package db is the one place in this codebase that knows Postgres exists.
// Every other package (workitems, teams, notifications, attachments)
// depends only on its own Store-shaped interfaces — this package just
// hands router.New a live connection pool and a way to bring the schema
// up to date, then gets out of the way. Nothing outside this package and
// the Postgres*Store implementations imports pgx directly.
//
// Migrate is a small hand-rolled runner rather than a third-party
// migration library — same instinct behind this repo's "net/http, not
// Gin" choice (see router.go): a full library (golang-migrate, in this
// case) pulls in a large transitive dependency tree, some of it test-only
// tooling for database backends this starter will never use, which bloats
// go.sum for no real benefit here. What a migration runner actually does —
// track which numbered .sql files have run, run whichever haven't, in
// order, each in its own transaction — is small and worth being able to
// read start to finish in one file.
package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Open creates a connection pool against dsn (a standard Postgres
// connection string, e.g. postgres://user:pass@host:5432/dbname) and
// confirms it's actually reachable with a Ping before handing it back —
// a pool that fails on the first real query, deep inside a request
// handler, is a much worse failure mode than failing loudly at startup.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}

// migration is one numbered .sql file pulled out of the embedded
// migrations directory. version is the leading number ("000003" ->
// "0003_create_assignments"), used both to order migrations and as the
// key recorded in schema_migrations once applied.
type migration struct {
	version string
	name    string
	upSQL   string
}

// Migrate brings the schema up to the latest migration in
// internal/db/migrations, embedded into the binary via go:embed so a
// deployed build never depends on the migration files existing on disk
// next to it.
//
// Only *.up.sql files are read here — this starter runs Migrate on every
// startup when DATABASE_URL is set (idempotent: already-applied
// migrations are skipped), and never rolls a migration back
// automatically. Down migrations exist in the same directory for a human
// to run by hand during development, not for this function to reach for.
//
// Everything below runs on one dedicated connection (pool.Acquire, not
// the pool directly), held for the entire function via a Postgres
// advisory lock. Without it, two replicas starting at the same moment
// against a fresh database could both see a given version as unapplied
// before either has recorded it, and both try to run the same
// non-idempotent CREATE TABLE/CREATE SEQUENCE statements — one of them
// fails with a duplicate-object error instead of just waiting its turn.
// An advisory lock is session-scoped (tied to this one connection, not
// released until explicitly unlocked or the connection closes), which is
// exactly why this can't just use the pool: a pooled Exec/QueryRow call
// could be served by a different underlying connection each time,
// silently losing the lock between statements. migrationLockKey is an
// arbitrary fixed number, unique to this application's use of advisory
// locks — its only requirement is being unlikely to collide with a lock
// key some other tool on the same database might pick.
const migrationLockKey = 8_675_309

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("db: acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("db: acquire migration lock: %w", err)
	}
	defer conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockKey)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("db: create schema_migrations: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("db: load embedded migrations: %w", err)
	}

	for _, m := range migrations {
		var alreadyApplied bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
			m.version,
		).Scan(&alreadyApplied); err != nil {
			return fmt.Errorf("db: check migration %s: %w", m.version, err)
		}

		if alreadyApplied {
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("db: begin migration %s: %w", m.version, err)
		}

		if _, err := tx.Exec(ctx, m.upSQL); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("db: apply migration %s (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, m.version,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("db: record migration %s: %w", m.version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("db: commit migration %s: %w", m.version, err)
		}
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	migrations := make([]migration, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		version, rest, ok := strings.Cut(name, "_")
		if !ok {
			continue
		}

		content, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    strings.TrimSuffix(rest, ".up.sql"),
			upSQL:   string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}
