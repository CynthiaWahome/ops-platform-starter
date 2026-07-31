package router

import (
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/handlers"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
)

func New(cfg config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	authService, err := auth.NewBootstrapService(cfg)
	if err != nil {
		return nil, err
	}

	healthHandler := handlers.NewHealthHandler(cfg)
	authHandler := handlers.NewAuthHandler(authService)
	accessHandler := handlers.NewAccessHandler()

	mux.Handle("GET /health", healthHandler)
	mux.HandleFunc("POST /auth/login", authHandler.Login)
	mux.Handle("GET /auth/me", httpmiddleware.RequireAuth(authService, http.HandlerFunc(authHandler.Me)))
	mux.Handle(
		"GET /access/admin",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(accessHandler.AdminOnly), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"GET /access/assignee",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(accessHandler.AssigneeOnly), auth.RoleAssignee),
		),
	)

	return mux, nil
}
