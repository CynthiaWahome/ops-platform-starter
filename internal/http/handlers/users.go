package handlers

import (
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/internal/teams"
)

// UsersHandler provisions and manages internal accounts (OPS-067). It
// composes auth.Service (account CRUD, passwords) with teams.Service
// (optional team provisioning at creation time, and supervisor-scoped
// visibility) the same way WorkItemHandler already composes
// workitems.Service with attachments.Service — two existing services, no
// new cross-package dependency added to either one.
type UsersHandler struct {
	authService  auth.Service
	teamsService teams.Service
}

func NewUsersHandler(authService auth.Service, teamsService teams.Service) UsersHandler {
	return UsersHandler{authService: authService, teamsService: teamsService}
}

type createUserInput struct {
	Role        string  `json:"role"`
	Identifier  string  `json:"identifier"`
	DisplayName string  `json:"displayName"`
	TeamID      *string `json:"teamId"`
}

type createUserResponse struct {
	User         auth.User `json:"user"`
	TempPassword string    `json:"tempPassword"`
}

// Create provisions a new account with a server-generated temp password,
// returned once in the response body (never emailed — no email infra for
// internal users by design, see OPS-067). TeamID is required for
// supervisor/assignee and, when present, immediately provisions the team
// membership/supervision too — one call instead of a follow-up
// POST /teams/{id}/assignees or /supervisors.
func (h UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input createUserInput
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	role := auth.Role(input.Role)

	if (role == auth.RoleSupervisor || role == auth.RoleAssignee) && (input.TeamID == nil || *input.TeamID == "") {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "teamId is required for supervisor and assignee roles"})
		return
	}

	user, tempPassword, err := h.authService.CreateUser(r.Context(), principal.UserID, auth.CreateUserInput{
		Role:        role,
		Identifier:  input.Identifier,
		DisplayName: input.DisplayName,
	})
	if err != nil {
		writeUserError(w, err)
		return
	}

	if input.TeamID != nil && *input.TeamID != "" {
		switch role {
		case auth.RoleAssignee:
			if _, err := h.teamsService.AddAssignee(r.Context(), *input.TeamID, user.ID, principal.UserID); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Message: "user created but team assignment failed: " + err.Error()})
				return
			}
		case auth.RoleSupervisor:
			if _, err := h.teamsService.AddSupervisor(r.Context(), *input.TeamID, user.ID, principal.UserID); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Message: "user created but supervisor assignment failed: " + err.Error()})
				return
			}
		}
	}

	writeJSON(w, http.StatusCreated, createUserResponse{User: user, TempPassword: tempPassword})
}

// List returns every account for admin, unrestricted. A supervisor instead
// gets only the assignees on the team(s) they supervise, plus themselves —
// a deliberate first-cut scoping: fellow co-supervisors of the same team
// are left out here, since the ticket asks for "own team" scoping of
// assignees, not a full team-roster view. Extending to co-supervisors is a
// small, separate follow-up if that's wanted later.
func (h UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	all, err := h.authService.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list users"})
		return
	}

	if principal.HasRole(auth.RoleAdmin) {
		writeJSON(w, http.StatusOK, all)
		return
	}

	supervised, err := h.teamsService.SupervisedAssigneeUserIDs(r.Context(), principal.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list users"})
		return
	}

	allowed := make(map[string]bool, len(supervised)+1)
	allowed[principal.UserID] = true
	for _, id := range supervised {
		allowed[id] = true
	}

	scoped := make([]auth.User, 0, len(allowed))
	for _, user := range all {
		if allowed[user.ID] {
			scoped = append(scoped, user)
		}
	}

	writeJSON(w, http.StatusOK, scoped)
}

type updateUserInput struct {
	IsActive *bool   `json:"isActive"`
	Role     *string `json:"role"`
}

// Update handles deactivate/reactivate and role change. A supervisor may
// only target a user currently in their SupervisedAssigneeUserIDs; team
// reassignment goes through /teams/{id}/assignees and /supervisors, not
// here, to avoid duplicating AddAssignee's move-semantics and
// AddSupervisor/RemoveSupervisor's co-supervision semantics.
func (h UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	var input updateUserInput
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	targetID := r.PathValue("id")

	if !principal.HasRole(auth.RoleAdmin) {
		allowed, err := h.supervisorMayAct(r, principal.UserID, targetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to update user"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, errorResponse{Message: "insufficient permissions"})
			return
		}
	}

	var rolePtr *auth.Role
	if input.Role != nil {
		role := auth.Role(*input.Role)
		rolePtr = &role
	}

	user, err := h.authService.UpdateUser(r.Context(), targetID, auth.UpdateUserInput{
		IsActive: input.IsActive,
		Role:     rolePtr,
	})
	if err != nil {
		writeUserError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

type resetPasswordResponse struct {
	TempPassword string `json:"tempPassword"`
}

// ResetPassword re-triggers the temp-password flow (OPS-067) — this is the
// internal-user password-reset story: admin-initiated, not self-service,
// since there's no email infra to send a reset link to.
func (h UsersHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	targetID := r.PathValue("id")

	if !principal.HasRole(auth.RoleAdmin) {
		allowed, err := h.supervisorMayAct(r, principal.UserID, targetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to reset password"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, errorResponse{Message: "insufficient permissions"})
			return
		}
	}

	tempPassword, err := h.authService.ResetPassword(r.Context(), targetID)
	if err != nil {
		writeUserError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resetPasswordResponse{TempPassword: tempPassword})
}

func (h UsersHandler) supervisorMayAct(r *http.Request, supervisorUserID, targetUserID string) (bool, error) {
	supervised, err := h.teamsService.SupervisedAssigneeUserIDs(r.Context(), supervisorUserID)
	if err != nil {
		return false, err
	}

	for _, id := range supervised {
		if id == targetUserID {
			return true, nil
		}
	}

	return false, nil
}

func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidUserInput), errors.Is(err, auth.ErrInvalidRole):
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
	case errors.Is(err, auth.ErrIdentifierTaken):
		writeJSON(w, http.StatusConflict, errorResponse{Message: err.Error()})
	case errors.Is(err, auth.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Message: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to complete user operation"})
	}
}
