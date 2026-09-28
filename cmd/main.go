package main

import (
	"log"

	"working-time-tracker/internal/config"
	"working-time-tracker/internal/database"
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

	if err := database.AutoMigrate(db.Client); err != nil {
		log.Fatalf("migration: %v", err)
	}

	e := server.New(db.Client, ENV.IntegrationEncryptKey)

	if err := e.Start(":" + ENV.APIPort); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}
