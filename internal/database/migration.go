package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"working-time-tracker/ent"
	"working-time-tracker/ent/migrate"
)

func AutoMigrate(client *ent.Client, raw *sql.DB) error {
	if err := client.Schema.Create(context.Background(),
		migrate.WithDropColumn(true),
		migrate.WithDropIndex(true),
	); err != nil {
		return fmt.Errorf("ent schema migration: %w", err)
	}

	if _, err := raw.ExecContext(context.Background(),
		`CREATE UNIQUE INDEX IF NOT EXISTS one_active_session ON work_sessions (person_id) WHERE end_at IS NULL`,
	); err != nil {
		return fmt.Errorf("partial unique index: %w", err)
	}

	log.Println("database migration completed")
	return nil
}
