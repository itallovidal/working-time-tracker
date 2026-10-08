package config

import (
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("API_PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("INTEGRATION_ENCRYPTION_KEY", "k")
}

// A sincronização olha as integrações de cinco em cinco minutos, a menos que se diga outra coisa; zero a
// desliga, e o que não é uma duração derruba a subida. SYNC_INTERVAL é o nome de agora; GITHUB_SYNC_INTERVAL,
// de quando só o GitHub sincronizava, ainda vale se o outro não está definido.
func TestLoad_SyncInterval(t *testing.T) {
	setRequired(t)
	for _, name := range []string{"SYNC_INTERVAL", "GITHUB_SYNC_INTERVAL"} {
		for raw, want := range map[string]time.Duration{"": 5 * time.Minute, "30s": 30 * time.Second, "1h": time.Hour, "0": 0, " 2m ": 2 * time.Minute} {
			t.Setenv("SYNC_INTERVAL", "")
			t.Setenv("GITHUB_SYNC_INTERVAL", "")
			t.Setenv(name, raw)
			cfg, err := Load()
			if err != nil || cfg.SyncInterval != want {
				t.Errorf("%s=%q: interval = %v, err = %v, want %v", name, raw, cfg.SyncInterval, err, want)
			}
		}
		for _, raw := range []string{"five minutes", "5", "-1m"} {
			t.Setenv("SYNC_INTERVAL", "")
			t.Setenv("GITHUB_SYNC_INTERVAL", "")
			t.Setenv(name, raw)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%q must be refused", name, raw)
			}
		}
	}

	// O nome novo vale sobre o antigo.
	t.Setenv("SYNC_INTERVAL", "10s")
	t.Setenv("GITHUB_SYNC_INTERVAL", "1h")
	if cfg, err := Load(); err != nil || cfg.SyncInterval != 10*time.Second {
		t.Errorf("SYNC_INTERVAL must win over GITHUB_SYNC_INTERVAL: %v, %v", cfg.SyncInterval, err)
	}
}

// A chave do app do Trello vai num cabeçalho: a que não é só letras e dígitos derruba a subida.
func TestLoad_TrelloSettings(t *testing.T) {
	setRequired(t)
	t.Setenv("SYNC_INTERVAL", "")
	t.Setenv("GITHUB_SYNC_INTERVAL", "")
	t.Setenv("TRELLO_API_KEY", " 0123456789abcdef ")
	t.Setenv("TRELLO_URL", "http://localhost:8092/")
	t.Setenv("TRELLO_API_URL", "http://localhost:8092/1/")
	t.Setenv("TRELLO_APP_NAME", " Meu app ")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TrelloAPIKey != "0123456789abcdef" || cfg.TrelloURL != "http://localhost:8092" || cfg.TrelloAPIURL != "http://localhost:8092/1" || cfg.TrelloAppName != "Meu app" {
		t.Errorf("trello settings = %q %q %q %q", cfg.TrelloAPIKey, cfg.TrelloURL, cfg.TrelloAPIURL, cfg.TrelloAppName)
	}
	t.Setenv("TRELLO_API_KEY", `abc", x="y`)
	if _, err := Load(); err == nil {
		t.Error("a key that is not letters and digits must be refused")
	}
	t.Setenv("TRELLO_API_KEY", "")
	if cfg, err := Load(); err != nil || cfg.TrelloAPIKey != "" {
		t.Errorf("no key is fine (the connection just says it is not configured): %+v, %v", cfg, err)
	}
}
