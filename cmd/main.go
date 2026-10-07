package main

import (
	"context"
	"log"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/config"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/server"
)

func main() {
	ENV, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Open(ENV.DatabaseURL)

	if err != nil {
		log.Fatalf("database: %v", err)
	}

	if err := database.Migrate(context.Background(), db.Raw); err != nil {
		log.Fatalf("migration: %v", err)
	}

	// O app OAuth do GitHub só vale com o endereço público do servidor: o GitHub devolve a
	// pessoa para PUBLIC_URL mais o caminho do callback, e esse é o endereço cadastrado nele.
	github := &adapter.GitHubOAuth{
		ClientID:     ENV.GitHubClientID,
		ClientSecret: ENV.GitHubClientSecret,
		SiteURL:      ENV.GitHubURL,
	}
	if ENV.PublicURL != "" {
		github.RedirectURL = ENV.PublicURL + integration.CallbackPath
	}
	if ENV.GitHubAPIURL != "" {
		adapter.Register("github", func() adapter.Integration {
			return &adapter.GitHubIntegration{BaseURL: ENV.GitHubAPIURL}
		})
	}

	e, err := server.New(db.Client, server.Options{
		EncryptKey:   ENV.IntegrationEncryptKey,
		CookieSecure: ENV.CookieSecure,
		GitHubOAuth:  github,
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	if err := e.Start(":" + ENV.APIPort); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
