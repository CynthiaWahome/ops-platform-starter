package config

import "os"

type Config struct {
	Port   string
	AppEnv string
}

func Load() Config {
	return Config{
		Port:   getEnv("PORT", "8080"),
		AppEnv: getEnv("APP_ENV", "development"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}
