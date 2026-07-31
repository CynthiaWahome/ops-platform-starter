package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                         string
	AppEnv                       string
	AuthTokenSecret              string
	AuthTokenTTL                 time.Duration
	BootstrapAdminIdentifier     string
	BootstrapAdminPassword       string
	BootstrapAdminDisplayName    string
	BootstrapAssigneeIdentifier  string
	BootstrapAssigneePassword    string
	BootstrapAssigneeDisplayName string
}

func Load() Config {
	return Config{
		Port:                        getEnv("PORT", "8080"),
		AppEnv:                      getEnv("APP_ENV", "development"),
		AuthTokenSecret:             getEnv("AUTH_TOKEN_SECRET", "change-me-super-secret"),
		AuthTokenTTL:                getEnvMinutes("AUTH_TOKEN_TTL_MINUTES", 60),
		BootstrapAdminIdentifier:    getEnv("BOOTSTRAP_ADMIN_IDENTIFIER", "admin@ops.local"),
		BootstrapAdminPassword:      getEnv("BOOTSTRAP_ADMIN_PASSWORD", "ChangeMe123!"),
		BootstrapAdminDisplayName:   getEnv("BOOTSTRAP_ADMIN_DISPLAY_NAME", "Platform Admin"),
		BootstrapAssigneeIdentifier: getEnv("BOOTSTRAP_ASSIGNEE_IDENTIFIER", "assignee@ops.local"),
		BootstrapAssigneePassword:   getEnv("BOOTSTRAP_ASSIGNEE_PASSWORD", "ChangeMe123!"),
		BootstrapAssigneeDisplayName: getEnv(
			"BOOTSTRAP_ASSIGNEE_DISPLAY_NAME",
			"Assigned Worker",
		),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}

func getEnvMinutes(key string, fallbackMinutes int) time.Duration {
	value := getEnv(key, "")
	if value == "" {
		return time.Duration(fallbackMinutes) * time.Minute
	}

	minutes, err := strconv.Atoi(value)
	if err != nil || minutes <= 0 {
		return time.Duration(fallbackMinutes) * time.Minute
	}

	return time.Duration(minutes) * time.Minute
}
