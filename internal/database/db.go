package database

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
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
		&organization.Organization{},
		&person.Person{},
		&project.Project{},
		&team.Team{},
		&team.TeamMembership{},
		&task.Task{},
		&work_session.WorkSession{},
		&integration.Integration{},
	)
	if err != nil {
		return fmt.Errorf("auto-migrate: %w", err)
	}

	err = db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS one_active_session ON work_sessions (person_id) WHERE end_at IS NULL`,
	).Error
	if err != nil {
		return fmt.Errorf("partial unique index: %w", err)
	}

	log.Println("database migration completed")
	return nil
}
