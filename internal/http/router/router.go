package router

import (
	"context"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/internal/config"
	"github.com/CynthiaWahome/ops-platform-starter/internal/db"
	"github.com/CynthiaWahome/ops-platform-starter/internal/http/handlers"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/internal/notifications"
	"github.com/CynthiaWahome/ops-platform-starter/internal/teams"
	"github.com/CynthiaWahome/ops-platform-starter/internal/workitems"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New builds the full HTTP handler. It also returns the Postgres pool it
// opened, if any (OPS-048) — nil when cfg.DatabaseURL is empty — so the
// caller (server.New) can close it on shutdown. The router itself never
// holds a reference to the pool once construction is done; only the
// Postgres*Store values built from it do.
func New(ctx context.Context, cfg config.Config) (http.Handler, *pgxpool.Pool, error) {
	mux := http.NewServeMux()

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
		// userStore joins this same in-memory/Postgres split for the first
		// time in OPS-067 — before this, auth was the one domain still
		// rebuilt from env vars on every process start, even when
		// DATABASE_URL was set for everything else.
		userStore auth.UserStore
		pool      *pgxpool.Pool
		txRunner  workitems.TxRunner
		err       error
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
		userStore = auth.NewMemoryUserStore()
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
		userStore = auth.NewPostgresUserStore(pool)
		// txRunner stays nil in the in-memory branch above — see
		// workitems.TxRunner's doc comment for why that's correct, not
		// just unimplemented.
		txRunner = db.PoolTxRunner{Pool: pool}
	}

	passwords := auth.NewBcryptPasswordManager(0)
	tokens := auth.NewJWTManager(cfg.AuthTokenSecret, "ops-platform-starter-backend", cfg.AuthTokenTTL)

	// Seeds the 4 bootstrap accounts into whichever userStore is active.
	// Idempotent (UserStore.Seed never overwrites an existing row) — an
	// admin's later edit to one of these (deactivate, re-role) survives a
	// restart instead of being silently reset back to the .env defaults.
	if err := auth.SeedBootstrapUsers(ctx, userStore, passwords, auth.BootstrapSeedsFromConfig(cfg)); err != nil {
		if pool != nil {
			pool.Close()
		}
		return nil, nil, err
	}

	// emailSender stays nil unless an email provider is actually
	// configured (OPS-068b) — Resend takes priority if both happen to be
	// set, since RESEND_FROM defaults to a usable value
	// (onboarding@resend.dev) while SMTP requires real credentials
	// someone had to deliberately set. A startup-time choice, not a
	// runtime failover — see SMTPEmailSender's doc comment for why.
	var emailSender auth.EmailSender
	emailEnabled := cfg.ResendAPIKey != "" || cfg.SMTPHost != ""
	switch {
	case cfg.ResendAPIKey != "":
		emailSender = auth.NewResendEmailSender(cfg.ResendAPIKey, cfg.ResendFrom)
	case cfg.SMTPHost != "":
		emailSender = auth.NewSMTPEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom)
	}

	authService := auth.NewService(userStore, passwords, tokens, emailSender)

	// googleClient stays nil unless Google OAuth is actually configured
	// (OPS-068a) — the same "absent config means the feature isn't wired
	// up" pattern cfg.DatabaseURL already uses. Checked here, once, rather
	// than at every call site.
	var googleClient auth.GoogleOAuthClient
	googleOAuthEnabled := cfg.GoogleOAuthClientID != ""
	if googleOAuthEnabled {
		googleClient = auth.NewOAuth2GoogleClient(cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleOAuthRedirectURL)
	}

	// Built before workItemService and passed in as its NotificationSink —
	// notifications.Service satisfies that interface by having a matching
	// Notify method, nothing more is needed to wire it in. Kept as its
	// own variable (not just an inline argument) because the notification
	// handler below needs the same instance to read notifications back.
	notificationService := notifications.NewService(notificationStore)
	// Same shape as notificationService above — built first, passed in as
	// workItemService's TeamAuthority, and kept as its own variable because
	// the team handler and the users handler (OPS-067) both need the same
	// instance.
	teamService := teams.NewService(teamStore, membershipStore, supervisionStore)
	workItemService := workitems.NewService(workItemStore, statusHistoryStore, assignmentStore, assignmentHistoryStore, notificationService, teamService, txRunner)

	attachmentDiskStorage, err := attachments.NewLocalDiskStorage(cfg.AttachmentUploadDir)
	if err != nil {
		return nil, nil, err
	}
	attachmentService := attachments.NewService(attachmentMetaStore, attachmentDiskStorage)

	healthHandler := handlers.NewHealthHandler(cfg)
	authHandler := handlers.NewAuthHandler(authService, googleClient)
	usersHandler := handlers.NewUsersHandler(authService, teamService)
	accessHandler := handlers.NewAccessHandler()
	workItemHandler := handlers.NewWorkItemHandler(workItemService, attachmentService)
	attachmentHandler := handlers.NewAttachmentHandler(workItemService, attachmentService)
	teamHandler := handlers.NewTeamHandler(teamService)
	notificationHandler := handlers.NewNotificationHandler(notificationService)

	mux.Handle("GET /health", healthHandler)

	// auth/login never requires a token to begin with; auth/me and
	// auth/change-password are the two routes OPS-067 deliberately leaves
	// out of the RequirePasswordChangeCleared gate below — they're what
	// let an account see itself and clear the flag in the first place.
	mux.HandleFunc("POST /auth/login", authHandler.Login)
	mux.Handle("GET /auth/me", httpmiddleware.RequireAuth(authService, http.HandlerFunc(authHandler.Me)))
	mux.Handle("POST /auth/change-password", httpmiddleware.RequireAuth(authService, http.HandlerFunc(authHandler.ChangePassword)))

	// Google OAuth signup/login (OPS-068a) — requester-only, additive
	// alongside the password login above, which stays untouched for the
	// other 3 roles. Only mounted when actually configured, same "absent
	// config, route doesn't exist" pattern as everything else gated on an
	// optional env var in this router.
	if googleOAuthEnabled {
		mux.HandleFunc("GET /auth/google/login", authHandler.GoogleLogin)
		mux.HandleFunc("GET /auth/google/callback", authHandler.GoogleCallback)
	}

	// Email/password requester signup (OPS-068b) — additive alongside
	// both the password login above and Google OAuth above, which stay
	// untouched. Only mounted when an email provider is actually
	// configured, same pattern as Google OAuth. verify-email requires a
	// session (SignUp already returns one); forgot/reset-password are
	// unauthenticated by design — a password reset is exactly the flow
	// for someone who can't necessarily log in right now.
	if emailEnabled {
		mux.HandleFunc("POST /auth/signup", authHandler.SignUp)
		mux.Handle("POST /auth/verify-email", httpmiddleware.RequireAuth(authService, http.HandlerFunc(authHandler.VerifyEmail)))
		mux.HandleFunc("POST /auth/forgot-password", authHandler.ForgotPassword)
		mux.HandleFunc("POST /auth/reset-password", authHandler.ResetPassword)
	}

	mux.Handle(
		"POST /users",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(usersHandler.Create), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"GET /users",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(usersHandler.List), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"PATCH /users/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(usersHandler.Update), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"POST /users/{id}/reset-password",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(usersHandler.ResetPassword), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)

	mux.Handle(
		"GET /access/admin",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(accessHandler.AdminOnly), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"GET /access/assignee",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(accessHandler.AssigneeOnly), auth.RoleAssignee),
			),
		),
	)
	mux.Handle(
		"POST /workitems",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireVerifiedEmailForRequester(
					httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Create), auth.RoleAdmin, auth.RoleSupervisor, auth.RoleRequester),
				),
			),
		),
	)
	mux.Handle(
		"GET /workitems",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.List), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor, auth.RoleRequester),
			),
		),
	)
	mux.Handle(
		"GET /workitems/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetByID), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor, auth.RoleRequester),
			),
		),
	)
	mux.Handle(
		"PATCH /workitems/{id}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Update), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"PATCH /workitems/{id}/status",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ChangeStatus), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/verify",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Verify), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/flag",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Flag), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/history",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ListStatusHistory), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/assignment-history",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.ListAssignmentHistory), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.Assign), auth.RoleAdmin, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/assignment",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.GetAssignment), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment/accept",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.AcceptAssignment), auth.RoleAssignee),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/assignment/decline",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(workItemHandler.DeclineAssignment), auth.RoleAssignee),
			),
		),
	)
	mux.Handle(
		"POST /teams",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.Create), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"GET /teams",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.List), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"POST /teams/{id}/assignees",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.AddAssignee), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"POST /teams/{id}/supervisors",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.AddSupervisor), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"DELETE /teams/{id}/supervisors/{userId}",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(teamHandler.RemoveSupervisor), auth.RoleAdmin),
			),
		),
	)
	mux.Handle(
		"POST /workitems/{id}/attachments",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.Upload), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"GET /workitems/{id}/attachments",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(attachmentHandler.List), auth.RoleAdmin, auth.RoleAssignee, auth.RoleSupervisor),
			),
		),
	)
	mux.Handle(
		"GET /notifications",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(notificationHandler.List), auth.RoleAdmin, auth.RoleAssignee),
			),
		),
	)
	mux.Handle(
		"POST /notifications/{id}/read",
		httpmiddleware.RequireAuth(
			authService,
			httpmiddleware.RequirePasswordChangeCleared(
				httpmiddleware.RequireRoles(http.HandlerFunc(notificationHandler.MarkAsRead), auth.RoleAdmin, auth.RoleAssignee),
			),
		),
	)

	// Wraps the whole mux, not individual routes (issue #72) — every
	// request gets one log line, including a 404 that hit no route at
	// all, which is exactly the visibility that was missing when a
	// failed #68b signup produced nothing in the server's own output.
	return httpmiddleware.RequestLogger(mux), pool, nil
}
