package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"

	"github.com/labstack/echo/v5"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/config"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
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

	// A chave do app do Trello também só vale com o endereço público: é para ele que o Trello devolve a pessoa.
	trello := &adapter.TrelloAuth{APIKey: ENV.TrelloAPIKey, AppName: ENV.TrelloAppName, SiteURL: ENV.TrelloURL}
	if ENV.PublicURL != "" {
		trello.RedirectURL = ENV.PublicURL + integration.TrelloCallbackPath
	}
	if ENV.TrelloAPIURL != "" {
		adapter.Register("trello", func() adapter.Integration {
			return &adapter.TrelloIntegration{BaseURL: ENV.TrelloAPIURL}
		})
	}

	app, err := server.Build(db.Client, server.Options{
		EncryptKey:   ENV.IntegrationEncryptKey,
		CookieSecure: ENV.CookieSecure,
		GitHubOAuth:  github,
		TrelloAuth:   trello,
		Sync:         issuesync.Config{Interval: ENV.SyncInterval},
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	e := app.Echo
	apiURL, siteURL := ENV.GitHubAPIURL, ENV.GitHubURL
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	if siteURL == "" {
		siteURL = "https://github.com"
	}
	e.Logger.Info("github", "api", apiURL, "site", siteURL, "sync_interval", ENV.SyncInterval.String())
	trelloAPI, trelloSite := ENV.TrelloAPIURL, ENV.TrelloURL
	if trelloAPI == "" {
		trelloAPI = "https://api.trello.com/1"
	}
	if trelloSite == "" {
		trelloSite = "https://trello.com"
	}
	e.Logger.Info("trello", "api", trelloAPI, "site", trelloSite, "connection_configured", trello.Configured())

	// Ctrl+C e SIGTERM acabam o servidor e a sincronização, que termina a rodada em andamento antes de
	// o banco fechar.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		app.Sync.Run(ctx)
	}()

	if err := (echo.StartConfig{Address: ":" + ENV.APIPort}).Start(ctx, e); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
	stop()
	background.Wait()
}
