// Comando das migrações do banco: gera os arquivos de internal/database/migrations a
// partir do ent/schema, recalcula o checksum, mostra o que já foi aplicado e aplica o
// que falta. Rode da raiz do repositório.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/joho/godotenv"

	"working-time-tracker/internal/database"
)

const migrationsDir = "internal/database/migrations"

const usage = `uso: go run ./cmd/migrate <comando>

  new <nome>   gera uma migração com a diferença entre os arquivos e o ent/schema
  checksum     recalcula o atlas.sum depois de editar um arquivo à mão
  status       lista as migrações e quais já foram aplicadas no banco
  up           aplica as migrações pendentes (o servidor e o seed já fazem isso ao iniciar)`

func main() {
	godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "new":
		if len(args) != 2 {
			return errors.New("uso: go run ./cmd/migrate new <nome>")
		}
		dsn, err := databaseURL()
		if err != nil {
			return err
		}
		return newMigration(ctx, dsn, args[1])
	case "checksum":
		return writeChecksum(migrationsDir)
	case "status":
		return status(ctx)
	case "up":
		return up(ctx)
	default:
		return fmt.Errorf("comando desconhecido %q\n\n%s", args[0], usage)
	}
}

func status(ctx context.Context) error {
	db, err := openDatabase()
	if err != nil {
		return err
	}
	defer db.Raw.Close()
	migrations, err := database.Status(ctx, db.Raw)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		applied := "pendente"
		if !m.AppliedAt.IsZero() {
			applied = "aplicada em " + m.AppliedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-48s %s\n", filepath.Base(m.Source.Path), applied)
	}
	return nil
}

func up(ctx context.Context) error {
	db, err := openDatabase()
	if err != nil {
		return err
	}
	defer db.Raw.Close()
	return database.Migrate(ctx, db.Raw)
}

func openDatabase() (*database.DB, error) {
	dsn, err := databaseURL()
	if err != nil {
		return nil, err
	}
	return database.Open(dsn)
}

func databaseURL() (string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return "", errors.New("defina DATABASE_URL (veja o .env.example)")
	}
	return dsn, nil
}
