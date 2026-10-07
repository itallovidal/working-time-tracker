package database_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	amigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"

	"working-time-tracker/ent"
	"working-time-tracker/ent/migrate"
	"working-time-tracker/internal/database"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

// testutil.Setup já recria o schema e aplica as migrações: os testes abaixo olham o
// banco que os arquivos de migrations/ produzem.
func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	os.Exit(m.Run())
}

func TestMigrate_Idempotent(t *testing.T) {
	before := appliedVersions(t)
	if len(before) == 0 {
		t.Fatal("expected at least one applied migration after setup")
	}
	if err := database.Migrate(context.Background(), testDB); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if after := appliedVersions(t); !slices.Equal(before, after) {
		t.Errorf("applied versions changed on the second run: %v, want %v", after, before)
	}
}

// Se este teste falhar, o ent/schema mudou sem um arquivo de migração, ou um arquivo
// mudou o banco sem o ent/schema acompanhar.
func TestMigrate_NoSchemaDrift(t *testing.T) {
	// WriteTo calcula o que o auto migrate do Ent faria e escreve o SQL no buffer, sem
	// executar nada. Depois das migrações, não pode sobrar nada a fazer.
	var pending bytes.Buffer
	if err := testClient.Schema.WriteTo(context.Background(), &pending,
		migrate.WithDropColumn(true), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("schema diff: %v", err)
	}
	if pending.Len() > 0 {
		t.Fatalf("ent/schema and the migrations diverge; run `go run ./cmd/migrate new <name>` and review the file:\n%s", pending.String())
	}

	// O diff do Ent só olha as tabelas que ele declara: uma tabela que existisse só nas
	// migrações passaria despercebida.
	want := []string{"goose_db_version"}
	for _, table := range migrate.Tables {
		want = append(want, table.Name)
	}
	slices.Sort(want)
	if got := tableNames(t); !slices.Equal(got, want) {
		t.Errorf("tables = %v, want %v", got, want)
	}
}

func TestMigrate_RefusesLegacyDatabase(t *testing.T) {
	cases := []struct {
		name         string
		historyTable bool
	}{
		{"tables without a history table", false},
		{"history table without applied migrations", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			testutil.ResetSchema(t, testDB)
			t.Cleanup(func() { restoreSchema(t) })

			if tc.historyTable {
				// Num banco vazio, consultar o status cria a tabela de versões.
				if _, err := database.Status(ctx, testDB); err != nil {
					t.Fatalf("status on an empty database: %v", err)
				}
			}
			// Uma tabela qualquer, como a que o auto migrate antigo deixava.
			if _, err := testDB.Exec(`CREATE TABLE organizations (id uuid PRIMARY KEY)`); err != nil {
				t.Fatalf("create legacy table: %v", err)
			}

			err := database.Migrate(ctx, testDB)
			if !errors.Is(err, database.ErrLegacyDatabase) {
				t.Fatalf("err = %v, want ErrLegacyDatabase", err)
			}
			tables := tableNames(t)
			if slices.Contains(tables, "persons") {
				t.Error("a refused database must not receive any migration")
			}
			if !tc.historyTable && slices.Contains(tables, "goose_db_version") {
				t.Error("a refused database must not receive the history table")
			}
		})
	}
}

func TestMigrate_RefusesUnknownMigration(t *testing.T) {
	const unknown = 99990101000000
	if _, err := testDB.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES ($1, true)`, unknown); err != nil {
		t.Fatalf("insert unknown version: %v", err)
	}
	t.Cleanup(func() {
		if _, err := testDB.Exec(`DELETE FROM goose_db_version WHERE version_id = $1`, unknown); err != nil {
			t.Fatalf("delete unknown version: %v", err)
		}
	})

	err := database.Migrate(context.Background(), testDB)
	if !errors.Is(err, database.ErrUnknownMigration) {
		t.Fatalf("err = %v, want ErrUnknownMigration", err)
	}
}

// Um arquivo de migração editado à mão sem `go run ./cmd/migrate checksum` deixa o
// atlas.sum desatualizado, e o gerador passa a recusar o diretório.
func TestMigrations_SumFileIsCurrent(t *testing.T) {
	dir, err := sqltool.NewGooseDir("migrations")
	if err != nil {
		t.Fatalf("open migrations: %v", err)
	}
	if err := amigrate.Validate(dir); err != nil {
		t.Fatalf("atlas.sum does not match the migration files; run `go run ./cmd/migrate checksum`: %v", err)
	}
}

func TestMigrate_ForeignKeyDeleteRules(t *testing.T) {
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
		"invites_organizations_invites":             "CASCADE",
		"sessions_persons_sessions":                 "CASCADE",
		"labels_projects_labels":                    "CASCADE",
		"task_labels_task_id":                       "CASCADE",
		"task_labels_label_id":                      "CASCADE",
		"projects_customers_projects":               "SET NULL",
		"tasks_integrations_tasks":                  "SET NULL",
		"invites_persons_created_invites":           "SET NULL",
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
	// Uma FK nova precisa entrar na lista acima: a regra de delete decide o que some
	// junto quando um projeto ou uma pessoa é apagada.
	for name, rule := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s (%s) is not covered by this test", name, rule)
		}
	}
}

func TestMigrate_OneActiveSessionIndex(t *testing.T) {
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
func TestMigrate_DatabaseRejectsSecondOpenSession(t *testing.T) {
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

// restoreSchema devolve o banco ao estado que os outros testes esperam.
func restoreSchema(t *testing.T) {
	t.Helper()
	testutil.ResetSchema(t, testDB)
	if err := database.Migrate(context.Background(), testDB); err != nil {
		t.Fatalf("restore schema: %v", err)
	}
}

func appliedVersions(t *testing.T) []int64 {
	t.Helper()
	return queryColumn[int64](t, `SELECT version_id FROM goose_db_version WHERE version_id > 0 ORDER BY version_id`)
}

func tableNames(t *testing.T) []string {
	t.Helper()
	return queryColumn[string](t, `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY tablename`)
}

func queryColumn[T any](t *testing.T, query string) []T {
	t.Helper()
	rows, err := testDB.Query(query)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var values []T
	for rows.Next() {
		var v T
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return values
}
