package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	APIPort               string
	DatabaseURL           string
	IntegrationEncryptKey string
	CookieSecure          bool
}

func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		APIPort:               os.Getenv("API_PORT"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		IntegrationEncryptKey: os.Getenv("INTEGRATION_ENCRYPTION_KEY"),
		CookieSecure:          os.Getenv("COOKIE_SECURE") == "true",
	}

	var missing []string
	if cfg.APIPort == "" {
		missing = append(missing, "API_PORT")
	}
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.IntegrationEncryptKey == "" {
		missing = append(missing, "INTEGRATION_ENCRYPTION_KEY")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}
