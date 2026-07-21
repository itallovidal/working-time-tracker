package database

import (
	"database/sql"
	"fmt"
	"log"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"working-time-tracker/ent"
)

type DB struct {
	Client *ent.Client
	Raw    *sql.DB
}

func Open(dsn string) (*DB, error) {
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	drv := entsql.OpenDB("postgres", raw)
	client := ent.NewClient(ent.Driver(drv))

	log.Println("database connection established")
	return &DB{Client: client, Raw: raw}, nil
}
