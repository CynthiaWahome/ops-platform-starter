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

func New(cfg config.Config) (Server, error) {
	handler, err := router.New(cfg)
	if err != nil {
		return Server{}, err
	}

	return Server{
		httpServer: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

func (s Server) Start() error {
	return s.httpServer.ListenAndServe()
}
