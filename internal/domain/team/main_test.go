package team_test

import (
	"database/sql"
	"os"
	"testing"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	if err := database.AutoMigrate(testClient, testDB); err != nil {
		panic(err)
	}
	code := m.Run()
	os.Exit(code)
}
