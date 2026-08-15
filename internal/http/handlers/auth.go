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
