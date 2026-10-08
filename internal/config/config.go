package config

import (
	"fmt"
	"os"
	"strings"
	"time"

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

	// A chave do app do Trello, para o botão "Conectar com o Trello" (a tela de autorização do Trello a
	// pede; ela é pública, o segredo é o token de cada pessoa). Opcional: sem ela, ou sem PUBLIC_URL, a tela
	// avisa que a conexão não está configurada. TrelloURL e TrelloAPIURL só se mudam para apontar para um
	// servidor fake; vazios, valem trello.com e api.trello.com/1.
	TrelloAPIKey  string
	TrelloAppName string
	TrelloURL     string
	TrelloAPIURL  string

	// SyncInterval é de quanto em quanto tempo o servidor olha as integrações com a sincronização ligada
	// (as issues do GitHub, os cartões do Trello). Zero desliga a rotina (o botão e o gancho das tarefas
	// seguem valendo).
	SyncInterval time.Duration
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
		TrelloAPIKey:          strings.TrimSpace(os.Getenv("TRELLO_API_KEY")),
		TrelloAppName:         strings.TrimSpace(os.Getenv("TRELLO_APP_NAME")),
		TrelloURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("TRELLO_URL")), "/"),
		TrelloAPIURL:          strings.TrimRight(strings.TrimSpace(os.Getenv("TRELLO_API_URL")), "/"),
	}
	// A chave vai no cabeçalho das requisições ao Trello: uma que não é só letras e dígitos é um erro de
	// quem configurou, e melhor aparecer na subida.
	if key := cfg.TrelloAPIKey; key != "" && strings.Trim(key, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return nil, fmt.Errorf("TRELLO_API_KEY must be the key of the app (letters and digits only)")
	}

	// O padrão é cinco minutos; "0" desliga. Um valor que não é uma duração (ex.: "5m", "30s") derruba a
	// subida: melhor que rodar com um intervalo que ninguém pediu. SYNC_INTERVAL é o nome de agora;
	// GITHUB_SYNC_INTERVAL, de quando só o GitHub sincronizava, ainda vale se ele não está definido.
	cfg.SyncInterval = 5 * time.Minute
	name, raw := "SYNC_INTERVAL", strings.TrimSpace(os.Getenv("SYNC_INTERVAL"))
	if raw == "" {
		name, raw = "GITHUB_SYNC_INTERVAL", strings.TrimSpace(os.Getenv("GITHUB_SYNC_INTERVAL"))
	}
	if raw != "" {
		d, err := time.ParseDuration(raw)
		if raw == "0" {
			d, err = 0, nil
		}
		if err != nil || d < 0 {
			return nil, fmt.Errorf("%s %q is not a duration like 5m, 30s or 0", name, raw)
		}
		cfg.SyncInterval = d
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
