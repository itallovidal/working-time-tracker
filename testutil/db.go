package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
)

// Setup abre o banco de teste, recria o schema e aplica as migrações. Recriar a cada
// pacote garante que os testes rodam sobre o que os arquivos de migração produzem
// hoje, e não sobre o que sobrou de uma execução anterior.
func Setup() (*ent.Client, *sql.DB) {
	// Nenhum teste fala com a internet: o GitHub, o GitLab e o Trello de verdade ficam fora do alcance.
	BlockExternalHTTP()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open test database: %v", err)
	}
	if err := resetSchema(db); err != nil {
		log.Fatalf("failed to reset test database: %v", err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		log.Fatalf("failed to migrate test database: %v", err)
	}

	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))

	return client, db
}

// ResetSchema apaga tudo no banco de teste, inclusive o histórico de migrações. Serve
// aos testes que precisam montar um banco em outro estado; quem chama reaplica as
// migrações ao terminar.
func ResetSchema(t testing.TB, db *sql.DB) {
	t.Helper()
	if err := resetSchema(db); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

// resetSchema só roda em banco cujo nome termina em _test: um TEST_DATABASE_URL
// apontado para o banco errado não pode custar os dados dele.
func resetSchema(db *sql.DB) error {
	var name string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		return err
	}
	if !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("refusing to reset database %q: the test database name must end in _test", name)
	}
	if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE SCHEMA public`)
	return err
}
