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

// A sincronização olha as integrações de cinco em cinco minutos, a menos que se diga outra coisa; zero a desliga, e o
// que não é uma duração derruba a subida. SYNC_INTERVAL vale para todas as plataformas; GITHUB_SYNC_INTERVAL e
// TRELLO_SYNC_INTERVAL valem só para a delas, e quem não tem o seu usa o comum.
func TestLoad_SyncIntervals(t *testing.T) {
	setRequired(t)
	clear := func() {
		for _, name := range []string{"SYNC_INTERVAL", "GITHUB_SYNC_INTERVAL", "TRELLO_SYNC_INTERVAL"} {
			t.Setenv(name, "")
		}
	}

	for _, name := range []string{"SYNC_INTERVAL", "GITHUB_SYNC_INTERVAL", "TRELLO_SYNC_INTERVAL"} {
		for raw, want := range map[string]time.Duration{"30s": 30 * time.Second, "1h": time.Hour, "0": 0, " 2m ": 2 * time.Minute} {
			clear()
			t.Setenv(name, raw)
			cfg, err := Load()
			if err != nil {
				t.Errorf("%s=%q: %v", name, raw, err)
				continue
			}
			if name == "SYNC_INTERVAL" {
				if cfg.SyncInterval != want || len(cfg.SyncIntervals) != 0 {
					t.Errorf("SYNC_INTERVAL=%q: default %v, per platform %v, want only the default %v", raw, cfg.SyncInterval, cfg.SyncIntervals, want)
				}
				continue
			}
			platform := map[string]string{"GITHUB_SYNC_INTERVAL": "github", "TRELLO_SYNC_INTERVAL": "trello"}[name]
			if got, ok := cfg.SyncIntervals[platform]; !ok || got != want || len(cfg.SyncIntervals) != 1 {
				t.Errorf("%s=%q: per platform %v, want only %s = %v", name, raw, cfg.SyncIntervals, platform, want)
			}
			if cfg.SyncInterval != 5*time.Minute {
				t.Errorf("%s=%q changed the default of the other platforms to %v", name, raw, cfg.SyncInterval)
			}
		}
		for _, raw := range []string{"five minutes", "5", "-1m"} {
			clear()
			t.Setenv(name, raw)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%q must be refused", name, raw)
			}
		}
	}

	// Sem nada definido, cinco minutos para todas.
	clear()
	if cfg, err := Load(); err != nil || cfg.SyncInterval != 5*time.Minute || len(cfg.SyncIntervals) != 0 {
		t.Errorf("defaults = %v %v, %v, want five minutes for everyone", cfg.SyncInterval, cfg.SyncIntervals, err)
	}
	// Cada uma com o seu, sobre o comum; "0" desliga só a daquela plataforma.
	clear()
	t.Setenv("SYNC_INTERVAL", "10m")
	t.Setenv("GITHUB_SYNC_INTERVAL", "1m")
	t.Setenv("TRELLO_SYNC_INTERVAL", "0")
	cfg, err := Load()
	if err != nil || cfg.SyncInterval != 10*time.Minute || cfg.SyncIntervals["github"] != time.Minute || cfg.SyncIntervals["trello"] != 0 || len(cfg.SyncIntervals) != 2 {
		t.Errorf("mixed = default %v, per platform %v, %v", cfg.SyncInterval, cfg.SyncIntervals, err)
	}
}

// A chave do app do Trello vai num cabeçalho: a que não é só letras e dígitos derruba a subida.
func TestLoad_TrelloSettings(t *testing.T) {
	setRequired(t)
	t.Setenv("SYNC_INTERVAL", "")
	t.Setenv("GITHUB_SYNC_INTERVAL", "")
	t.Setenv("TRELLO_SYNC_INTERVAL", "")
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

func clearClerk(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CLERK_SECRET_KEY", "CLERK_PUBLISHABLE_KEY", "CLERK_API_URL", "CLERK_AUTHORIZED_PARTIES", "PUBLIC_URL"} {
		t.Setenv(name, "")
	}
}

// Sem as chaves o Clerk fica desligado. Com as duas, o sistema precisa do PUBLIC_URL, e as origens aceitas são a
// dele, a menos que se diga outras.
func TestLoad_ClerkSettings(t *testing.T) {
	setRequired(t)
	clearClerk(t)

	cfg, err := Load()
	if err != nil || cfg.ClerkEnabled() || len(cfg.ClerkAuthorizedParties) != 0 {
		t.Fatalf("defaults = enabled %v, parties %v, %v; want off, none", cfg.ClerkEnabled(), cfg.ClerkAuthorizedParties, err)
	}

	t.Setenv("CLERK_SECRET_KEY", " sk_test_abc ")
	t.Setenv("CLERK_PUBLISHABLE_KEY", "pk_test_abc")
	t.Setenv("PUBLIC_URL", "http://localhost:8080/")
	cfg, err = Load()
	if err != nil || !cfg.ClerkEnabled() || cfg.ClerkSecretKey != "sk_test_abc" {
		t.Fatalf("both keys = enabled %v, secret %q, %v; want on, trimmed", cfg.ClerkEnabled(), cfg.ClerkSecretKey, err)
	}
	if got := cfg.ClerkAuthorizedParties; len(got) != 1 || got[0] != "http://localhost:8080" {
		t.Errorf("authorized parties = %v, want the origin of PUBLIC_URL", got)
	}

	t.Setenv("CLERK_AUTHORIZED_PARTIES", "http://localhost:8080/, http://localhost:8090 ,")
	cfg, err = Load()
	if err != nil || len(cfg.ClerkAuthorizedParties) != 2 || cfg.ClerkAuthorizedParties[1] != "http://localhost:8090" {
		t.Errorf("listed parties = %v, %v; want the two origins without slashes", cfg.ClerkAuthorizedParties, err)
	}
}

// Um engano na configuração do Clerk derruba a subida.
func TestLoad_ClerkMisconfigurationIsRefused(t *testing.T) {
	setRequired(t)
	for name, env := range map[string]map[string]string{
		"only the secret":      {"CLERK_SECRET_KEY": "sk_test_a"},
		"only the publishable": {"CLERK_PUBLISHABLE_KEY": "pk_test_a"},
		"no PUBLIC_URL":        {"CLERK_SECRET_KEY": "sk_test_a", "CLERK_PUBLISHABLE_KEY": "pk_test_a"},
		"swapped keys":         {"CLERK_SECRET_KEY": "pk_test_a", "CLERK_PUBLISHABLE_KEY": "sk_test_a", "PUBLIC_URL": "http://localhost:8080"},
		"relative PUBLIC_URL":  {"CLERK_SECRET_KEY": "sk_test_a", "CLERK_PUBLISHABLE_KEY": "pk_test_a", "PUBLIC_URL": "localhost:8080"},
	} {
		clearClerk(t)
		for k, v := range env {
			t.Setenv(k, v)
		}
		if _, err := Load(); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}
