package database

import (
	"context"
	"fmt"
	"log"

	"working-time-tracker/ent"
	"working-time-tracker/ent/migrate"
)

func AutoMigrate(client *ent.Client) error {
	if err := client.Schema.Create(context.Background(),
		migrate.WithDropColumn(true),
		migrate.WithDropIndex(true),
	); err != nil {
		return fmt.Errorf("ent schema migration: %w", err)
	}

	log.Println("database migration completed")
	return nil
}
