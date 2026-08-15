package server

import (
	"context"
	"net/http"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/router"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	httpServer *http.Server
	// dbPool is nil unless cfg.DatabaseURL was set (OPS-048) — Close is a
	// no-op in that case, same as calling Close on a nil *pgxpool.Pool
	// would panic, so it's guarded explicitly below rather than assumed
	// safe.
	dbPool *pgxpool.Pool
}

func New(ctx context.Context, cfg config.Config) (Server, error) {
	handler, pool, err := router.New(ctx, cfg)
	if err != nil {
		return Server{}, err
	}

	return Server{
		httpServer: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		dbPool: pool,
	}, nil
}

func (s Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown stops accepting new requests and waits for in-flight ones to
// finish, then closes the database pool if one was opened. Introduced
// alongside OPS-048 rather than before it: a leaked in-memory map costs
// nothing on process exit, but a real Postgres connection pool left open
// on every restart is exactly the kind of thing worth closing cleanly
// once there's an actual connection to close.
func (s Server) Shutdown(ctx context.Context) error {
	err := s.httpServer.Shutdown(ctx)

	if s.dbPool != nil {
		s.dbPool.Close()
	}

	return err
}
