package router

import (
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/handlers"
)

func New(cfg config.Config) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/health", handlers.NewHealthHandler(cfg))

	return mux
}
