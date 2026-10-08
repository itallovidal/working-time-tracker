package database_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	amigrate "ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/sqltool"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"working-time-tracker/ent"
	"working-time-tracker/ent/issuesync"
	"working-time-tracker/ent/migrate"
	entperson "working-time-tracker/ent/person"
	enttask "working-time-tracker/ent/task"
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
		"persons_organizations_persons":               "CASCADE",
		"integrations_projects_integrations":          "CASCADE",
		"tasks_projects_tasks":                        "CASCADE",
		"teams_projects_teams":                        "CASCADE",
		"team_memberships_teams_memberships":          "CASCADE",
		"work_sessions_projects_work_sessions":        "CASCADE",
		"work_session_tasks_tasks_session_links":      "CASCADE",
		"work_session_tasks_work_sessions_task_links": "CASCADE",
		"customers_organizations_customers":           "CASCADE",
		"allocations_projects_allocations":            "CASCADE",
		"allocations_persons_allocations":             "CASCADE",
		"invites_organizations_invites":               "CASCADE",
		"sessions_persons_sessions":                   "CASCADE",
		"labels_projects_labels":                      "CASCADE",
		"task_labels_task_id":                         "CASCADE",
		"task_labels_label_id":                        "CASCADE",
		"projects_customers_projects":                 "SET NULL",
		"issue_syncs_integrations_issue_syncs":        "CASCADE",
		"issue_syncs_tasks_issue_syncs":               "SET NULL",
		"invites_persons_created_invites":             "SET NULL",
		"invites_projects_invites":                    "SET NULL",
		"invites_teams_invites":                       "SET NULL",
		"projects_organizations_projects":             "NO ACTION",
		"tasks_persons_tasks":                         "NO ACTION",
		"team_memberships_persons_team_memberships":   "NO ACTION",
		"work_sessions_persons_work_sessions":         "NO ACTION",
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

	open := func() error {
		_, err := testClient.WorkSession.Create().
			SetProjectID(proj.ID).SetPersonID(p.ID).SetStartAt(time.Now()).
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
		SetProjectID(proj.ID).SetPersonID(p.ID).
		SetStartAt(time.Now().Add(-2 * time.Hour)).SetEndAt(time.Now().Add(-time.Hour))
	if _, err := closed.Save(ctx); err != nil {
		t.Fatalf("closed session should be allowed alongside an open one: %v", err)
	}
}

// Uma organização tem no máximo um dono: o índice parcial barra o segundo, e quem não é
// dono não conta.
func TestMigrate_OneOwnerPerOrganization(t *testing.T) {
	testutil.Truncate(t, testDB)
	ctx := context.Background()

	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	other := testClient.Organization.Create().SetName("Outra").SaveX(ctx)
	person := func(orgID uuid.UUID, email string, owner bool) error {
		_, err := testClient.Person.Create().SetName(email).SetEmail(email).SetOrganizationID(orgID).SetIsOwner(owner).Save(ctx)
		return err
	}
	if err := person(org.ID, "ana@test.com", true); err != nil {
		t.Fatalf("first owner: %v", err)
	}
	if err := person(org.ID, "bia@test.com", true); err == nil {
		t.Error("expected the database to reject a second owner in the same organization")
	}
	if err := person(org.ID, "caio@test.com", false); err != nil {
		t.Errorf("people who are not the owner are unlimited: %v", err)
	}
	if err := person(other.ID, "zeca@test.com", true); err != nil {
		t.Errorf("another organization has its own owner: %v", err)
	}
}

// A migração torna dono o admin mais antigo de cada organização que já existia. O
// teste roda o UPDATE do arquivo de migração sobre pessoas sem dono.
func TestMigrate_OwnerMigrationPicksTheOldestAdmin(t *testing.T) {
	testutil.Truncate(t, testDB)
	ctx := context.Background()

	raw, err := os.ReadFile("migrations/20261008020000_organization_owner.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	text := string(raw)
	from := strings.Index(text, `UPDATE "persons"`)
	to := strings.Index(text[from:], ");")
	if from < 0 || to < 0 {
		t.Fatal("the owner migration has no UPDATE statement to test")
	}
	update := text[from : from+to+1]

	now := time.Now()
	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	other := testClient.Organization.Create().SetName("Outra").SaveX(ctx)
	person := func(orgID uuid.UUID, email, role string, age time.Duration) {
		testClient.Person.Create().SetName(email).SetEmail(email).SetOrganizationID(orgID).
			SetRole(entperson.Role(role)).SetCreatedAt(now.Add(-age)).SaveX(ctx)
	}
	person(org.ID, "member-old@test.com", "member", 10*time.Hour)
	person(org.ID, "admin-old@test.com", "admin", 9*time.Hour)
	person(org.ID, "admin-new@test.com", "admin", time.Hour)
	person(other.ID, "only-member@test.com", "member", time.Hour)

	if _, err := testDB.ExecContext(ctx, update); err != nil {
		t.Fatalf("run the owner update: %v", err)
	}
	owners := queryColumn[string](t, `SELECT email FROM persons WHERE is_owner ORDER BY email`)
	if !slices.Equal(owners, []string{"admin-old@test.com"}) {
		t.Errorf("owners = %v, want only the oldest admin of the organization (and none where there is no admin)", owners)
	}
}

// A migração das tarefas da sessão roda sobre um banco com sessões de antes: cada uma passa
// a ser do projeto da tarefa que tinha e ganha um intervalo dessa tarefa, do início ao fim
// (aberto, se a sessão estava aberta).
func TestMigrate_SessionTasksMigrationKeepsTheOldSessions(t *testing.T) {
	ctx := context.Background()
	defer restoreSchema(t)
	testutil.ResetSchema(t, testDB)

	const migration = "20261008050000_session_tasks.sql"
	files, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	before := fstest.MapFS{}
	var last string
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".sql") || f.Name() >= migration {
			continue
		}
		raw, err := os.ReadFile("migrations/" + f.Name())
		if err != nil {
			t.Fatalf("read %s: %v", f.Name(), err)
		}
		before[f.Name()] = &fstest.MapFile{Data: raw}
		last = f.Name()
	}
	if last == "" {
		t.Fatal("no migration before the session tasks one")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, testDB, before)
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply the migrations before %s: %v", migration, err)
	}

	// O banco como era: a sessão tem task_id e nenhuma tabela de intervalos.
	org, ana, prjA, prjB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	taskA, taskB, closedID, openID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	start := time.Now().Add(-3 * time.Hour).Truncate(time.Second).UTC()
	end := start.Add(time.Hour)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, created_at) VALUES ($1, 'Org', now())`, []any{org}},
		{`INSERT INTO persons (id, name, email, organization_id, created_at) VALUES ($1, 'Ana', 'ana@test.com', $2, now())`, []any{ana, org}},
		{`INSERT INTO projects (id, name, organization_id, created_at) VALUES ($1, 'A', $3, now()), ($2, 'B', $3, now())`, []any{prjA, prjB, org}},
		{`INSERT INTO tasks (id, name, project_id, created_at) VALUES ($1, 'Tarefa A', $3, now()), ($2, 'Tarefa B', $4, now())`, []any{taskA, taskB, prjA, prjB}},
		{`INSERT INTO work_sessions (id, person_id, task_id, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, now())`, []any{closedID, ana, taskA, start, end}},
		{`INSERT INTO work_sessions (id, person_id, task_id, start_at, created_at) VALUES ($1, $2, $3, $4, now())`, []any{openID, ana, taskB, end.Add(time.Hour)}},
	} {
		if _, err := testDB.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed the old schema: %v\n%s", err, q.sql)
		}
	}

	raw, err := os.ReadFile("migrations/" + migration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, string(raw)); err != nil {
		t.Fatalf("run %s over the old sessions: %v", migration, err)
	}

	rows, err := testDB.QueryContext(ctx, `SELECT s.id, s.project_id, l.task_id, l.from_at, l.until_at
		FROM work_sessions s JOIN work_session_tasks l ON l.session_id = s.id ORDER BY s.start_at`)
	if err != nil {
		t.Fatalf("query the migrated sessions: %v", err)
	}
	defer rows.Close()
	type migrated struct {
		session, project, task uuid.UUID
		from                   time.Time
		until                  sql.NullTime
	}
	var got []migrated
	for rows.Next() {
		var m migrated
		if err := rows.Scan(&m.session, &m.project, &m.task, &m.from, &m.until); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, m)
	}
	if len(got) != 2 {
		t.Fatalf("got %d session-task rows, want one per old session (2)", len(got))
	}
	if got[0].session != closedID || got[0].project != prjA || got[0].task != taskA ||
		!got[0].from.Equal(start) || !got[0].until.Valid || !got[0].until.Time.Equal(end) {
		t.Errorf("closed session migrated as %+v, want project A, task A from %v until %v", got[0], start, end)
	}
	if got[1].session != openID || got[1].project != prjB || got[1].task != taskB || got[1].until.Valid {
		t.Errorf("open session migrated as %+v, want project B, task B, still open", got[1])
	}
}

// A migração que troca o número da issue pela chave do item (texto) não pode perder os vínculos que já
// existem, nem o índice que impede dois vínculos para o mesmo item, nem a tarefa solta que vira lápide.
func TestMigrate_IssueSyncItemIDKeepsTheLinks(t *testing.T) {
	ctx := context.Background()
	defer restoreSchema(t)
	testutil.ResetSchema(t, testDB)

	const migration = "20261008070000_issue_sync_item_id.sql"
	files, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	before := fstest.MapFS{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".sql") || f.Name() >= migration {
			continue
		}
		raw, err := os.ReadFile("migrations/" + f.Name())
		if err != nil {
			t.Fatalf("read %s: %v", f.Name(), err)
		}
		before[f.Name()] = &fstest.MapFile{Data: raw}
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, testDB, before)
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply the migrations before %s: %v", migration, err)
	}

	// O banco como era: o vínculo guarda o número da issue.
	org, prj, integ, taskA := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	linkA, tombstone := uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, created_at) VALUES ($1, 'Org', now())`, []any{org}},
		{`INSERT INTO projects (id, name, organization_id, created_at) VALUES ($1, 'A', $2, now())`, []any{prj, org}},
		{`INSERT INTO integrations (id, type, display_name, project_id, created_at) VALUES ($1, 'github', 'GitHub', $2, now())`, []any{integ, prj}},
		{`INSERT INTO tasks (id, name, project_id, created_at) VALUES ($1, 'Tarefa A', $2, now())`, []any{taskA, prj}},
		{`INSERT INTO issue_syncs (id, issue_number, title, integration_id, task_id, synced_at, created_at) VALUES ($1, 7, 'Sete', $2, $3, now(), now())`, []any{linkA, integ, taskA}},
		{`INSERT INTO issue_syncs (id, issue_number, integration_id, synced_at, created_at) VALUES ($1, 12, $2, now(), now())`, []any{tombstone, integ}},
	} {
		if _, err := testDB.ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed the old schema: %v\n%s", err, q.sql)
		}
	}

	raw, err := os.ReadFile("migrations/" + migration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, string(raw)); err != nil {
		t.Fatalf("run %s over the old links: %v", migration, err)
	}

	type link struct {
		item     string
		title    string
		task     uuid.NullUUID
		deadline sql.NullTime
	}
	read := func(id uuid.UUID) link {
		var l link
		if err := testDB.QueryRowContext(ctx, `SELECT item_id, title, task_id, deadline FROM issue_syncs WHERE id = $1`, id).
			Scan(&l.item, &l.title, &l.task, &l.deadline); err != nil {
			t.Fatalf("read the migrated link: %v", err)
		}
		return l
	}
	if got := read(linkA); got.item != "7" || got.title != "Sete" || !got.task.Valid || got.task.UUID != taskA || got.deadline.Valid {
		t.Errorf("link migrated as %+v, want item 7 kept with its task and no deadline", got)
	}
	if got := read(tombstone); got.item != "12" || got.task.Valid {
		t.Errorf("tombstone migrated as %+v, want item 12 with no task", got)
	}
	if _, err := testDB.ExecContext(ctx, `INSERT INTO issue_syncs (id, item_id, integration_id, synced_at, created_at) VALUES ($1, '7', $2, now(), now())`, uuid.New(), integ); err == nil {
		t.Error("the unique index must still refuse a second link for the same item of the integration")
	}
	if _, err := testDB.ExecContext(ctx, `INSERT INTO issue_syncs (id, item_id, integration_id, synced_at, created_at) VALUES ($1, 'H0TZyzbK', $2, now(), now())`, uuid.New(), integ); err != nil {
		t.Errorf("an alphanumeric item key (a Trello card) must be accepted: %v", err)
	}
}

// A migração que tira o vínculo das colunas da tarefa e o deixa só em issue_syncs não pode perder nenhum
// vínculo de hoje: o que já tinha linha a mantém (com o endereço), o que foi ligado à mão ganha uma linha
// pending, o que já não aponta mais para a linha a solta, e a tarefa mais antiga vence quando duas pedem o
// mesmo item.
func TestMigrate_IssueSyncLinksKeepsTheLegacyLinks(t *testing.T) {
	ctx := context.Background()
	defer restoreSchema(t)
	testutil.ResetSchema(t, testDB)

	const migration = "20261008080000_issue_sync_links.sql"
	files, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	before := fstest.MapFS{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".sql") || f.Name() >= migration {
			continue
		}
		raw, err := os.ReadFile("migrations/" + f.Name())
		if err != nil {
			t.Fatalf("read %s: %v", f.Name(), err)
		}
		before[f.Name()] = &fstest.MapFile{Data: raw}
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, testDB, before)
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply the migrations before %s: %v", migration, err)
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := testDB.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed the old schema: %v\n%s", err, query)
		}
	}
	org, prj, gh, trello := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO organizations (id, name, created_at) VALUES ($1, 'Org', now())`, org)
	exec(`INSERT INTO projects (id, name, organization_id, created_at) VALUES ($1, 'A', $2, now())`, prj, org)
	exec(`INSERT INTO integrations (id, type, display_name, project_id, created_at) VALUES ($1, 'github', 'GitHub', $2, now())`, gh, prj)
	exec(`INSERT INTO integrations (id, type, display_name, project_id, created_at) VALUES ($1, 'trello', 'Trello', $2, now())`, trello, prj)

	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	nTask := 0
	newTask := func(integ *uuid.UUID, item, url string) uuid.UUID {
		id := uuid.New()
		nTask++
		var itemArg, urlArg any
		if item != "" {
			itemArg, urlArg = item, url
		}
		exec(`INSERT INTO tasks (id, name, project_id, external_integration_id, external_item_id, external_item_url, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, "T"+strconv.Itoa(nTask), prj, integ, itemArg, urlArg, day.Add(time.Duration(nTask)*time.Hour))
		return id
	}
	newRow := func(integ uuid.UUID, item, title string, taskID *uuid.UUID) {
		exec(`INSERT INTO issue_syncs (id, item_id, title, integration_id, task_id, synced_at, created_at)
			VALUES ($1, $2, $3, $4, $5, now(), now())`, uuid.New(), item, title, integ, taskID)
	}

	// Já sincronizada: a linha existe, o endereço vem da tarefa.
	bound := newTask(&gh, "7", "https://github.com/o/r/issues/7")
	newRow(gh, "7", "Sete", &bound)
	// Ligada à mão no GitHub e no Trello (pela URL do cartão), sem linha.
	manualGH := newTask(&gh, "9", "https://github.com/o/r/issues/9")
	manualTrello := newTask(&trello, "https://trello.com/c/AbC123/3-cartao", "https://trello.com/c/AbC123/3-cartao")
	// Desligada: a linha segue presa à tarefa, que já não aponta para o item.
	unlinked := newTask(nil, "", "")
	newRow(gh, "12", "Doze", &unlinked)
	// Religada a outro item: a linha do antigo é solta e o novo ganha a sua.
	moved := newTask(&gh, "31", "")
	newRow(gh, "30", "Trinta", &moved)
	// Duas tarefas pedem o mesmo item: a mais antiga vence.
	oldest := newTask(&gh, "20", "")
	youngest := newTask(&gh, "20", "")
	// Um item descartado que uma tarefa volta a pedir à mão: é religado, sem o acordo antigo.
	revived := newTask(&gh, "40", "")
	newRow(gh, "40", "Quarenta", nil)
	// A integração do vínculo foi apagada: a coluna ficou nula e o item sem dono.
	orphan := newTask(nil, "5", "")

	raw, err := os.ReadFile("migrations/" + migration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	// O goose roda o arquivo numa transação (a tabela temporária some no commit); aqui, igual.
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
		_ = tx.Rollback()
		t.Fatalf("run %s over the old links: %v", migration, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	type link struct {
		state, title, url string
		task              uuid.NullUUID
		found             bool
	}
	read := func(integ uuid.UUID, item string) link {
		l := link{found: true}
		err := testDB.QueryRowContext(ctx, `SELECT state, title, url, task_id FROM issue_syncs WHERE integration_id = $1 AND item_id = $2`, integ, item).
			Scan(&l.state, &l.title, &l.url, &l.task)
		if errors.Is(err, sql.ErrNoRows) {
			return link{}
		}
		if err != nil {
			t.Fatalf("read %s: %v", item, err)
		}
		return l
	}
	want := func(name string, got link, state, title, url string, task *uuid.UUID) {
		t.Helper()
		if !got.found {
			t.Errorf("%s: no link row", name)
			return
		}
		if got.state != state || got.title != title || got.url != url || got.task.Valid != (task != nil) || (task != nil && got.task.UUID != *task) {
			t.Errorf("%s migrated as %+v, want state %q, title %q, url %q, task %v", name, got, state, title, url, task)
		}
	}

	want("bound", read(gh, "7"), "open", "Sete", "https://github.com/o/r/issues/7", &bound)
	want("hand-linked GitHub", read(gh, "9"), "pending", "", "https://github.com/o/r/issues/9", &manualGH)
	want("hand-linked Trello", read(trello, "AbC123"), "pending", "", "https://trello.com/c/AbC123/3-cartao", &manualTrello)
	want("unlinked", read(gh, "12"), "open", "Doze", "", nil)
	want("moved, old item", read(gh, "30"), "open", "Trinta", "", nil)
	want("moved, new item", read(gh, "31"), "pending", "", "", &moved)
	want("wanted twice", read(gh, "20"), "pending", "", "", &oldest)
	want("revived", read(gh, "40"), "pending", "", "", &revived)
	if n := queryColumn[int](t, `SELECT count(*) FROM issue_syncs WHERE task_id IN ('`+youngest.String()+`', '`+orphan.String()+`')`); n[0] != 0 {
		t.Errorf("the younger task of the same item and the task of a deleted integration must end with no link, got %d", n[0])
	}
	if n := queryColumn[int](t, `SELECT count(*) FROM issue_syncs`); n[0] != 8 {
		t.Errorf("want 8 link rows (7, 9, AbC123, 12, 30, 31, 20, 40), got %d", n[0])
	}
	for _, col := range []string{"external_integration_id", "external_item_id", "external_item_url"} {
		if n := queryColumn[int](t, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'tasks' AND column_name = '`+col+`'`); n[0] != 0 {
			t.Errorf("tasks.%s must be gone", col)
		}
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

// O vínculo item ↔ tarefa: um item por chave e integração, uma tarefa por item em cada integração (e a mesma
// tarefa pode estar em integrações diferentes); excluir a tarefa deixa a linha como lápide (task_id nulo, o item
// não volta) e excluir a integração leva as linhas.
func TestMigrate_IssueSyncRules(t *testing.T) {
	testutil.Truncate(t, testDB)
	ctx := context.Background()

	org := testClient.Organization.Create().SetName("Org").SaveX(ctx)
	proj := testClient.Project.Create().SetName("Projeto").SetOrganizationID(org.ID).SaveX(ctx)
	integ := testClient.Integration.Create().SetProjectID(proj.ID).SetType("github").SetDisplayName("GitHub").SaveX(ctx)
	other := testClient.Integration.Create().SetProjectID(proj.ID).SetType("trello").SetDisplayName("Trello").SaveX(ctx)
	task := func(name string) *ent.Task {
		return testClient.Task.Create().SetProjectID(proj.ID).SetName(name).SaveX(ctx)
	}
	linkTo := func(in *ent.Integration, item string, taskID *uuid.UUID) error {
		_, err := testClient.IssueSync.Create().SetIntegrationID(in.ID).SetItemID(item).SetNillableTaskID(taskID).Save(ctx)
		return err
	}
	link := func(number int, taskID *uuid.UUID) error { return linkTo(integ, strconv.Itoa(number), taskID) }

	a, b := task("A"), task("B")
	if err := link(1, &a.ID); err != nil {
		t.Fatalf("first link: %v", err)
	}
	if err := link(1, &b.ID); err == nil {
		t.Error("the database must refuse two links for the same issue number of an integration")
	}
	if err := link(2, &a.ID); err == nil {
		t.Error("the database must refuse two items of the same integration for the same task")
	}
	if err := linkTo(other, "H0TZyzbK", &a.ID); err != nil {
		t.Errorf("the same task must be allowed one item in each integration: %v", err)
	}
	if err := linkTo(other, "Zz9yXwVu", &a.ID); err == nil {
		t.Error("the database must refuse a second card of the same integration for the task")
	}
	if err := link(3, nil); err != nil {
		t.Errorf("a tombstone (no task) must be allowed: %v", err)
	}
	if err := link(4, nil); err != nil {
		t.Errorf("several tombstones must be allowed: %v", err)
	}

	testClient.Task.DeleteOneID(a.ID).ExecX(ctx)
	row := testClient.IssueSync.Query().Where(issuesync.ItemIDEQ("1")).OnlyX(ctx)
	if row.TaskID != nil {
		t.Errorf("deleting the task must leave the link with no task, got %v", row.TaskID)
	}

	testClient.Integration.DeleteOneID(integ.ID).ExecX(ctx)
	if n := testClient.IssueSync.Query().Where(issuesync.IntegrationID(integ.ID)).CountX(ctx); n != 0 {
		t.Errorf("deleting the integration must delete its links, %d left", n)
	}
	if n := testClient.IssueSync.Query().Where(issuesync.IntegrationID(other.ID)).CountX(ctx); n != 1 {
		t.Errorf("deleting an integration must keep the links of the other, got %d", n)
	}
	if !testClient.Task.Query().Where(enttask.IDEQ(b.ID)).ExistX(ctx) {
		t.Error("deleting the integration must keep the tasks")
	}
}
