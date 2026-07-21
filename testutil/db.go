package testutil

import (
	"database/sql"
	"log"
	"os"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"working-time-tracker/ent"
)

func Setup() (*ent.Client, *sql.DB) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open test database: %v", err)
	}

	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))

	return client, db
}
