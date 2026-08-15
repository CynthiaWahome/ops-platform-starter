package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/CynthiaWahome/ops-platform-starter/backend/internal/config"
)

type HealthHandler struct {
	config config.Config
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	AppEnv  string `json:"appEnv"`
}

func NewHealthHandler(cfg config.Config) HealthHandler {
	return HealthHandler{config: cfg}
}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:  "ok",
		Service: "ops-platform-starter-backend",
		AppEnv:  h.config.AppEnv,
	})
}
