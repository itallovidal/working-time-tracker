package testutil

import (
	"database/sql"
	"testing"
)

// Truncate limpa todas as tabelas do schema entre testes. Falha o teste se o
// TRUNCATE der erro, para que um nome de tabela desatualizado não passe despercebido.
func Truncate(t testing.TB, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE TABLE
		sessions, invites, work_sessions, tasks, team_memberships, teams,
		integrations, allocations, projects, customers, persons, organizations CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
