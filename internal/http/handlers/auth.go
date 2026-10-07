package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
)

type AuthHandler struct {
	service auth.Service
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type errorResponse struct {
	Message string `json:"message"`
}

func NewAuthHandler(service auth.Service) AuthHandler {
	return AuthHandler{service: service}
}

func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	request, err := decodeLoginRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	session, err := h.service.Login(r.Context(), request.Identifier, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "invalid credentials"})
		case errors.Is(err, auth.ErrInactiveUser):
			writeJSON(w, http.StatusForbidden, errorResponse{Message: "user is inactive"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to complete login"})
		}

		return
	}

	writeJSON(w, http.StatusOK, session)
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChangePassword is the one write an account is always allowed to make,
// even while auth.User.RequiresPasswordChange has every other route
// 403ing (router.go exempts this route from that gate, not anything
// here) — it's the escape hatch for a brand-new admin-created account or
// one that just went through an admin-initiated reset (OPS-067).
func (h AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var input changePasswordRequest
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid request payload"})
		return
	}

	principal, ok := httpmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	if err := h.service.ChangePassword(r.Context(), principal.UserID, input.OldPassword, input.NewPassword); err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "old password is incorrect"})
		case errors.Is(err, auth.ErrInvalidUserInput):
			writeJSON(w, http.StatusBadRequest, errorResponse{Message: "new password must be at least 8 characters"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to change password"})
		}

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := httpmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Message: "authentication required"})
		return
	}

	writeJSON(w, http.StatusOK, principal)
}

func decodeLoginRequest(r *http.Request) (loginRequest, error) {
	defer r.Body.Close()

	var request loginRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		return loginRequest{}, err
	}

	if request.Identifier == "" || request.Password == "" {
		return loginRequest{}, errors.New("missing credentials")
	}

	return request, nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(payload)
}
