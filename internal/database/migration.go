package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Os arquivos de migrations/ são gerados por `go run ./cmd/migrate new <nome>` a partir
// do ent/schema, revisados e commitados. Só eles mudam o schema do banco.
//
//go:embed migrations
var migrationFiles embed.FS

var (
	// ErrLegacyDatabase indica um banco com tabelas e sem histórico de migração: foi
	// criado pelo auto migrate antigo, ou à mão.
	ErrLegacyDatabase = errors.New("database has tables but no migration history")
	// ErrUnknownMigration indica um banco com uma migração que este binário não tem:
	// o binário é mais antigo que o banco, ou os arquivos de migração foram refeitos.
	ErrUnknownMigration = errors.New("database has a migration this build does not know")
)

// Migrate aplica, em ordem, as migrações que ainda não rodaram neste banco. Cada
// arquivo roda numa transação, e um advisory lock impede dois processos de migrarem ao
// mesmo tempo. Nada é calculado na hora: só roda o SQL que está nos arquivos.
func Migrate(ctx context.Context, db *sql.DB) error {
	p, err := newProvider(db)
	if err != nil {
		return err
	}
	if err := checkHistory(ctx, db, p.ListSources()); err != nil {
		return err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	for _, r := range results {
		log.Printf("migration applied: %s", r)
	}
	log.Println("database schema is up to date")
	return nil
}

// Status lista as migrações conhecidas e quais já foram aplicadas neste banco.
func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := newProvider(db)
	if err != nil {
		return nil, err
	}
	if err := checkHistory(ctx, db, p.ListSources()); err != nil {
		return nil, err
	}
	return p.Status(ctx)
}

func newProvider(db *sql.DB) (*goose.Provider, error) {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	return p, nil
}

// checkHistory recusa os bancos que as migrações não sabem continuar. Só lê, e roda
// antes de qualquer chamada ao goose, porque todas elas criam a tabela de versões.
func checkHistory(ctx context.Context, db *sql.DB, sources []*goose.Source) error {
	var (
		name       string
		hasHistory bool
		appTables  int
	)
	err := db.QueryRowContext(ctx, `SELECT current_database(),
			count(*) FILTER (WHERE tablename = $1) > 0,
			count(*) FILTER (WHERE tablename <> $1)
		FROM pg_tables WHERE schemaname = current_schema()`, goose.DefaultTablename,
	).Scan(&name, &hasHistory, &appTables)
	if err != nil {
		return fmt.Errorf("reading migration history: %w", err)
	}

	var applied []int64
	if hasHistory {
		// A versão 0 é a linha que o goose grava ao criar a tabela; não é uma migração.
		rows, err := db.QueryContext(ctx,
			`SELECT version_id FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND is_applied`)
		if err != nil {
			return fmt.Errorf("reading migration history: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			if err := rows.Scan(&v); err != nil {
				return fmt.Errorf("reading migration history: %w", err)
			}
			applied = append(applied, v)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reading migration history: %w", err)
		}
	}

	const hint = `recreate database %q (drop it, create it again, then run the seed; see "Migrações do banco" in the README)`
	if len(applied) == 0 {
		if appTables > 0 {
			return fmt.Errorf("%w: "+hint, ErrLegacyDatabase, name)
		}
		return nil
	}
	known := make(map[int64]bool, len(sources))
	for _, s := range sources {
		known[s.Version] = true
	}
	for _, v := range applied {
		if !known[v] {
			return fmt.Errorf("%w (version %d): "+hint, ErrUnknownMigration, v, name)
		}
	}
	return nil
}
