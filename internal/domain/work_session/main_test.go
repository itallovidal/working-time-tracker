package work_session_test

import (
	"os"
	"testing"

	"gorm.io/gorm"

	"working-time-tracker/internal/database"
	"working-time-tracker/testutil"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	testDB = testutil.Setup()
	if err := database.AutoMigrate(testDB); err != nil {
		panic(err)
	}
	code := m.Run()
	os.Exit(code)
}
