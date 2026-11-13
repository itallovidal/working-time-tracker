package issuesync_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/issuesync"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/personaccount"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

const glPath = "group/project"

// glenv é um projeto com uma integração do GitLab sincronizando com um GitLab fake, o mesmo núcleo do GitHub
// (env). Ana e Bia estão no projeto e têm usuário no GitLab com e-mail público; "ghost" é um usuário sem
// e-mail público. O projeto começa vazio, e a conta é Owner dele.
type glenv struct {
	t       *testing.T
	fake    *testutil.GitLab
	srv     *httptest.Server
	integ   *integration.Service
	tasks   *task.Store
	taskSvc *task.Service
	rows    *issuesync.Store
	syncer  *issuesync.Syncer
	it      *integration.Integration
	project string

	ana, bia *person.Person
}

func newGitLabEnv(t *testing.T) *glenv {
	t.Helper()
	testutil.Truncate(t, testDB)

	e := &glenv{t: t, fake: testutil.NewGitLab()}
	e.fake.AddProject(glPath) // vazio: sem a issue 7 de sempre
	for username, email := range map[string]string{"ana-lab": "ana@test.com", "bia-lab": "bia@test.com", "ghost": ""} {
		e.fake.AddUser(username, email)
	}
	e.srv = httptest.NewServer(e.fake)
	t.Cleanup(e.srv.Close)
	adapter.Register("gitlab", func() adapter.Integration { return &adapter.GitLabIntegration{BaseURL: e.srv.URL + "/api/v4"} })

	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	must(t, err)
	proj, err := project.NewService(project.NewStore(testClient)).Create(org.ID.String(), "Projeto", "", 0, project.Routine{})
	must(t, err)
	e.project = proj.ID.String()

	people := person.NewService(person.NewStore(testClient))
	mk := func(name, email string) *person.Person {
		p, err := people.Create(org.ID.String(), name, email)
		must(t, err)
		_, err = allocation.NewService(allocation.NewStore(testClient)).Set(e.project, p.ID.String(), 1000)
		must(t, err)
		return p
	}
	e.ana, e.bia = mk("Ana", "ana@test.com"), mk("Bia", "bia@test.com")

	e.integ = integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")
	e.it, err = e.integ.Create(e.project, "gitlab", "GitLab", "glpat-test", map[string]any{"project_url": glPath}, true)
	must(t, err)
	yes := true
	e.it, err = e.integ.Edit(e.it.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)

	e.tasks = task.NewStore(testClient)
	e.taskSvc = task.NewService(e.tasks, team.NewMembershipStore(testClient), e.integ)
	e.rows = issuesync.NewStore(testClient)
	e.syncer = issuesync.New(issuesync.Deps{
		Integrations: e.integ, Tasks: e.tasks, People: person.NewStore(testClient),
		Members: team.NewMembershipStore(testClient), Rows: e.rows, Accounts: personaccount.NewStore(testClient),
	}, issuesync.Config{})
	e.taskSvc.SetRemover(e.syncer)
	e.fake.Reset()
	return e
}

func (e *glenv) sync(mode issuesync.Mode) *issuesync.Summary {
	e.t.Helper()
	sum, err := e.syncer.Sync(context.Background(), e.it.ID, mode)
	if err != nil {
		e.t.Fatalf("sync: %v", err)
	}
	return sum
}

func (e *glenv) add(i testutil.GitLabIssue) int { return e.fake.AddIssue(glPath, i) }

func (e *glenv) row(iid int) *issuesync.Row {
	e.t.Helper()
	rows, err := e.rows.ByIntegration(e.it.ID)
	must(e.t, err)
	return rows[strconv.Itoa(iid)]
}

func (e *glenv) taskFor(iid int) *task.Task {
	e.t.Helper()
	row := e.row(iid)
	if row == nil || row.TaskID == nil {
		e.t.Fatalf("issue #%d has no task (row %+v)", iid, row)
	}
	tk, err := e.tasks.GetByID(row.TaskID.String())
	must(e.t, err)
	return tk
}

func (e *glenv) lastError() string {
	it, err := e.integ.Get(e.it.ID.String())
	must(e.t, err)
	return it.LastSyncError
}

// A primeira rodada traz as issues abertas (e só as issues: o incidente fica de fora), liga o responsável pelo
// e-mail público e não escreve nada no GitLab.
func TestGitLab_ImportsOpenIssues(t *testing.T) {
	e := newGitLabEnv(t)
	one := e.add(testutil.GitLabIssue{Title: "Corrigir login", Description: "Passos:\r\n1. abrir", Labels: []string{"bug", "UX"}, Assignees: []string{"ana-lab"}})
	ghost := e.add(testutil.GitLabIssue{Title: "Do fantasma", Assignees: []string{"ghost"}})
	closed := e.add(testutil.GitLabIssue{Title: "Antiga", State: "closed"})
	incident := e.add(testutil.GitLabIssue{Title: "Um incidente", Type: "incident"})

	sum := e.sync(issuesync.Full)
	if sum.Created != 2 || sum.Pushed != 0 || sum.Errors != 0 || sum.Partial || sum.Unmapped != 1 {
		t.Errorf("summary = %+v, want 2 created and 1 unmapped", sum)
	}
	if e.fake.Writes() != 0 {
		t.Errorf("importing wrote to GitLab: %v", e.fake.Requests())
	}
	tk := e.taskFor(one)
	if tk.Name != "Corrigir login" || tk.Description != "Passos:\n1. abrir" || tk.Status != "backlog" || !eq(labelNames(tk), []string{"UX", "bug"}) {
		t.Errorf("task = %+v, labels %v", tk, labelNames(tk))
	}
	if tk.AssigneeID == nil || *tk.AssigneeID != e.ana.ID {
		t.Errorf("assignee = %v, want Ana (her email is public on GitLab)", tk.AssigneeID)
	}
	if len(tk.Links) != 1 || tk.Links[0].ItemID != "1" || tk.Links[0].URL != "https://gitlab.com/group/project/-/issues/1" {
		t.Errorf("links = %+v", tk.Links)
	}
	if e.taskFor(ghost).AssigneeID != nil {
		t.Error("an assignee with no public email leaves the task available")
	}
	for _, n := range []int{closed, incident} {
		if e.row(n) != nil {
			t.Errorf("issue #%d must not be imported", n)
		}
	}
	if again := e.sync(issuesync.Full); *again != (issuesync.Summary{Unmapped: 1}) || e.fake.Writes() != 0 {
		t.Errorf("second round = %+v, writes %d; want nothing done", again, e.fake.Writes())
	}
}

// O que muda no GitLab chega à tarefa pela rodada incremental (updated_after); fechar e reabrir valem.
func TestGitLab_ChangesComeIn(t *testing.T) {
	e := newGitLabEnv(t)
	one := e.add(testutil.GitLabIssue{Title: "Corrigir login", Labels: []string{"bug"}, Assignees: []string{"ana-lab"}})
	two := e.add(testutil.GitLabIssue{Title: "Outra"})
	e.sync(issuesync.Full)

	e.fake.EditIssue(glPath, one, func(i *testutil.GitLabIssue) {
		i.Title = "Corrigir o login"
		i.Labels = []string{"bug", "api"}
		i.Assignees = []string{"bia-lab"}
	})
	e.fake.EditIssue(glPath, two, func(i *testutil.GitLabIssue) { i.State = "closed" })
	e.fake.Reset()

	sum := e.sync(issuesync.Incremental)
	if sum.Updated != 2 || sum.Closed != 1 || sum.Pushed != 0 {
		t.Errorf("summary = %+v", sum)
	}
	if q := e.fake.Requests(); len(q) == 0 || e.fake.Count("GET", "/api/v4/projects/group%2Fproject/issues") == 0 {
		t.Fatalf("requests = %+v", q)
	}
	tk := e.taskFor(one)
	if tk.Name != "Corrigir o login" || !eq(labelNames(tk), []string{"api", "bug"}) || tk.AssigneeID == nil || *tk.AssigneeID != e.bia.ID {
		t.Errorf("task after the GitLab edit = %q, %v, %v", tk.Name, labelNames(tk), tk.AssigneeID)
	}
	if e.taskFor(two).Status != "closed" {
		t.Errorf("task of the closed issue = %q, want closed", e.taskFor(two).Status)
	}

	e.fake.EditIssue(glPath, two, func(i *testutil.GitLabIssue) { i.State = "opened" })
	e.sync(issuesync.Incremental)
	if got := e.taskFor(two).Status; got != "backlog" {
		t.Errorf("reopened task = %q, want backlog", got)
	}
}

// O que muda aqui chega ao GitLab: o nome, a descrição, a etiqueta que o projeto não tinha (criada antes), o
// responsável pelo id e o fechamento por state_event. A rodada seguinte não repete.
func TestGitLab_LocalChangesGoOut(t *testing.T) {
	e := newGitLabEnv(t)
	one := e.add(testutil.GitLabIssue{Title: "Corrigir login", Description: "texto", Labels: []string{"bug"}})
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

	if sum := e.sync(issuesync.Full); sum.Pushed != 1 || sum.Errors != 0 {
		t.Errorf("summary = %+v, want one issue pushed", sum)
	}
	issue, _ := e.fake.Issue(glPath, one)
	if issue.Title != "Corrigir o login (daqui)" || issue.Description != "texto daqui" || issue.State != "closed" ||
		!eq(issue.Labels, []string{"bug", "Nova etiqueta"}) || !eq(issue.Assignees, []string{"bia-lab"}) {
		t.Errorf("issue after the push = %+v", issue)
	}
	if n := e.fake.Count("POST", "/api/v4/projects/group%2Fproject/labels"); n != 1 {
		t.Errorf("%d label creations, want the missing one only", n)
	}
	if n := e.fake.Count("PUT", "/api/v4/projects/group%2Fproject/issues/"); n != 1 {
		t.Errorf("%d PUT requests, want exactly one", n)
	}

	e.fake.Reset()
	for _, mode := range []issuesync.Mode{issuesync.Full, issuesync.Incremental, issuesync.Full} {
		if sum := e.sync(mode); sum.Pushed != 0 || sum.Updated != 0 || sum.Errors != 0 {
			t.Errorf("round %v after the push = %+v", mode, sum)
		}
	}
	if e.fake.Writes() != 0 {
		t.Errorf("rounds with no difference wrote to GitLab: %v", e.fake.Requests())
	}

	backlog := "backlog"
	if _, err := e.taskSvc.UpdateAttrs(tk.ID.String(), task.Attrs{Status: &backlog}); err != nil {
		t.Fatal(err)
	}
	if sum := e.sync(issuesync.Full); sum.Pushed != 1 {
		t.Errorf("reopen: %+v", sum)
	}
	if issue, _ = e.fake.Issue(glPath, one); issue.State != "opened" {
		t.Errorf("issue state = %q, want opened", issue.State)
	}
}

// Quem só é Guest no projeto lê: as issues vêm para cá, nada é empurrado, e o cartão avisa. O projeto
// arquivado também é só leitura.
func TestGitLab_ReadOnlyProject(t *testing.T) {
	e := newGitLabEnv(t)
	n := e.add(testutil.GitLabIssue{Title: "Só leitura"})
	e.fake.SetAccess(glPath, testutil.GitLabGuest)
	e.sync(issuesync.Full)
	if _, err := e.taskSvc.UpdateAs("", e.taskFor(n).ID.String(), "Mudei aqui", "", nil, nil, task.Attrs{}); err != nil {
		t.Fatal(err)
	}
	e.fake.Reset()

	if sum := e.sync(issuesync.Full); sum.Pushed != 0 || e.fake.Writes() != 0 {
		t.Errorf("summary = %+v, writes %v", sum, e.fake.Requests())
	}
	if got := e.lastError(); got != "issue_sync.read_only" {
		t.Errorf("last error = %q, want issue_sync.read_only", got)
	}
	e.fake.SetAccess(glPath, testutil.GitLabOwner)
	e.fake.SetArchived(glPath, true)
	e.sync(issuesync.Full)
	if e.lastError() != "issue_sync.read_only" || e.fake.Writes() != 0 {
		t.Errorf("an archived project: error %q, writes %d", e.lastError(), e.fake.Writes())
	}
}

// A issue apagada e a transferida para outro projeto ficam "gone", com a tarefa como estava.
func TestGitLab_GoneIssues(t *testing.T) {
	e := newGitLabEnv(t)
	deleted := e.add(testutil.GitLabIssue{Title: "Apagada"})
	moved := e.add(testutil.GitLabIssue{Title: "Transferida"})
	e.sync(issuesync.Full)

	e.fake.DeleteIssue(glPath, deleted)
	e.fake.MoveIssue(glPath, moved)
	if sum := e.sync(issuesync.Full); sum.Errors != 0 || sum.Closed != 0 {
		t.Errorf("summary = %+v; a moved issue is gone, not closed", sum)
	}
	for _, n := range []int{deleted, moved} {
		if row := e.row(n); row.State != "gone" || row.TaskID == nil {
			t.Errorf("row #%d = %+v, want gone with its task", n, row)
		}
		if e.taskFor(n).Status != "backlog" {
			t.Errorf("the task of #%d must stay as it was", n)
		}
	}
}

// O token recusado e o projeto que sumiu param a rodada inteira, com o código do motivo no cartão.
func TestGitLab_StopsTheRound(t *testing.T) {
	e := newGitLabEnv(t)
	e.add(testutil.GitLabIssue{Title: "Uma"})
	e.fake.Fail("GET", "/api/v4/projects/group%2Fproject", 401, 1)
	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full); !errors.Is(err, adapter.ErrInvalidToken) {
		t.Errorf("invalid token: err = %v", err)
	}
	if got := e.lastError(); got != "integration.invalid_token" {
		t.Errorf("last error = %q", got)
	}

	e.fake.Fail("GET", "/api/v4/projects/group%2Fproject", 404, 1)
	if _, err := e.syncer.Sync(context.Background(), e.it.ID, issuesync.Full); !errors.Is(err, adapter.ErrGitLabProjectMissing) {
		t.Errorf("missing project: err = %v", err)
	}
	if got := e.lastError(); got != "integration.gitlab_project_not_found" {
		t.Errorf("last error = %q", got)
	}
}

// Publicar cria a issue com o que a tarefa tem; excluir a tarefa apaga a issue (Owner) ou a fecha (quem não é
// Owner), com o aviso de que ela continua lá.
func TestGitLab_PublishAndRemove(t *testing.T) {
	e := newGitLabEnv(t)
	ana := e.ana.ID.String()
	bug, err := e.taskSvc.CreateLabel(e.project, "bug")
	must(t, err)
	tk, err := e.taskSvc.CreateAs("", e.project, "Nova daqui", "descrição", ana, nil, task.Attrs{LabelIDs: &[]string{bug.ID.String()}})
	must(t, err)

	if _, err := e.syncer.Publish(context.Background(), tk.ID, e.it.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	issue, ok := e.fake.Issue(glPath, 1)
	if !ok || issue.Title != "Nova daqui" || issue.Description != "descrição" || !eq(issue.Labels, []string{"bug"}) || !eq(issue.Assignees, []string{"ana-lab"}) {
		t.Errorf("published issue = %+v", issue)
	}
	res, err := e.taskSvc.Delete(context.Background(), tk.ID.String(), []uuid.UUID{e.it.ID})
	must(t, err)
	if len(res) != 1 || res[0].Outcome != "deleted" || res[0].Problem != nil || res[0].Provider != "gitlab" {
		t.Errorf("remove as owner = %+v", res)
	}
	if issue, _ = e.fake.Issue(glPath, 1); !issue.Deleted {
		t.Error("the owner's issue must be deleted")
	}

	e.fake.SetAccess(glPath, testutil.GitLabDeveloper)
	other, err := e.taskSvc.CreateAs("", e.project, "Outra", "", "", nil, task.Attrs{})
	must(t, err)
	if _, err := e.syncer.Publish(context.Background(), other.ID, e.it.ID); err != nil {
		t.Fatalf("publish as developer: %v", err)
	}
	res, err = e.taskSvc.Delete(context.Background(), other.ID.String(), []uuid.UUID{e.it.ID})
	must(t, err)
	if len(res) != 1 || res[0].Outcome != "closed" || res[0].Problem == nil || res[0].Problem.Code != "issue_sync.remove_only_closed" {
		t.Errorf("remove as developer = %+v", res)
	}
	if issue, _ = e.fake.Issue(glPath, 2); issue.Deleted || issue.State != "closed" {
		t.Errorf("the developer's issue = %+v, want closed", issue)
	}
}

// A mesma tarefa numa issue do GitHub e numa do GitLab: o que muda num lado chega ao outro pela tarefa, e o
// fechamento também. Depois de assentado, nenhuma rodada escreve de novo.
func TestDual_GitHubAndGitLab(t *testing.T) {
	e := newGitLabEnv(t)
	gh := testutil.NewGitHub()
	gh.AddRepo(repo)
	ghSrv := httptest.NewServer(gh)
	t.Cleanup(ghSrv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: ghSrv.URL} })
	yes := true
	ghIt, err := e.integ.Create(e.project, "github", "GitHub", "ghp_test", map[string]any{"repo": repo}, true)
	must(t, err)
	ghIt, err = e.integ.Edit(ghIt.ID.String(), integration.EditInput{SyncIssues: &yes})
	must(t, err)
	e.tasks.SetChangeHook(e.syncer.Notify)

	tk, err := e.taskSvc.CreateAs("", e.project, "Nas duas", "descrição", "", nil, task.Attrs{})
	must(t, err)
	for _, it := range []*integration.Integration{ghIt, e.it} {
		_, err := e.syncer.Publish(context.Background(), tk.ID, it.ID)
		must(t, err)
	}
	if issue, ok := e.fake.Issue(glPath, 1); !ok || issue.Title != "Nas duas" {
		t.Fatalf("GitLab issue = %+v", issue)
	}
	if issue, ok := gh.Issue(repo, 1); !ok || issue.Title != "Nas duas" {
		t.Fatalf("GitHub issue = %+v", issue)
	}

	e.fake.EditIssue(glPath, 1, func(i *testutil.GitLabIssue) { i.Title = "Mudou no GitLab" })
	if sum := e.sync(issuesync.Full); sum.Updated != 1 {
		t.Fatalf("GitLab round = %+v", sum)
	}
	e.syncer.Flush(context.Background())
	if issue, _ := gh.Issue(repo, 1); issue.Title != "Mudou no GitLab" {
		t.Errorf("GitHub issue title = %q, want the change to reach it through the task", issue.Title)
	}

	gh.EditIssue(repo, 1, func(i *testutil.GitHubIssue) { i.State = "closed"; i.StateReason = "completed" })
	if _, err := e.syncer.Sync(context.Background(), ghIt.ID, issuesync.Full); err != nil {
		t.Fatal(err)
	}
	e.syncer.Flush(context.Background())
	if issue, _ := e.fake.Issue(glPath, 1); issue.State != "closed" {
		t.Errorf("GitLab issue state = %q, want it closed through the task", issue.State)
	}

	e.fake.Reset()
	gh.Reset()
	e.sync(issuesync.Full)
	if _, err := e.syncer.Sync(context.Background(), ghIt.ID, issuesync.Full); err != nil {
		t.Fatal(err)
	}
	e.syncer.Flush(context.Background())
	if e.fake.Writes() != 0 || gh.Writes() != 0 || e.syncer.Pending() != 0 {
		t.Errorf("after settling: GitLab %v, GitHub %v, pending %d", e.fake.Requests(), gh.Requests(), e.syncer.Pending())
	}
}
