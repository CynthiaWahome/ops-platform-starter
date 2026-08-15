package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is the shape every Postgres*Store method actually needs — Exec,
// Query, QueryRow — and it's satisfied structurally by both *pgxpool.Pool
// and pgx.Tx. Every store in this codebase calls Querier(ctx, s.pool)
// once at the top of each method instead of using s.pool directly, so the
// exact same store code runs whether or not it's inside a transaction —
// the store never needs an if/else for "am I in a tx right now."
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Querier returns the active transaction stashed in ctx by WithTx, if
// there is one, or pool itself otherwise. This is the one function every
// Postgres*Store method calls before doing anything — it's what makes a
// whole group of store calls (see workitems.Service.runAtomic) atomic
// without any store needing to know a transaction exists at all.
func Querier(ctx context.Context, pool *pgxpool.Pool) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}

	return pool
}

// WithTx begins a real Postgres transaction, runs fn with a context that
// carries it, and commits on success or rolls back on any error fn
// returns. This is what closes the P1 a review caught on PR #57:
// multi-store workflow methods like AssignWorkItem write to four
// different tables (work_items, assignments, assignment_history,
// notifications) as four independent statements. Without this, a failure
// on the third write left the first two permanently committed — the work
// item ends up "assigned" with no assignment record, and a retry gets
// rejected because the status already moved past where the retry expects
// to start from. Wrapping the whole sequence in one transaction means
// either all four writes land, or none of them do.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit transaction: %w", err)
	}

	return nil
}

// PoolTxRunner adapts a *pgxpool.Pool to workitems.TxRunner's
// WithTx(ctx, fn) error shape (no pool parameter) — defined here, in the
// one package that's allowed to know Postgres exists, and wired in by
// router.New only when DATABASE_URL is set. workitems never imports this
// package; it depends only on the small interface it declares itself.
type PoolTxRunner struct {
	Pool *pgxpool.Pool
}

func (r PoolTxRunner) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithTx(ctx, r.Pool, fn)
}
