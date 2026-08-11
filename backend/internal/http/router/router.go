package router

import (
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/handlers"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

func New(cfg config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	authService, err := auth.NewBootstrapService(cfg)
	if err != nil {
		return nil, err
	}
	workItemService := workitems.NewService(workitems.NewMemoryStore(), workitems.NewMemoryStatusHistoryStore(), workitems.NewMemoryAssignmentStore())

	attachmentDiskStorage, err := attachments.NewLocalDiskStorage(cfg.AttachmentUploadDir)
	if err != nil {
		return nil, err
	}
	attachmentService := attachments.NewService(attachments.NewMemoryStore(), attachmentDiskStorage)

	healthHandler := handlers.NewHealthHandler(cfg)
	authHandler := handlers.NewAuthHandler(authService)
	accessHandler := handlers.NewAccessHandler()
	workItemHandler := handlers.NewWorkItemHandler(workItemService, attachmentService)
	attachmentHandler := handlers.NewAttachmentHandler(workItemService, attachmentService)

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
	mux.Handle(
		"POST /workitems",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Create), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"GET /workitems",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.List), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"GET /workitems/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetByID), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"PATCH /workitems/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Update), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"PATCH /workitems/{id}/status",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ChangeStatus), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/history",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ListStatusHistory), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Assign), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetAssignment), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment/accept",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.AcceptAssignment), auth.RoleAssignee),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment/decline",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.DeclineAssignment), auth.RoleAssignee),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/attachments",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.Upload), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/attachments",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.List), auth.RoleAdmin, auth.RoleAssignee),
		),
	)

	return mux, nil
}
