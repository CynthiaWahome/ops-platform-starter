package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/internal/auth"
	httpmiddleware "github.com/CynthiaWahome/ops-platform-starter/internal/http/middleware"
)

type AuthHandler struct {
	service      auth.Service
	googleClient auth.GoogleOAuthClient
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type errorResponse struct {
	Message string `json:"message"`
}

// NewAuthHandler's googleClient may be nil — GoogleLogin and
// GoogleCallback are the only two methods that ever touch it, and
// router.go only wires those two routes up at all when Google OAuth is
// actually configured (OPS-068a). Every other AuthHandler method (Login,
// ChangePassword, Me) never references it.
func NewAuthHandler(service auth.Service, googleClient auth.GoogleOAuthClient) AuthHandler {
	return AuthHandler{service: service, googleClient: googleClient}
}

// googleOAuthStateCookie is the short-lived, httponly cookie GoogleLogin
// sets and GoogleCallback checks against the ?state= query param — CSRF
// protection for the redirect round trip. This codebase is otherwise
// entirely stateless bearer-token auth; a cookie exists only for this one
// flow, where the browser (not an API client holding a token) is the one
// making the redirect.
const googleOAuthStateCookie = "google_oauth_state"

// GoogleLogin redirects the browser to Google's consent screen (OPS-068a).
func (h AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateOAuthState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to start google login"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, h.googleClient.AuthCodeURL(state), http.StatusFound)
}

// GoogleCallback completes the round trip: verifies state, exchanges the
// code for the caller's Google identity, and logs them in — creating a
// requester-role account on first login for that identity, finding the
// existing one on every later login (OPS-068a).
func (h AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(googleOAuthStateCookie)
	if err != nil || r.URL.Query().Get("state") == "" || r.URL.Query().Get("state") != cookie.Value {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "invalid or missing oauth state"})
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Message: "missing authorization code"})
		return
	}

	identity, err := h.googleClient.Exchange(r.Context(), code)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Message: "unable to verify google identity"})
		return
	}

	session, err := h.service.LoginWithGoogle(r.Context(), identity)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrGoogleEmailAlreadyRegistered):
			writeJSON(w, http.StatusConflict, errorResponse{Message: err.Error()})
		case errors.Is(err, auth.ErrInactiveUser):
			writeJSON(w, http.StatusForbidden, errorResponse{Message: "user is inactive"})
		default:
			writeJSON(w, http.StatusInternalServerError, errorResponse{Message: "unable to complete google login"})
		}

		return
	}

	writeJSON(w, http.StatusOK, session)
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
