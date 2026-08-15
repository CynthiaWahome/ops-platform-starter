package router

import (
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/handlers"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/notifications"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/teams"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

func New(cfg config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	authService, err := auth.NewBootstrapService(cfg)
	if err != nil {
		return nil, err
	}
	// Built before workItemService and passed in as its NotificationSink —
	// notifications.Service satisfies that interface by having a matching
	// Notify method, nothing more is needed to wire it in. Kept as its
	// own variable (not just an inline argument) because the notification
	// handler below needs the same instance to read notifications back.
	notificationService := notifications.NewService(notifications.NewMemoryStore())
	// Same shape as notificationService above — built first, passed in as
	// workItemService's TeamAuthority, and kept as its own variable because
	// the team handler below needs the same instance.
	teamService := teams.NewService(teams.NewMemoryStore(), teams.NewMemoryMembershipStore(), teams.NewMemorySupervisionStore())
	workItemService := workitems.NewService(workitems.NewMemoryStore(), workitems.NewMemoryStatusHistoryStore(), workitems.NewMemoryAssignmentStore(), workitems.NewMemoryAssignmentHistoryStore(), notificationService, teamService)

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
	teamHandler := handlers.NewTeamHandler(teamService)
	notificationHandler := handlers.NewNotificationHandler(notificationService)

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
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Create), auth.RoleAdmin, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.List), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetByID), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
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
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ChangeStatus), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/verify",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Verify), auth.RoleAdmin, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/flag",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Flag), auth.RoleAdmin, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/history",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ListStatusHistory), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/assignment-history",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ListAssignmentHistory), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Assign), auth.RoleAdmin, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetAssignment), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
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
		"POST /teams",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.Create), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"GET /teams",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.List), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"POST /teams/{id}/assignees",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.AddAssignee), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"POST /teams/{id}/supervisors",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.AddSupervisor), auth.RoleAdmin),
		),
	)
	mux.Handle(
		"DELETE /teams/{id}/supervisors/{userId}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.RemoveSupervisor), auth.RoleAdmin),
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
	mux.Handle(
		"GET /notifications",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(notificationHandler.List), auth.RoleAdmin, auth.RoleAssignee),
		),
	)
	mux.Handle(
		"POST /notifications/{id}/read",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(notificationHandler.MarkAsRead), auth.RoleAdmin, auth.RoleAssignee),
		),
	)

	return mux, nil
}
