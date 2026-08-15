package router

import (
	"context"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/db"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/handlers"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/notifications"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/teams"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New builds the full HTTP handler. It also returns the Postgres pool it
// opened, if any (OPS-048) — nil when cfg.DatabaseURL is empty — so the
// caller (server.New) can close it on shutdown. The router itself never
// holds a reference to the pool once construction is done; only the
// Postgres*Store values built from it do.
func New(ctx context.Context, cfg config.Config) (http.Handler, *pgxpool.Pool, error) {
	mux := http.NewServeMux()

	authService, err := auth.NewBootstrapService(cfg)
	if err != nil {
		return nil, nil, err
	}

	var (
		workItemStore          workitems.Store
		statusHistoryStore     workitems.StatusHistoryStore
		assignmentStore        workitems.AssignmentStore
		assignmentHistoryStore workitems.AssignmentHistoryStore
		teamStore              teams.Store
		membershipStore        teams.MembershipStore
		supervisionStore       teams.SupervisionStore
		notificationStore      notifications.Store
		attachmentMetaStore    attachments.Store
		pool                   *pgxpool.Pool
		txRunner               workitems.TxRunner
	)

	// cfg.DatabaseURL empty is the default, zero-setup path every test
	// and every `go run ./cmd/api` has used since before OPS-048 — the
	// in-memory stores, unchanged. Setting DATABASE_URL is what opts a
	// deployment into real persistence (OPS-048): open a pool, bring the
	// schema up to date, and swap every Postgres*Store in instead.
	if cfg.DatabaseURL == "" {
		workItemStore = workitems.NewMemoryStore()
		statusHistoryStore = workitems.NewMemoryStatusHistoryStore()
		assignmentStore = workitems.NewMemoryAssignmentStore()
		assignmentHistoryStore = workitems.NewMemoryAssignmentHistoryStore()
		teamStore = teams.NewMemoryStore()
		membershipStore = teams.NewMemoryMembershipStore()
		supervisionStore = teams.NewMemorySupervisionStore()
		notificationStore = notifications.NewMemoryStore()
		attachmentMetaStore = attachments.NewMemoryStore()
	} else {
		pool, err = db.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}

		if err := db.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, err
		}

		workItemStore = workitems.NewPostgresStore(pool)
		statusHistoryStore = workitems.NewPostgresStatusHistoryStore(pool)
		assignmentStore = workitems.NewPostgresAssignmentStore(pool)
		assignmentHistoryStore = workitems.NewPostgresAssignmentHistoryStore(pool)
		teamStore = teams.NewPostgresStore(pool)
		membershipStore = teams.NewPostgresMembershipStore(pool)
		supervisionStore = teams.NewPostgresSupervisionStore(pool)
		notificationStore = notifications.NewPostgresStore(pool)
		attachmentMetaStore = attachments.NewPostgresStore(pool)
		// txRunner stays nil in the in-memory branch above — see
		// workitems.TxRunner's doc comment for why that's correct, not
		// just unimplemented.
		txRunner = db.PoolTxRunner{Pool: pool}
	}

	// Built before workItemService and passed in as its NotificationSink —
	// notifications.Service satisfies that interface by having a matching
	// Notify method, nothing more is needed to wire it in. Kept as its
	// own variable (not just an inline argument) because the notification
	// handler below needs the same instance to read notifications back.
	notificationService := notifications.NewService(notificationStore)
	// Same shape as notificationService above — built first, passed in as
	// workItemService's TeamAuthority, and kept as its own variable because
	// the team handler below needs the same instance.
	teamService := teams.NewService(teamStore, membershipStore, supervisionStore)
	workItemService := workitems.NewService(workItemStore, statusHistoryStore, assignmentStore, assignmentHistoryStore, notificationService, teamService, txRunner)

	attachmentDiskStorage, err := attachments.NewLocalDiskStorage(cfg.AttachmentUploadDir)
	if err != nil {
		return nil, nil, err
	}
	attachmentService := attachments.NewService(attachmentMetaStore, attachmentDiskStorage)

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
			httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.Upload), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/attachments",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.List), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
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

	return mux, pool, nil
}
