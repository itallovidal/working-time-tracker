package database_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	os.Exit(m.Run())
}

func TestAutoMigrate_Idempotent(t *testing.T) {
	for i := 0; i < 2; i++ {
		if err := database.AutoMigrate(testClient); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}
}

func TestAutoMigrate_ForeignKeyDeleteRules(t *testing.T) {
	if err := database.AutoMigrate(testClient); err != nil {
		t.Fatalf("migration: %v", err)
	}

	want := map[string]string{
		"persons_organizations_persons":             "CASCADE",
		"integrations_projects_integrations":        "CASCADE",
		"tasks_projects_tasks":                      "CASCADE",
		"teams_projects_teams":                      "CASCADE",
		"team_memberships_teams_memberships":        "CASCADE",
		"work_sessions_tasks_work_sessions":         "CASCADE",
		"customers_organizations_customers":         "CASCADE",
		"allocations_projects_allocations":          "CASCADE",
		"allocations_persons_allocations":           "CASCADE",
		"projects_customers_projects":               "SET NULL",
		"tasks_integrations_tasks":                  "SET NULL",
		"projects_organizations_projects":           "NO ACTION",
		"tasks_persons_tasks":                       "NO ACTION",
		"team_memberships_persons_team_memberships": "NO ACTION",
		"work_sessions_persons_work_sessions":       "NO ACTION",
	}

	rows, err := testDB.Query(`SELECT constraint_name, delete_rule
		FROM information_schema.referential_constraints
		WHERE constraint_schema = current_schema()`)
	if err != nil {
		t.Fatalf("query constraints: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var name, rule string
		if err := rows.Scan(&name, &rule); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = rule
	}

	for name, rule := range want {
		if got[name] != rule {
			t.Errorf("%s: delete rule = %q, want %q", name, got[name], rule)
		}
	}
}

func TestAutoMigrate_OneActiveSessionIndex(t *testing.T) {
	if err := database.AutoMigrate(testClient); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var def string
	err := testDB.QueryRow(`SELECT indexdef FROM pg_indexes
		WHERE tablename = 'work_sessions' AND indexname = 'one_active_session'`).Scan(&def)
	if err != nil {
		t.Fatalf("one_active_session index not found: %v", err)
	}
	if !strings.Contains(def, "UNIQUE") || !strings.Contains(def, "(person_id)") || !strings.Contains(def, "end_at IS NULL") {
		t.Errorf("unexpected index definition: %s", def)
	}
}

// O índice parcial é a garantia final contra corrida no clock-in: mesmo gravando
// direto no banco, sem passar pelo service, a segunda sessão aberta é rejeitada.
func TestAutoMigrate_DatabaseRejectsSecondOpenSession(t *testing.T) {
	if err := database.AutoMigrate(testClient); err != nil {
		t.Fatalf("migration: %v", err)
	}
	testutil.Truncate(t, testDB)
	ctx := context.Background()

	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	p := testClient.Person.Create().SetName("Ana").SetEmail("ana@test.com").SetOrganizationID(org.ID).SaveX(ctx)
	proj := testClient.Project.Create().SetName("Projeto").SetOrganizationID(org.ID).SaveX(ctx)
	task := testClient.Task.Create().SetName("Tarefa").SetProjectID(proj.ID).SetAssigneeID(p.ID).SaveX(ctx)

	open := func() error {
		_, err := testClient.WorkSession.Create().
			SetTaskID(task.ID).SetPersonID(p.ID).SetStartAt(time.Now()).
			Save(ctx)
		return err
	}
	if err := open(); err != nil {
		t.Fatalf("first open session: %v", err)
	}
	if err := open(); err == nil {
		t.Fatal("expected the database to reject a second open session for the same person")
	}

	// Sessões encerradas não contam para o índice.
	closed := testClient.WorkSession.Create().
		SetTaskID(task.ID).SetPersonID(p.ID).
		SetStartAt(time.Now().Add(-2 * time.Hour)).SetEndAt(time.Now().Add(-time.Hour))
	if _, err := closed.Save(ctx); err != nil {
		t.Fatalf("closed session should be allowed alongside an open one: %v", err)
	}
}
