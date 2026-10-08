package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// realMigrations é a pasta das migrações, vista de cmd/migrate (onde o go test roda).
const realMigrations = "../../internal/database/migrations"

// testDSN é o servidor de teste: o gerador cria nele um banco temporário e o apaga, sem tocar nos bancos.
func testDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
}

func listDir(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	slices.Sort(names)
	return names
}

// O gerador aplica as migrações como o servidor (goose, um arquivo por transação) e compara o resultado com o
// ent/schema: nas migrações de hoje não há diferença, então nada é escrito. Quando o replay era o do Atlas, que
// roda um comando por vez, sem transação, ele quebrava numa migração com tabela temporária `ON COMMIT DROP`
// (a da Sprint 78), que o servidor aplica sem problema.
func TestGenerateMigration_NothingToGenerateOnTheRealMigrations(t *testing.T) {
	// Uma cópia das migrações: se algum dia o ent/schema andar sem migração, o arquivo gerado cai aqui e o
	// teste falha, em vez de aparecer no repositório.
	path := t.TempDir()
	for _, name := range listDir(t, realMigrations) {
		body, err := os.ReadFile(filepath.Join(realMigrations, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := listDir(t, path)

	if err := generateMigration(context.Background(), testDSN(), path, "nothing_to_do"); err != nil {
		t.Fatalf("generateMigration: %v", err)
	}
	if after := listDir(t, path); !slices.Equal(before, after) {
		t.Errorf("the generator wrote files (%v, was %v): ent/schema and the migrations diverge", after, before)
	}
}

func TestGenerateMigration_RefusesABadName(t *testing.T) {
	if err := generateMigration(context.Background(), testDSN(), t.TempDir(), "Nome Ruim"); err == nil {
		t.Error("a name with capitals and spaces must be refused")
	}
}
