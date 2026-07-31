package server

import (
	"net/http"
	"time"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/router"
)

type Server struct {
	httpServer *http.Server
}

func New(cfg config.Config) Server {
	return Server{
		httpServer: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           router.New(cfg),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s Server) Start() error {
	return s.httpServer.ListenAndServe()
}
