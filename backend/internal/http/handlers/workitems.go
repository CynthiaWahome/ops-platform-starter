package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/attachments"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/auth"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/workitems"
)

type WorkItemHandler struct {
	service           workitems.Service
	attachmentService attachments.Service
}

// NewWorkItemHandler takes an attachments.Service alongside the existing
// workitems.Service. workitems and attachments never import each other —
// this handler is the one place that knows both exist, the same role it
// already plays for AttachmentHandler's ownership gate. Here it enforces
// one small rule that connects the two: a work item cannot move to
// StatusSubmittedForReview with zero attachments recorded against it.
func NewWorkItemHandler(service workitems.Service, attachmentService attachments.Service) WorkItemHandler {
	return WorkItemHandler{service: service, attachmentService: attachmentService}
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
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	items, err := h.service.List(r.Context(), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), principal.HasRole(auth.RoleRequester))
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to list work items"})
		}

		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (h WorkItemHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.GetByID(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), principal.HasRole(auth.RoleRequester))
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

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.Update(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidPriority), errors.Is(err, workitems.ErrAlreadyAssigned):
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

	workItemID := r.PathValue("id")
	callerIsAdmin := principal.HasRole(auth.RoleAdmin)
	callerIsSupervisor := principal.HasRole(auth.RoleSupervisor)

	// Explicit evidence submission (OPS-031): moving into
	// StatusSubmittedForReview requires at least one attachment already
	// recorded. Confirms access to the work item first, the same
	// GetByID ownership gate AttachmentHandler already uses, before
	// asking attachmentService anything — a caller who cannot see this
	// work item never learns whether it has attachments.
	if input.ToStatus == workitems.StatusSubmittedForReview {
		if _, err := h.service.GetByID(r.Context(), workItemID, principal.UserID, callerIsAdmin, callerIsSupervisor, false); err != nil {
			writeWorkItemAccessError(w, err)
			return
		}

		existing, err := h.attachmentService.List(r.Context(), workItemID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to check attachments"})
			return
		}

		if len(existing) == 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: "at least one attachment is required before submitting for review"})
			return
		}
	}

	item, err := h.service.ChangeStatus(r.Context(), workItemID, principal.UserID, callerIsAdmin, callerIsSupervisor, input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput),
			errors.Is(err, workitems.ErrInvalidStatus),
			errors.Is(err, workitems.ErrInvalidTransition),
			errors.Is(err, workitems.ErrFeedbackRequired):
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

// Verify is a purpose-built front door onto ChangeStatus for the one
// transition OPS-032 cares about: SubmittedForReview -> Verified. The
// route this is wired to (POST /workitems/{id}/verify) is registered for
// admin and supervisor (OPS-045: a supervisor may verify their own team's
// work), so unlike the generic PATCH /workitems/{id}/status endpoint — where
// a wrong-role caller reaches the service layer and gets rejected there as
// an invalid transition (400) — an assignee hitting this route never gets
// past the middleware (403). Same underlying state machine, tighter, more
// honest error for the wrong-role case. A supervisor who clears the role
// check but targets another team's work item still gets rejected, at the
// service layer, by supervisorMayActOn.
func (h WorkItemHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var input workitems.VerifyInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.ChangeStatus(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), workitems.ChangeStatusInput{
		ToStatus: workitems.StatusVerified,
		Reason:   input.Note,
	})
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidTransition):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to verify work item"})
		}

		return
	}

	writeJSON(w, http.StatusOK, item)
}

// Flag is Verify's sibling for the rejection path: a purpose-built front
// door onto SubmittedForReview -> Flagged, open to admin and supervisor
// the same way Verify is. Unlike Verify, the feedback note is mandatory —
// flagging without saying why leaves the assignee stuck, so this handler
// rejects an empty note before it ever reaches ChangeStatus, the same
// "check the business rule before touching the service" layering OPS-031
// established for the evidence-required check.
func (h WorkItemHandler) Flag(w http.ResponseWriter, r *http.Request) {
	var input workitems.FlagInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	note := strings.TrimSpace(input.Note)
	if note == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "feedback note is required when flagging a work item"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	item, err := h.service.ChangeStatus(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), workitems.ChangeStatusInput{
		ToStatus: workitems.StatusFlagged,
		Reason:   &note,
	})
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidTransition):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to flag work item"})
		}

		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (h WorkItemHandler) ListStatusHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	history, err := h.service.ListStatusHistory(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor))
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

// ListAssignmentHistory is ListStatusHistory's OPS-040 counterpart for
// assignment events — same scoping, same error mapping, different
// service method and response shape.
func (h WorkItemHandler) ListAssignmentHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	history, err := h.service.ListAssignmentHistory(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor))
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch assignment history"})
		}

		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (h WorkItemHandler) Assign(w http.ResponseWriter, r *http.Request) {
	var input workitems.AssignInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	assignment, err := h.service.AssignWorkItem(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor), input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidTransition):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to assign work item"})
		}

		return
	}

	writeJSON(w, http.StatusCreated, assignment)
}

func (h WorkItemHandler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	assignment, err := h.service.GetAssignment(r.Context(), r.PathValue("id"), principal.UserID, principal.HasRole(auth.RoleAdmin), principal.HasRole(auth.RoleSupervisor))
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		case errors.Is(err, workitems.ErrAssignmentNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "assignment not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch assignment"})
		}

		return
	}

	writeJSON(w, http.StatusOK, assignment)
}

func (h WorkItemHandler) AcceptAssignment(w http.ResponseWriter, r *http.Request) {
	h.respondToAssignment(w, r, true)
}

func (h WorkItemHandler) DeclineAssignment(w http.ResponseWriter, r *http.Request) {
	h.respondToAssignment(w, r, false)
}

// respondToAssignment is shared by AcceptAssignment and DeclineAssignment so
// the request decoding, error mapping, and response writing live in one
// place instead of two near-identical copies.
func (h WorkItemHandler) respondToAssignment(w http.ResponseWriter, r *http.Request, accept bool) {
	var input workitems.RespondToAssignmentInput

	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	assignment, err := h.service.RespondToAssignment(r.Context(), r.PathValue("id"), principal.UserID, accept, input)
	if err != nil {
		switch {
		case errors.Is(err, workitems.ErrInvalidInput), errors.Is(err, workitems.ErrInvalidTransition):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrAssignmentNotOwned):
			writeJSON(w, http.StatusForbidden, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrAssignmentNotPending):
			writeJSON(w, http.StatusConflict, errorResponse{Message: err.Error()})
		case errors.Is(err, workitems.ErrNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "work item not found"})
		case errors.Is(err, workitems.ErrAssignmentNotFound):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "assignment not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to respond to assignment"})
		}

		return
	}

	writeJSON(w, http.StatusOK, assignment)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}
