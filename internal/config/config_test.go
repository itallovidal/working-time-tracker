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

// A sincronização das issues olha o GitHub de cinco em cinco minutos, a menos que se diga outra coisa; zero
// a desliga, e o que não é uma duração derruba a subida.
func TestLoad_GitHubSyncInterval(t *testing.T) {
	setRequired(t)
	for raw, want := range map[string]time.Duration{"": 5 * time.Minute, "30s": 30 * time.Second, "1h": time.Hour, "0": 0, " 2m ": 2 * time.Minute} {
		t.Setenv("GITHUB_SYNC_INTERVAL", raw)
		cfg, err := Load()
		if err != nil || cfg.GitHubSyncInterval != want {
			t.Errorf("GITHUB_SYNC_INTERVAL=%q: interval = %v, err = %v, want %v", raw, cfg.GitHubSyncInterval, err, want)
		}
	}
	for _, raw := range []string{"five minutes", "5", "-1m"} {
		t.Setenv("GITHUB_SYNC_INTERVAL", raw)
		if _, err := Load(); err == nil {
			t.Errorf("GITHUB_SYNC_INTERVAL=%q must be refused", raw)
		}
	}
}
