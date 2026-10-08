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
	// seguem valendo). SyncIntervals tem, por plataforma ("github", "trello"), o intervalo próprio de quem o
	// definiu (GITHUB_SYNC_INTERVAL, TRELLO_SYNC_INTERVAL); a que não está aí usa SyncInterval.
	SyncInterval  time.Duration
	SyncIntervals map[string]time.Duration
}

// syncIntervalEnv são as variáveis do intervalo de cada plataforma.
var syncIntervalEnv = map[string]string{"github": "GITHUB_SYNC_INTERVAL", "trello": "TRELLO_SYNC_INTERVAL"}

// parseInterval lê uma duração do ambiente ("5m", "30s"; "0" desliga). Vazio é não definido. O que não é uma
// duração derruba a subida: melhor que rodar com um intervalo que ninguém pediu.
func parseInterval(name string) (d time.Duration, set bool, err error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	if raw == "0" {
		return 0, true, nil
	}
	d, err = time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0, false, fmt.Errorf("%s %q is not a duration like 5m, 30s or 0", name, raw)
	}
	return d, true, nil
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

	// O padrão é cinco minutos; "0" desliga. SYNC_INTERVAL vale para todas as plataformas, e
	// GITHUB_SYNC_INTERVAL e TRELLO_SYNC_INTERVAL valem só para a delas, sobre o comum.
	cfg.SyncInterval = 5 * time.Minute
	if d, set, err := parseInterval("SYNC_INTERVAL"); err != nil {
		return nil, err
	} else if set {
		cfg.SyncInterval = d
	}
	for platform, name := range syncIntervalEnv {
		d, set, err := parseInterval(name)
		if err != nil {
			return nil, err
		}
		if set {
			if cfg.SyncIntervals == nil {
				cfg.SyncIntervals = map[string]time.Duration{}
			}
			cfg.SyncIntervals[platform] = d
		}
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
