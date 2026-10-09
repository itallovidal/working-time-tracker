package payment_test

import (
	"database/sql"
	"os"
	"testing"

	"working-time-tracker/ent"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	os.Exit(m.Run())
}
