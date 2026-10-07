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

	// O app OAuth do GitHub, para o botão "Conectar com o GitHub". São opcionais: sem
	// eles a tela avisa que a conexão não está configurada.
	PublicURL          string
	GitHubClientID     string
	GitHubClientSecret string
	// GitHubURL e GitHubAPIURL só se mudam para apontar para um GitHub Enterprise ou
	// para um servidor fake. Vazios, valem github.com e api.github.com.
	GitHubURL    string
	GitHubAPIURL string
}

func Load() (*Config, error) {
	godotenv.Load()

	cfg := &Config{
		APIPort:               os.Getenv("API_PORT"),
		DatabaseURL:           os.Getenv("DATABASE_URL"),
		IntegrationEncryptKey: os.Getenv("INTEGRATION_ENCRYPTION_KEY"),
		CookieSecure:          os.Getenv("COOKIE_SECURE") == "true",
		PublicURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_URL")), "/"),
		GitHubClientID:        strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID")),
		GitHubClientSecret:    strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET")),
		GitHubURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_URL")), "/"),
		GitHubAPIURL:          strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_API_URL")), "/"),
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
