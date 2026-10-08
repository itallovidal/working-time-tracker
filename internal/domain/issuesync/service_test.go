package issuesync_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	os.Exit(m.Run())
}

const repo = "owner/repo"

// env é um projeto com uma integração do GitHub sincronizando com um GitHub fake. Ana e Bia estão no
// projeto e têm usuário no GitHub com e-mail público; Dan está no projeto mas não tem usuário; Cai é da
// organização mas não do projeto; "ghost" é um usuário do GitHub sem e-mail público e "carol-dev" tem um
// e-mail que não é de ninguém.
type env struct {
	t       *testing.T
	fake    *testutil.GitHub
	srv     *httptest.Server
	integ   *integration.Service
	tasks   *task.Store
	taskSvc *task.Service
	rows    *issuesync.Store
	syncer  *issuesync.Syncer
	it      *integration.Integration
	project string
	org     string

	ana, bia, dan, cai *person.Person
}

func newEnv(t *testing.T, cfg issuesync.Config) *env {
	t.Helper()
	testutil.Truncate(t, testDB)

	e := &env{t: t, fake: testutil.NewGitHub()}
	e.fake.AddRepo(repo) // vazio: sem a issue 42 de sempre
	for login, email := range map[string]string{"ana-dev": "ana@test.com", "bia-dev": "bia@test.com", "ghost": "", "carol-dev": "carol@elsewhere.com"} {
		e.fake.AddUser(login, email)
	}
	e.srv = httptest.NewServer(e.fake)
	t.Cleanup(e.srv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: e.srv.URL} })

	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	must(t, err)
	e.org = org.ID.String()
	proj, err := project.NewService(project.NewStore(testClient)).Create(e.org, "Projeto", "", 0, project.Routine{})
	must(t, err)
	e.project = proj.ID.String()

	people := person.NewService(person.NewStore(testClient))
	mk := func(name, email string, inProject bool) *person.Person {
		p, err := people.Create(e.org, name, email)
		must(t, err)
		if inProject {
			_, err := allocation.NewService(allocation.NewStore(testClient)).Set(e.project, p.ID.String(), 1000)
			must(t, err)
		}
		return p
	}
	e.ana, e.bia, e.dan, e.cai = mk("Ana", "ana@test.com", true), mk("Bia", "bia@test.com", true), mk("Dan", "dan@test.com", true), mk("Cai", "cai@test.com", false)

	e.integ = integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")
	e.it, err = e.integ.Create(e.project, "github", "GitHub", "ghp_test", map[string]any{"repo": repo}, true)
	must(t, err)
	yes := true
	e.it, err = e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)

	e.tasks = task.NewStore(testClient)
	e.taskSvc = task.NewService(e.tasks, team.NewMembershipStore(testClient), e.integ)
	e.rows = issuesync.NewStore(testClient)
	e.syncer = issuesync.New(issuesync.Deps{
		Integrations: e.integ, Tasks: e.tasks, People: person.NewStore(testClient),
		Members: team.NewMembershipStore(testClient), Rows: e.rows,
	}, cfg)
	e.fake.Reset()
	return e
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) sync(mode issuesync.Mode) *issuesync.Summary {
	e.t.Helper()
	sum, err := e.syncer.Sync(context.Background(), e.it.ID, mode)
	if err != nil {
		e.t.Fatalf("sync: %v", err)
	}
	return sum
}

func (e *env) add(i testutil.GitHubIssue) int { return e.fake.AddIssue(repo, i) }

func (e *env) row(number int) *issuesync.Row {
	e.t.Helper()
	rows, err := e.rows.ByIntegration(e.it.ID)
	must(e.t, err)
	return rows[strconv.Itoa(number)]
}

// taskFor devolve a tarefa que a sincronização ligou à issue.
func (e *env) taskFor(number int) *task.Task {
	e.t.Helper()
	row := e.row(number)
	if row == nil || row.TaskID == nil {
		e.t.Fatalf("issue #%d has no task (row %+v)", number, row)
	}
	tk, err := e.tasks.GetByID(row.TaskID.String())
	must(e.t, err)
	return tk
}

func (e *env) tasksCount() int {
	list, err := e.taskSvc.ListByProject(e.project, task.ListFilter{})
	must(e.t, err)
	return len(list)
}

func labelNames(t *task.Task) []string {
	var out []string
	for _, l := range t.Labels {
		out = append(out, l.Name)
	}
	slices.Sort(out)
	return out
}

func sorted(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

func eq(a, b []string) bool { return slices.Equal(sorted(a), sorted(b)) }

func (e *env) integration() *integration.Integration {
	e.t.Helper()
	it, err := e.integ.Get(e.it.ID.String())
	must(e.t, err)
	return it
}

// A primeira rodada traz as issues abertas e só elas, liga o responsável pelo e-mail público e não escreve
// nada no GitHub; a segunda, sem diferença, também não.
func TestSync_ImportsOpenIssues(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	one := e.add(testutil.GitHubIssue{Title: "Corrigir login", Body: "Passos:\r\n1. abrir", Labels: []string{"bug", "UX"}, Assignees: []string{"ana-dev"}})
	two := e.add(testutil.GitHubIssue{Title: "Sem dono"})
	closed := e.add(testutil.GitHubIssue{Title: "Antiga", State: "closed"})
	pr := e.add(testutil.GitHubIssue{Title: "Um pull request", PullRequest: true})
	ghost := e.add(testutil.GitHubIssue{Title: "Do fantasma", Assignees: []string{"ghost"}})
	carol := e.add(testutil.GitHubIssue{Title: "Da Carol", Assignees: []string{"carol-dev"}})

	sum := e.sync(issuesync.Full)
	if sum.Created != 4 || sum.Updated != 0 || sum.Pushed != 0 || sum.Errors != 0 || sum.Partial || sum.Unmapped != 2 {
		t.Errorf("summary = %+v, want 4 created, 2 unmapped, nothing else", sum)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("importing wrote to GitHub: %v", e.fake.Requests())
	}

	tk := e.taskFor(one)
	if tk.Name != "Corrigir login" || tk.Description != "Passos:\n1. abrir" || tk.Status != "backlog" || tk.Priority != "none" {
		t.Errorf("task = %+v", tk)
	}
	if !eq(labelNames(tk), []string{"UX", "bug"}) {
		t.Errorf("labels = %v", labelNames(tk))
	}
	if tk.AssigneeID == nil || *tk.AssigneeID != e.ana.ID {
		t.Errorf("assignee = %v, want Ana (her email is public on GitHub)", tk.AssigneeID)
	}
	if tk.ExternalIntegrationID == nil || *tk.ExternalIntegrationID != e.it.ID || tk.ExternalItemID == nil || *tk.ExternalItemID != "1" ||
		tk.ExternalItemURL == nil || *tk.ExternalItemURL != "https://github.com/owner/repo/issues/1" {
		t.Errorf("link = %v / %v / %v", tk.ExternalIntegrationID, tk.ExternalItemID, tk.ExternalItemURL)
	}
	if tk.Deadline.After(time.Date(1971, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("an imported task has no deadline, got %v", tk.Deadline)
	}
	if e.taskFor(two).AssigneeID != nil {
		t.Error("an issue with no assignee must become an available task")
	}
	if e.taskFor(ghost).AssigneeID != nil || e.taskFor(carol).AssigneeID != nil {
		t.Error("an assignee with no public email, or whose email is nobody's here, leaves the task available")
	}
	for _, n := range []int{closed, pr} {
		if e.row(n) != nil {
			t.Errorf("issue #%d must not be imported", n)
		}
	}
	if e.tasksCount() != 4 {
		t.Errorf("%d tasks, want 4", e.tasksCount())
	}

	it := e.integration()
	if it.LastSyncedAt == nil || it.LastSyncError != "" || it.SyncUnmatched != 2 {
		t.Errorf("integration after the round = synced %v, error %q, unmatched %d", it.LastSyncedAt, it.LastSyncError, it.SyncUnmatched)
	}
	if row := testClient.Integration.GetX(context.Background(), e.it.ID); row.SyncCursor == nil {
		t.Error("a round that saw everything must set the cursor")
	}

	// Sem diferença nenhuma, a rodada seguinte não escreve em lugar nenhum.
	again := e.sync(issuesync.Full)
	if *again != (issuesync.Summary{Unmapped: 2}) {
		t.Errorf("second round = %+v, want nothing done", again)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("a round with no difference wrote to GitHub: %v", e.fake.Requests())
	}
	if e.tasksCount() != 4 {
		t.Errorf("%d tasks after the second round, want 4", e.tasksCount())
	}
}

// Mais de uma página de issues.
func TestSync_ImportsManyPages(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	for n := 0; n < 230; n++ {
		e.add(testutil.GitHubIssue{Title: "Issue"})
	}
	if sum := e.sync(issuesync.Full); sum.Created != 230 || sum.Partial {
		t.Errorf("summary = %+v", sum)
	}
	if e.tasksCount() != 230 {
		t.Errorf("%d tasks", e.tasksCount())
	}
}

// O que muda no GitHub chega à tarefa; fechar e reabrir valem nos dois sentidos.
func TestSync_GitHubChangesComeIn(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	one := e.add(testutil.GitHubIssue{Title: "Corrigir login", Body: "texto", Labels: []string{"bug", "UX"}, Assignees: []string{"ana-dev"}})
	two := e.add(testutil.GitHubIssue{Title: "Outra"})
	e.sync(issuesync.Full)

	e.fake.EditIssue(repo, one, func(i *testutil.GitHubIssue) {
		i.Title = "Corrigir o login"
		i.Body = "novo\r\ntexto"
		i.Labels = []string{"bug", "api"}
		i.Assignees = []string{"bia-dev"}
	})
	e.fake.EditIssue(repo, two, func(i *testutil.GitHubIssue) { i.State = "closed" })

	sum := e.sync(issuesync.Incremental)
	if sum.Updated != 2 || sum.Closed != 1 || sum.Pushed != 0 || sum.Created != 0 {
		t.Errorf("summary = %+v", sum)
	}
	tk := e.taskFor(one)
	if tk.Name != "Corrigir o login" || tk.Description != "novo\ntexto" || !eq(labelNames(tk), []string{"api", "bug"}) ||
		tk.AssigneeID == nil || *tk.AssigneeID != e.bia.ID {
		t.Errorf("task after the GitHub edit = name %q, description %q, labels %v, assignee %v", tk.Name, tk.Description, labelNames(tk), tk.AssigneeID)
	}
	if e.taskFor(two).Status != "closed" {
		t.Errorf("task of the closed issue = %q, want closed", e.taskFor(two).Status)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to GitHub: %v", e.fake.Requests())
	}

	// Tirar o responsável no GitHub tira daqui.
	e.fake.EditIssue(repo, one, func(i *testutil.GitHubIssue) { i.Assignees = nil })
	e.sync(issuesync.Incremental)
	if e.taskFor(one).AssigneeID != nil {
		t.Error("removing the assignee on GitHub must clear it here")
	}

	// Reabrir a issue reabre a tarefa fechada, em backlog; uma tarefa que seguiu em andamento continua.
	e.fake.EditIssue(repo, two, func(i *testutil.GitHubIssue) { i.State = "open" })
	e.sync(issuesync.Incremental)
	if got := e.taskFor(two).Status; got != "backlog" {
		t.Errorf("reopened task = %q, want backlog", got)
	}
}

// O que muda aqui chega ao GitHub, e a rodada seguinte não repete.
func TestSync_LocalChangesGoOut(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	one := e.add(testutil.GitHubIssue{Title: "Corrigir login", Body: "texto", Labels: []string{"bug"}, Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	tk := e.taskFor(one)

	ux, err := e.taskSvc.CreateLabel(e.project, "Nova etiqueta")
	must(t, err)
	bug, _ := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"bug"})
	ids := []string{bug[0].ID.String(), ux.ID.String()}
	bia := e.bia.ID.String()
	if _, err := e.taskSvc.UpdateAs(bia, tk.ID.String(), "Corrigir o login (daqui)", "texto daqui", &bia, nil, task.Attrs{LabelIDs: &ids}); err != nil {
		t.Fatalf("edit the task: %v", err)
	}
	closed := "closed"
	if _, err := e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &closed}); err != nil {
		t.Fatalf("close the task: %v", err)
	}
	e.fake.Reset()

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 1 || sum.Updated != 0 || sum.Errors != 0 {
		t.Errorf("summary = %+v, want one issue pushed", sum)
	}
	issue, _ := e.fake.Issue(repo, one)
	if issue.Title != "Corrigir o login (daqui)" || issue.Body != "texto daqui" || issue.State != "closed" || issue.StateReason != "completed" {
		t.Errorf("issue after the push = %+v", issue)
	}
	if !eq(issue.Labels, []string{"bug", "Nova etiqueta"}) {
		t.Errorf("issue labels = %v; the label the repository did not have must be created first", issue.Labels)
	}
	if !eq(issue.Assignees, []string{"bia-dev", "ghost"}) {
		t.Errorf("issue assignees = %v; Bia is found by her public email and the assignee we cannot link stays", issue.Assignees)
	}
	if n := e.fake.Count("PATCH", "/repos/owner/repo/issues/"); n != 1 {
		t.Errorf("%d PATCH requests, want exactly one", n)
	}
	if !slices.Contains(e.fake.LabelNames(repo), "Nova etiqueta") {
		t.Errorf("repository labels = %v", e.fake.LabelNames(repo))
	}

	// Nenhuma escrita a mais: o acordo agora é o que está no GitHub.
	e.fake.Reset()
	for _, mode := range []issuesync.Mode{issuesync.Full, issuesync.Incremental, issuesync.Full} {
		if sum := e.sync(mode); sum.Pushed != 0 || sum.Updated != 0 || sum.Errors != 0 {
			t.Errorf("round %v after the push = %+v", mode, sum)
		}
	}
	if e.fake.Writes() != 0 {
		t.Errorf("rounds with no difference wrote to GitHub: %v", e.fake.Requests())
	}
	if row := e.row(one); row.StuckSig != "" || row.LastError != "" || row.State != "closed" {
		t.Errorf("row after the push = %+v", row)
	}

	// Reabrir aqui reabre lá.
	backlog := "backlog"
	if got, err := e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &backlog}); err != nil || got.Status != "backlog" {
		t.Fatalf("reopen the task: %v %v", got, err)
	}
	if sum := e.sync(issuesync.Full); sum.Pushed != 1 {
		t.Errorf("reopen: %+v", sum)
	}
	if issue, _ = e.fake.Issue(repo, one); issue.State != "open" {
		t.Errorf("issue state = %q, want open", issue.State)
	}
}

// Os dois lados mudaram o mesmo campo: o GitHub vence. As etiquetas não conflitam.
func TestSync_GitHubWinsConflicts(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	one := e.add(testutil.GitHubIssue{Title: "Original", Body: "corpo", Labels: []string{"bug"}})
	e.sync(issuesync.Full)
	tk := e.taskFor(one)

	api, _ := e.taskSvc.CreateLabel(e.project, "api")
	bug, _ := e.tasks.FindOrCreateLabels(uuid.MustParse(e.project), []string{"bug"})
	ids := []string{bug[0].ID.String(), api.ID.String()}
	if _, err := e.taskSvc.UpdateAs("", tk.ID.String(), "Título daqui", "corpo daqui", nil, nil, task.Attrs{LabelIDs: &ids}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	e.fake.EditIssue(repo, one, func(i *testutil.GitHubIssue) {
		i.Title = "Título do GitHub"
		i.Labels = []string{"bug", "ux"}
	})

	sum := e.sync(issuesync.Full)
	if sum.Updated != 1 || sum.Pushed != 1 {
		t.Errorf("summary = %+v", sum)
	}
	got := e.taskFor(one)
	if got.Name != "Título do GitHub" {
		t.Errorf("name = %q, want GitHub's", got.Name)
	}
	if got.Description != "corpo daqui" {
		t.Errorf("description = %q; only the body the task changed, it must go to GitHub", got.Description)
	}
	issue, _ := e.fake.Issue(repo, one)
	if issue.Title != "Título do GitHub" || issue.Body != "corpo daqui" {
		t.Errorf("issue = %q / %q", issue.Title, issue.Body)
	}
	if !eq(issue.Labels, []string{"bug", "ux", "api"}) || !eq(labelNames(got), []string{"api", "bug", "ux"}) {
		t.Errorf("labels: issue %v, task %v; both sides must end with the union", issue.Labels, labelNames(got))
	}
}

// Uma tarefa que alguém ligou à issue à mão passa a ser sincronizada, em vez de a issue virar uma tarefa
// nova; havendo duas, a mais antiga.
func TestSync_AdoptsManuallyLinkedTask(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Do GitHub", Body: "corpo", Labels: []string{"bug"}})

	mine, err := e.taskSvc.CreateAs("", e.project, "Minha tarefa", "minha descrição", e.dan.ID.String(), nil, task.Attrs{})
	must(t, err)
	ux, _ := e.taskSvc.CreateLabel(e.project, "ux")
	ids := []string{ux.ID.String()}
	e.taskSvc.UpdateAs("", mine.ID.String(), "Minha tarefa", "minha descrição", nil, nil, task.Attrs{LabelIDs: &ids})
	_, err = e.taskSvc.LinkExternalItem(mine.ID.String(), e.it.ID.String(), "1", "https://github.com/owner/repo/issues/1")
	must(t, err)
	dup, _ := e.taskSvc.CreateAs("", e.project, "Duplicada", "", "", nil, task.Attrs{})
	e.taskSvc.LinkExternalItem(dup.ID.String(), e.it.ID.String(), "1", "https://github.com/owner/repo/issues/1")

	sum := e.sync(issuesync.Full)
	if sum.Created != 0 || sum.Errors != 0 {
		t.Errorf("summary = %+v; the issue already has a task", sum)
	}
	if e.tasksCount() != 2 {
		t.Errorf("%d tasks, want the two that existed", e.tasksCount())
	}
	row := e.row(n)
	if row == nil || row.TaskID == nil || *row.TaskID != mine.ID {
		t.Fatalf("row = %+v, want it bound to the oldest linked task", row)
	}
	got := e.taskFor(n)
	if got.Name != "Do GitHub" || got.Description != "corpo" {
		t.Errorf("task = %q / %q; GitHub wins on the title and body", got.Name, got.Description)
	}
	if !eq(labelNames(got), []string{"bug", "ux"}) {
		t.Errorf("labels = %v, want the union", labelNames(got))
	}
	if got.AssigneeID == nil || *got.AssigneeID != e.dan.ID {
		t.Errorf("assignee = %v; a task assigned here keeps its assignee when the issue has none", got.AssigneeID)
	}
	issue, _ := e.fake.Issue(repo, n)
	if !eq(issue.Labels, []string{"bug", "ux"}) {
		t.Errorf("issue labels = %v; the label of the task goes to the issue", issue.Labels)
	}
	if other, _ := e.tasks.GetByID(dup.ID.String()); other.Name != "Duplicada" {
		t.Errorf("the second linked task was touched: %+v", other)
	}
}

// Excluir a tarefa descarta a issue (ela não volta); ligar outra tarefa a ela à mão a traz de volta.
func TestSync_DeletedTaskIsATombstone(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Vai embora"})
	e.sync(issuesync.Full)
	tk := e.taskFor(n)

	must(t, e.taskSvc.Delete(tk.ID.String()))
	if row := e.row(n); row == nil || row.TaskID != nil {
		t.Fatalf("row after deleting the task = %+v, want it kept with no task", row)
	}
	for _, mode := range []issuesync.Mode{issuesync.Full, issuesync.Incremental} {
		e.fake.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.Body = "mexeram" })
		if sum := e.sync(mode); sum.Created != 0 || sum.Errors != 0 {
			t.Errorf("summary = %+v; a discarded issue must not come back", sum)
		}
	}
	if e.tasksCount() != 0 {
		t.Errorf("%d tasks, the discarded issue came back", e.tasksCount())
	}

	// Ligar uma tarefa a ela à mão a traz de volta.
	back, _ := e.taskSvc.CreateAs("", e.project, "De volta", "", "", nil, task.Attrs{})
	e.taskSvc.LinkExternalItem(back.ID.String(), e.it.ID.String(), "1", "https://github.com/owner/repo/issues/1")
	e.sync(issuesync.Full)
	if row := e.row(n); row.TaskID == nil || *row.TaskID != back.ID {
		t.Errorf("row = %+v, want it bound to the task linked by hand", row)
	}
	if got := e.taskFor(n); got.Name != "Vai embora" {
		t.Errorf("name = %q, want the issue's", got.Name)
	}
}

// Desvincular a tarefa da issue tira a tarefa da sincronização.
func TestSync_UnlinkedTaskLeavesTheSync(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Ligada"})
	e.sync(issuesync.Full)
	tk := e.taskFor(n)
	_, err := e.taskSvc.UnlinkExternalItem(tk.ID.String())
	must(t, err)

	e.fake.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.Title = "Mudou lá" })
	name := "Mudou aqui"
	e.taskSvc.UpdateAs("", tk.ID.String(), name, "", nil, nil, task.Attrs{})
	e.fake.Reset()
	sum := e.sync(issuesync.Full)
	if sum.Created != 0 || sum.Updated != 0 || sum.Pushed != 0 {
		t.Errorf("summary = %+v; an unlinked task is out of the sync", sum)
	}
	if got, _ := e.tasks.GetByID(tk.ID.String()); got.Name != name {
		t.Errorf("name = %q; the issue must not rename an unlinked task", got.Name)
	}
	if row := e.row(n); row == nil || row.TaskID != nil {
		t.Errorf("row = %+v, want a tombstone", row)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to GitHub: %v", e.fake.Requests())
	}
}

// A issue apagada ou transferida vira "sumida"; a tarefa fica, e a rodada não pergunta de novo por ela.
func TestSync_GoneIssues(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	deleted := e.add(testutil.GitHubIssue{Title: "Apagada"})
	moved := e.add(testutil.GitHubIssue{Title: "Transferida"})
	e.sync(issuesync.Full)

	e.fake.DeleteIssue(repo, deleted)
	e.fake.MoveIssue(repo, moved)
	e.fake.Reset()
	sum := e.sync(issuesync.Full)
	if sum.Errors != 0 || sum.Partial {
		t.Errorf("summary = %+v", sum)
	}
	for _, n := range []int{deleted, moved} {
		if row := e.row(n); row.State != "gone" || row.TaskID == nil {
			t.Errorf("row #%d = %+v, want gone with its task", n, row)
		}
		if e.taskFor(n).Status != "backlog" {
			t.Errorf("the task of #%d must stay as it was", n)
		}
	}
	e.fake.Reset()
	e.sync(issuesync.Full)
	if n := e.fake.Count("GET", "/repos/owner/repo/issues/"); n != 0 {
		t.Errorf("a gone issue was fetched again %d times", n)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to GitHub: %v", e.fake.Requests())
	}
}

// Uma issue fechada, que a lista de abertas não traz, é achada pela conferência das ligadas.
func TestSync_FullRoundFindsIssuesClosedMeanwhile(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Vai fechar"})
	e.sync(issuesync.Full)
	e.fake.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.State = "closed" })

	sum := e.sync(issuesync.Full)
	if sum.Updated != 1 || sum.Closed != 1 {
		t.Errorf("summary = %+v", sum)
	}
	if e.taskFor(n).Status != "closed" || e.row(n).State != "closed" {
		t.Errorf("task %q, row %q", e.taskFor(n).Status, e.row(n).State)
	}
}

// A rodada incremental pede só o que mudou desde o cursor, abertas e fechadas.
func TestSync_IncrementalAsksForWhatChanged(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	e.add(testutil.GitHubIssue{Title: "Uma"})
	e.sync(issuesync.Full)
	e.fake.Reset()
	e.sync(issuesync.Incremental)

	var list *testutil.GitHubRequest
	for _, r := range e.fake.Requests() {
		if r.Path == "/repos/owner/repo/issues" {
			list = &r
		}
	}
	if list == nil || !strings.Contains(list.Query, "since=") || !strings.Contains(list.Query, "state=all") || !strings.Contains(list.Query, "sort=updated") {
		t.Errorf("list request = %+v", list)
	}
}

// Sem permissão de escrita, as issues só vêm para cá: nada é empurrado, e o cartão avisa.
func TestSync_ReadOnlyRepository(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Só leitura"})
	e.fake.SetPush(repo, false)

	if sum := e.sync(issuesync.Full); sum.Created != 1 || sum.Errors != 0 {
		t.Errorf("summary = %+v; reading still works", sum)
	}
	tk := e.taskFor(n)
	e.taskSvc.UpdateAs("", tk.ID.String(), "Mudei aqui", "", nil, nil, task.Attrs{})
	closed := "closed"
	e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &closed})
	e.add(testutil.GitHubIssue{Title: "Outra"})
	e.fake.Reset()

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 0 || sum.Created != 1 {
		t.Errorf("summary = %+v", sum)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("a read-only repository was written to: %v", e.fake.Requests())
	}
	if it := e.integration(); it.LastSyncError != "issue_sync.read_only" {
		t.Errorf("last error = %q, want issue_sync.read_only", it.LastSyncError)
	}
	if got := e.taskFor(n); got.Name != "Mudei aqui" || got.Status != "closed" {
		t.Errorf("the local edit was lost: %+v", got)
	}
	// E o repositório arquivado também é só leitura.
	e.fake.SetPush(repo, true)
	e.fake.SetArchived(repo, true)
	e.sync(issuesync.Full)
	if e.integration().LastSyncError != "issue_sync.read_only" || e.fake.Writes() != 0 {
		t.Errorf("an archived repository: error %q, writes %d", e.integration().LastSyncError, e.fake.Writes())
	}
}

// O GitHub que responde 200 sem gravar a mudança (token sem permissão para etiquetas, por exemplo) é
// detectado, e a mesma mudança não é repetida a cada rodada.
func TestSync_SilentDiscardDoesNotLoop(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Etiquetas descartadas"})
	e.sync(issuesync.Full)
	e.fake.Discard(repo, "labels")

	tk := e.taskFor(n)
	l, _ := e.taskSvc.CreateLabel(e.project, "ux")
	ids := []string{l.ID.String()}
	e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", nil, nil, task.Attrs{LabelIDs: &ids})
	e.fake.Reset()

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 1 {
		t.Errorf("summary = %+v", sum)
	}
	row := e.row(n)
	if row.StuckSig == "" || row.LastError != "issue_sync.push_discarded" {
		t.Errorf("row = %+v, want the discard recorded", row)
	}
	if e.integration().LastSyncError != "issue_sync.push_discarded" {
		t.Errorf("integration error = %q", e.integration().LastSyncError)
	}
	if got := e.taskFor(n); !eq(labelNames(got), []string{"ux"}) {
		t.Errorf("the local label must stay: %v", labelNames(got))
	}

	// As rodadas seguintes não insistem.
	e.fake.Reset()
	for i := 0; i < 3; i++ {
		e.sync(issuesync.Full)
	}
	if n := e.fake.Count("PATCH", "/repos"); n != 0 {
		t.Errorf("%d more PATCH requests for a change GitHub already discarded", n)
	}
	if e.integration().LastSyncError != "issue_sync.push_discarded" {
		t.Errorf("the warning must stay while the change is stuck, got %q", e.integration().LastSyncError)
	}

	// Outra mudança é outra tentativa.
	e.taskSvc.UpdateAs("", tk.ID.String(), "Novo nome", "", nil, nil, task.Attrs{LabelIDs: &ids})
	e.fake.Reset()
	e.sync(issuesync.Full)
	if n := e.fake.Count("PATCH", "/repos"); n != 1 {
		t.Errorf("%d PATCH requests, want one for the new change", n)
	}
}

// O responsável que não tem e-mail público fica na issue, e o que se escolhe aqui é achado pelo e-mail.
func TestSync_AssigneeWithoutPublicEmailIsKept(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Dois responsáveis", Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	tk := e.taskFor(n)
	if tk.AssigneeID != nil {
		t.Fatalf("assignee = %v", tk.AssigneeID)
	}

	ana := e.ana.ID.String()
	e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", &ana, nil, task.Attrs{})
	e.sync(issuesync.Full)
	issue, _ := e.fake.Issue(repo, n)
	if !eq(issue.Assignees, []string{"ghost", "ana-dev"}) {
		t.Errorf("assignees = %v, want the fantasma kept and Ana added", issue.Assignees)
	}
	if e.integration().SyncUnmatched != 0 {
		t.Errorf("unmatched = %d; the issue has a linked assignee now", e.integration().SyncUnmatched)
	}

	// Tirar o responsável aqui tira só o que estava ligado.
	none := ""
	e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", &none, nil, task.Attrs{})
	e.sync(issuesync.Full)
	if issue, _ = e.fake.Issue(repo, n); !eq(issue.Assignees, []string{"ghost"}) {
		t.Errorf("assignees = %v, want only the fantasma", issue.Assignees)
	}
	if e.taskFor(n).AssigneeID != nil {
		t.Error("the task must stay unassigned")
	}
}

// Escolher aqui alguém sem usuário do GitHub que se ache pelo e-mail não manda nada e deixa o aviso.
func TestSync_NoGitHubLoginForThePerson(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Sem login"})
	e.sync(issuesync.Full)
	tk := e.taskFor(n)

	dan := e.dan.ID.String()
	e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", &dan, nil, task.Attrs{})
	e.fake.Reset()
	sum := e.sync(issuesync.Full)
	if sum.Pushed != 0 || sum.Errors != 0 {
		t.Errorf("summary = %+v", sum)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to GitHub: %v", e.fake.Requests())
	}
	if e.integration().LastSyncError != "issue_sync.no_login" || e.row(n).LastError != "issue_sync.no_login" {
		t.Errorf("errors: integration %q, row %q", e.integration().LastSyncError, e.row(n).LastError)
	}
	if got := e.taskFor(n); got.AssigneeID == nil || *got.AssigneeID != e.dan.ID {
		t.Errorf("the local choice must stay: %v", got.AssigneeID)
	}

	// Perguntar de novo a cada rodada não custa buscas ao GitHub: a resposta "não achei" fica guardada.
	e.fake.Reset()
	e.sync(issuesync.Full)
	if searches := e.fake.Count("GET", "/search/users"); searches != 0 {
		t.Errorf("%d searches in the next round; the miss must be remembered", searches)
	}
}

// Quem publica o e-mail no GitHub depois passa a ser reconhecido, sem ninguém mexer na issue; o botão
// pergunta tudo de novo.
func TestSync_AssigneeWhoPublishesTheEmailLater(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Do fantasma", Assignees: []string{"ghost"}})
	e.sync(issuesync.Full)
	if e.taskFor(n).AssigneeID != nil {
		t.Fatal("ghost has no public email")
	}

	e.fake.AddUser("ghost", "bia@test.com") // agora publica
	e.sync(issuesync.Full)
	if e.taskFor(n).AssigneeID != nil {
		t.Error("a background round within the cache time must not ask again")
	}
	sum, err := e.syncer.SyncNow(context.Background(), e.it.ID)
	must(t, err)
	if got := e.taskFor(n); got.AssigneeID == nil || *got.AssigneeID != e.bia.ID || sum.Updated != 1 {
		t.Errorf("after Sincronizar agora: assignee %v, summary %+v", got.AssigneeID, sum)
	}
	if e.integration().SyncUnmatched != 0 {
		t.Errorf("unmatched = %d", e.integration().SyncUnmatched)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("wrote to GitHub: %v", e.fake.Requests())
	}
}

// A rodada interrompida não avança o cursor, mas faz o que deu, e a seguinte termina.
func TestSync_InterruptedRoundDoesNotMoveTheCursor(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	for n := 0; n < 3; n++ {
		e.add(testutil.GitHubIssue{Title: "Issue"})
	}

	e.fake.Fail("GET", "/repos/owner/repo/issues", http.StatusBadGateway, 1)
	sum, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full)
	if err != nil {
		t.Fatalf("a failed listing is not a fatal error of the round: %v", err)
	}
	if !sum.Partial || sum.Errors != 1 || sum.Created != 0 {
		t.Errorf("summary = %+v", sum)
	}
	row := testClient.Integration.GetX(context.Background(), e.it.ID)
	if row.SyncCursor != nil {
		t.Errorf("cursor = %v after a round that did not finish", row.SyncCursor)
	}
	if got := e.integration().LastSyncError; got != "integration.provider_status" {
		t.Errorf("last error = %q", got)
	}

	if sum := e.sync(issuesync.Full); sum.Partial || sum.Created != 3 {
		t.Errorf("next round = %+v", sum)
	}
	if testClient.Integration.GetX(context.Background(), e.it.ID).SyncCursor == nil {
		t.Error("the finished round must set the cursor")
	}
	if e.integration().LastSyncError != "" {
		t.Errorf("the error stayed after a good round: %q", e.integration().LastSyncError)
	}
}

// O limite de requisições para a rodada e é registrado; a espera da rotina de fundo o respeita.
func TestSync_RateLimit(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	for n := 0; n < 250; n++ {
		e.add(testutil.GitHubIssue{Title: "Issue"})
	}
	e.fake.Remaining(3)
	sum, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full)
	if err == nil || !errors.Is(err, adapter.ErrRateLimited) {
		t.Fatalf("err = %v, want the rate limit", err)
	}
	if !sum.Partial || sum.Created != 100 {
		t.Errorf("summary = %+v, want the first page done and the round partial", sum)
	}
	if e.integration().LastSyncError != "integration.rate_limited" {
		t.Errorf("last error = %q", e.integration().LastSyncError)
	}
	if testClient.Integration.GetX(context.Background(), e.it.ID).SyncCursor != nil {
		t.Error("a limited round must not set the cursor")
	}

	e.fake.Remaining(-1)
	if sum := e.sync(issuesync.Full); sum.Partial || sum.Created != 150 {
		t.Errorf("next round = %+v, want the rest", sum)
	}
}

// Um token recusado para a rodada logo no começo e deixa o motivo na integração.
func TestSync_InvalidToken(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	e.add(testutil.GitHubIssue{Title: "Uma"})
	if _, err := e.integ.Update(e.it.ID.String(), "", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	// Troca o token guardado por um que o GitHub rejeita, direto no banco.
	sealed, _ := adapter.EncryptConfig(map[string]any{"token": testutil.InvalidToken}, "test-32-byte-encryption-key!!!!")
	testClient.Integration.UpdateOneID(e.it.ID).SetCredentials(sealed).ExecX(context.Background())

	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full); !errors.Is(err, adapter.ErrInvalidToken) {
		t.Errorf("err = %v, want invalid token", err)
	}
	if e.integration().LastSyncError != "integration.invalid_token" {
		t.Errorf("last error = %q", e.integration().LastSyncError)
	}
	if e.tasksCount() != 0 {
		t.Error("nothing may be imported with a rejected token")
	}
}

// Ligada ou desligada: só a integração ativa com a sincronização ligada roda.
func TestSync_OffAndDisabled(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	e.add(testutil.GitHubIssue{Title: "Uma"})
	no, yes := false, true

	e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &no})
	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full); !errors.Is(err, issuesync.ErrSyncOff) {
		t.Errorf("sync off: err = %v", err)
	}
	e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &yes})
	e.integ.Edit(e.it.ID.String(), integration.EditInput{Enabled: &no})
	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full); !errors.Is(err, issuesync.ErrSyncOff) {
		t.Errorf("disabled: err = %v", err)
	}
	if e.tasksCount() != 0 || len(e.fake.Requests()) != 0 {
		t.Errorf("a round that is off touched things: %d tasks, %d requests", e.tasksCount(), len(e.fake.Requests()))
	}
}

// O teto de issues mudadas por rodada deixa o resto para a seguinte.
func TestSync_PushCapPerRound(t *testing.T) {
	e := newEnv(t, issuesync.Config{MaxPushes: 2})
	var numbers []int
	for i := 0; i < 5; i++ {
		numbers = append(numbers, e.add(testutil.GitHubIssue{Title: "Issue"}))
	}
	e.sync(issuesync.Full)
	for _, n := range numbers {
		e.taskSvc.UpdateAs("", e.taskFor(n).ID.String(), "Renomeada", "", nil, nil, task.Attrs{})
	}

	sum := e.sync(issuesync.Full)
	if sum.Pushed != 2 || !sum.Partial {
		t.Errorf("summary = %+v, want 2 pushed and the round partial", sum)
	}
	e.sync(issuesync.Full)
	e.sync(issuesync.Full)
	for _, n := range numbers {
		if issue, _ := e.fake.Issue(repo, n); issue.Title != "Renomeada" {
			t.Errorf("issue #%d title = %q after the rounds", n, issue.Title)
		}
	}
}

// Uma etiqueta que o token não pode criar fica registrada, e o descarte também.
func TestSync_LabelCreationRefused(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Etiqueta"})
	e.sync(issuesync.Full)
	e.fake.LabelCreateStatus(repo, http.StatusForbidden)
	tk := e.taskFor(n)
	l, _ := e.taskSvc.CreateLabel(e.project, "nova")
	ids := []string{l.ID.String()}
	e.taskSvc.UpdateAs("", tk.ID.String(), tk.Name, "", nil, nil, task.Attrs{LabelIDs: &ids})

	e.sync(issuesync.Full)
	if issue, _ := e.fake.Issue(repo, n); len(issue.Labels) != 0 {
		t.Errorf("labels = %v; GitHub drops a label it does not have", issue.Labels)
	}
	if got := e.integration().LastSyncError; got != "issue_sync.push_discarded" {
		t.Errorf("last error = %q", got)
	}
}

// Só uma rodada de cada vez por integração; a do botão avisa em vez de esperar.
func TestSync_OneRoundAtATime(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var once sync.Once
	e := newEnv(t, issuesync.Config{})
	e.add(testutil.GitHubIssue{Title: "Uma"})
	// Um GitHub que segura a listagem até o teste soltar.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/issues" {
			once.Do(func() { started <- struct{}{} })
			<-gate
		}
		e.fake.ServeHTTP(w, r)
	}))
	defer slow.Close()
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: slow.URL} })

	done := make(chan error, 1)
	go func() { _, err := e.syncer.SyncNow(context.Background(), e.it.ID); done <- err }()
	<-started

	if _, err := e.syncer.SyncNow(context.Background(), e.it.ID); !errors.Is(err, issuesync.ErrSyncRunning) {
		t.Errorf("second SyncNow: err = %v, want sync_running", err)
	}
	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Incremental); !errors.Is(err, issuesync.ErrSyncRunning) {
		t.Errorf("background round: err = %v, want sync_running", err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Errorf("the first round: %v", err)
	}
	if e.tasksCount() != 1 {
		t.Errorf("%d tasks, want 1", e.tasksCount())
	}
}

// O gancho: a tarefa que mudou aqui empurra a issue dela, sem esperar a rodada.
func TestSync_HookPushesATaskThatChanged(t *testing.T) {
	e := newEnv(t, issuesync.Config{})
	n := e.add(testutil.GitHubIssue{Title: "Original"})
	other := e.add(testutil.GitHubIssue{Title: "Outra"})
	e.sync(issuesync.Full)
	e.tasks.SetChangeHook(e.syncer.Notify)
	tk := e.taskFor(n)
	free, _ := e.taskSvc.CreateAs("", e.project, "Sem issue", "", "", nil, task.Attrs{})
	e.fake.Reset()

	e.taskSvc.UpdateAs("", tk.ID.String(), "Renomeada aqui", "", nil, nil, task.Attrs{})
	// Pegar uma tarefa livre (o que bater o ponto faz) vai para o responsável da issue.
	if _, err := e.taskSvc.Claim(e.ana.ID.String(), e.taskFor(other).ID.String()); err != nil {
		t.Fatalf("claim: %v", err)
	}
	// Mudanças em tarefas sem issue não custam nada.
	e.taskSvc.UpdateAs("", free.ID.String(), "Sem issue 2", "", nil, nil, task.Attrs{})
	if e.syncer.Pending() != 3 {
		t.Fatalf("pending = %d, want the three tasks that changed", e.syncer.Pending())
	}

	if touched := e.syncer.Flush(context.Background()); touched != 2 {
		t.Errorf("flush touched %d issues, want 2 (the third task has none)", touched)
	}
	if e.syncer.Pending() != 0 {
		t.Errorf("pending = %d after the flush", e.syncer.Pending())
	}
	if issue, _ := e.fake.Issue(repo, n); issue.Title != "Renomeada aqui" {
		t.Errorf("title on GitHub = %q", issue.Title)
	}
	if issue, _ := e.fake.Issue(repo, other); !eq(issue.Assignees, []string{"ana-dev"}) {
		t.Errorf("assignees on GitHub = %v", issue.Assignees)
	}
	if n := e.fake.Count("PATCH", "/repos"); n != 2 {
		t.Errorf("%d PATCH requests, want one per task", n)
	}

	// O que a sincronização grava não volta a ela como mudança.
	e.sync(issuesync.Full)
	e.fake.EditIssue(repo, n, func(i *testutil.GitHubIssue) { i.Title = "Do GitHub" })
	e.sync(issuesync.Incremental)
	if e.syncer.Pending() != 0 {
		t.Errorf("a round queued %d tasks through the hook", e.syncer.Pending())
	}
}

// A rotina para quando o contexto acaba, e o worker do gancho empurra sozinho.
func TestSync_RunStopsWithTheContext(t *testing.T) {
	e := newEnv(t, issuesync.Config{Debounce: 20 * time.Millisecond})
	n := e.add(testutil.GitHubIssue{Title: "Original"})
	e.sync(issuesync.Full)
	e.tasks.SetChangeHook(e.syncer.Notify)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.syncer.Run(ctx); close(stopped) }()

	e.taskSvc.UpdateAs("", e.taskFor(n).ID.String(), "Pelo worker", "", nil, nil, task.Attrs{})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if issue, _ := e.fake.Issue(repo, n); issue.Title == "Pelo worker" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the worker did not push the change")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

// Com o intervalo ligado, a rotina de fundo faz a primeira rodada sozinha e para com o contexto.
func TestSync_RunPollsOnItsOwn(t *testing.T) {
	e := newEnv(t, issuesync.Config{Interval: 50 * time.Millisecond})
	e.add(testutil.GitHubIssue{Title: "Chega sozinha"})

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.syncer.Run(ctx); close(stopped) }()
	defer func() {
		cancel()
		<-stopped
	}()

	deadline := time.Now().Add(20 * time.Second)
	for e.tasksCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the background routine did not import the issue")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
