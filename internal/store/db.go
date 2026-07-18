package store

import (
	"fmt"
	"log"

	"working-time-tracker/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	log.Println("database connection established")

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&model.Organization{},
		&model.Person{},
		&model.Project{},
		&model.Team{},
		&model.TeamMembership{},
		&model.Task{},
		&model.WorkSession{},
		&model.Integration{},
	)
	if err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	err = db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS one_active_session ON time_entries (person_id) WHERE end_at IS NULL`,
	).Error
	if err != nil {
		return fmt.Errorf("partial unique index: %w", err)
	}

	log.Println("database migration completed")
	return nil
}
