package service

import (
	"log"
	"os"
	"testing"
	"working-time-tracker/internal/store"

	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
	}

	var err error
	testDB, err = store.Open(dsn)
	if err != nil {
		log.Fatalf("failed to connect to test database: %v", err)
	}

	if err := store.AutoMigrate(testDB); err != nil {
		log.Fatalf("failed to migrate test database: %v", err)
	}

	code := m.Run()
	os.Exit(code)
}

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE work_sessions CASCADE")
	testDB.Exec("TRUNCATE TABLE tasks CASCADE")
	testDB.Exec("TRUNCATE TABLE team_memberships CASCADE")
	testDB.Exec("TRUNCATE TABLE integrations CASCADE")
	testDB.Exec("TRUNCATE TABLE teams CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE persons CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}
