package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

type WorkItemHandler struct {
	service workitems.Service
}

func NewWorkItemHandler(service workitems.Service) WorkItemHandler {
	return WorkItemHandler{service: service}
}

func (h WorkItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input workitems.CreateInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.Create(r.Context(), principal.UserID, input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidPriority):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to create work item"})
		}

		return
	}

	writeJSON(w, http.StatusCreated, item)
}

func (h WorkItemHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list work items"})
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h WorkItemHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch work item"})
		}

		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h WorkItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	var input workitems.UpdateInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	item, err := h.service.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidPriority):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to update work item"})
		}

		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h WorkItemHandler) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	var input workitems.ChangeStatusInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.ChangeStatus(r.Context(), r.PathValue("id"), principal.UserID, input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput),
			errors.Is(err, workitems.ErrInvalidStatus),
			errors.Is(err, workitems.ErrInvalidTransition):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to change work item status"})
		}

		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h WorkItemHandler) ListStatusHistory(w http.ResponseWriter, r *http.Request) {
	history, err := h.service.ListStatusHistory(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch status history"})
		}

		return
	}

	writeJSON(w, http.StatusOK, history)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}
