package handlers

import "net/http"

type accessResponse struct {
	Message string `json:"message"`
}

type AccessHandler struct{}

func NewAccessHandler() AccessHandler {
	return AccessHandler{}
}

func (h AccessHandler) AdminOnly(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, accessResponse{Message: "admin access granted"})
}

func (h AccessHandler) AssigneeOnly(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, accessResponse{Message: "assignee access granted"})
}
