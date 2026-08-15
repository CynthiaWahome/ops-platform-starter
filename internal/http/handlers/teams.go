package handlers

import (
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/internal/teams"
)

// TeamHandler exposes team structure management — create a team, add or
// remove a supervisor, move an assignee onto a team. Every route this is
// wired to is admin-only (see router.go): who has authority over a team is
// an org-structure decision, not something a supervisor should be able to
// grant a peer unilaterally (OPS-045).
type TeamHandler struct {
	service teams.Service
}

func NewTeamHandler(service teams.Service) TeamHandler {
	return TeamHandler{service: service}
}

type createTeamInput struct {
	Name string `json:"name"`
}

func (h TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input createTeamInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	team, err := h.service.CreateTeam(r.Context(), input.Name)
	if err != nil {
		switch {
		case errors.Is(err, teams.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to create team"})
		}

		return
	}

	writeJSON(w, http.StatusCreated, team)
}

func (h TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListTeams(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list teams"})
		return
	}

	writeJSON(w, http.StatusOK, list)
}

type teamMemberInput struct {
	UserID string `json:"userId"`
}

func (h TeamHandler) AddAssignee(w http.ResponseWriter, r *http.Request) {
	var input teamMemberInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	membership, err := h.service.AddAssignee(r.Context(), r.PathValue("id"), input.UserID, principal.UserID)
	if err != nil {
		writeTeamError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, membership)
}

func (h TeamHandler) AddSupervisor(w http.ResponseWriter, r *http.Request) {
	var input teamMemberInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	supervision, err := h.service.AddSupervisor(r.Context(), r.PathValue("id"), input.UserID, principal.UserID)
	if err != nil {
		writeTeamError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, supervision)
}

func (h TeamHandler) RemoveSupervisor(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RemoveSupervisor(r.Context(), r.PathValue("id"), r.PathValue("userId")); err != nil {
		writeTeamError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeTeamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, teams.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
	case errors.Is(err, teams.ErrNotFound), errors.Is(err, teams.ErrNotSupervisor):
		writeJSON(w, http.StatusNotFound, errorResponse{Message: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to complete team operation"})
	}
}
