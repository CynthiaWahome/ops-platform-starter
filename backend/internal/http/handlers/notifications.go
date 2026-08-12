package handlers

import (
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/http/middleware"
	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/notifications"
)

type NotificationHandler struct {
	service notifications.Service
}

func NewNotificationHandler(service notifications.Service) NotificationHandler {
	return NotificationHandler{service: service}
}

// List returns the caller's own notifications, oldest first. ?unread=true
// filters to unread only — the same query-string-flag shape a client
// would use to render an unread-count badge without fetching everything.
func (h NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	onlyUnread := r.URL.Query().Get("unread") == "true"

	list, err := h.service.List(r.Context(), principal.UserID, onlyUnread)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to fetch notifications"})
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// MarkAsRead sets ReadAt on one notification. A caller can only mark
// their own notifications read — attempting someone else's comes back as
// 404, the same "acts as if it doesn't exist" shape used everywhere else
// in this codebase for cross-user access (see workitems.Service.GetByID).
func (h NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	notification, err := h.service.MarkAsRead(r.Context(), r.PathValue("id"), principal.UserID)
	if err != nil {
		switch {
		case errors.Is(err, notifications.ErrInvalidInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: err.Error()})
		case errors.Is(err, notifications.ErrNotFound), errors.Is(err, notifications.ErrNotOwned):
			writeJSON(w, http.StatusNotFound, errorResponse{Message: "notification not found"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to mark notification as read"})
		}

		return
	}

	writeJSON(w, http.StatusOK, notification)
}
