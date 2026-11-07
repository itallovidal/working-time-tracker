package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"working-time-tracker/testutil"
)

const glProject = "group/project"

// gitlabFixture sobe o GitLab fake com o projeto group/project vazio (a conta é Owner) e devolve o adapter
// ligado a ele.
func gitlabFixture(t *testing.T) (*GitLabIntegration, Connection, *testutil.GitLab) {
	t.Helper()
	fake := testutil.NewGitLab()
	fake.AddProject(glProject) // recria vazio
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return &GitLabIntegration{BaseURL: srv.URL + "/api/v4"},
		Connection{Token: testutil.GitLabOAuthToken, Metadata: map[string]any{"project_url": glProject}},
		fake
}

// O nível de acesso decide se a conta escreve: Planner ou acima escreve, Guest só lê.
func TestGitLabSync_Repo(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()

	repo, err := g.Repo(ctx, conn)
	if err != nil || repo.FullName != glProject || !repo.CanPush || repo.Archived {
		t.Fatalf("repo = %+v, %v", repo, err)
	}
	fake.SetAccess(glProject, testutil.GitLabPlanner)
	if repo, _ = g.Repo(ctx, conn); !repo.CanPush {
		t.Errorf("a planner writes: %+v", repo)
	}
	fake.SetAccess(glProject, testutil.GitLabGuest)
	fake.SetArchived(glProject, true)
	if repo, _ = g.Repo(ctx, conn); repo.CanPush || !repo.Archived {
		t.Errorf("a guest on an archived project = %+v", repo)
	}
	fake.SetAccess(glProject, 0)
	if repo, err = g.Repo(ctx, conn); err != nil || repo.CanPush {
		t.Errorf("not a member = %+v, %v", repo, err)
	}
	if n := fake.Count("GET", "/api/v4/projects/group%2Fproject/members/all/"+strconv.Itoa(testutil.GitLabUserID)); n != 4 {
		t.Errorf("%d reads of the effective access level, want 4", n)
	}

	conn.Metadata = map[string]any{"project_url": "group/missing"}
	if _, err := g.Repo(ctx, conn); !errors.Is(err, ErrGitLabProjectMissing) {
		t.Errorf("missing project: err = %v", err)
	}
	conn.Token = testutil.InvalidToken
	conn.Metadata = map[string]any{"project_url": glProject}
	if _, err := g.Repo(ctx, conn); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("invalid token: err = %v", err)
	}
}

// A lista junta todas as páginas, deixa de fora o que não é issue e a transferida, traduz "opened" e informa o
// relógio do GitLab.
func TestGitLabSync_ListIssuesPagesAndTypes(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	for n := 1; n <= 230; n++ {
		kind := "issue"
		if n%50 == 0 {
			kind = "incident"
		}
		fake.AddIssue(glProject, testutil.GitLabIssue{Title: fmt.Sprintf("Issue %d", n), Type: kind})
	}
	fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Fechada", State: "closed"})

	before := time.Now().Add(-5 * time.Second)
	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Issues) != 230-4 { // 4 incidentes (50, 100, 150, 200); a fechada não é pedida
		t.Errorf("got %d issues, want %d", len(list.Issues), 230-4)
	}
	if first := list.Issues[0]; first.ID != "1" || first.Number != 1 || first.Title != "Issue 1" || first.State != "open" ||
		first.URL != "https://gitlab.com/group/project/-/issues/1" || first.UpdatedAt.IsZero() {
		t.Errorf("first issue = %+v", first)
	}
	if list.ServerTime.Before(before) || list.ServerTime.After(time.Now().Add(5*time.Second)) {
		t.Errorf("server time = %v, want about now", list.ServerTime)
	}
	if pages := fake.Count("GET", "/api/v4/projects/group%2Fproject/issues"); pages != 3 {
		t.Errorf("%d requests for 226 issues, want 3 pages of 100", pages)
	}
	q := fake.Requests()[0].Query
	for _, want := range []string{"state=opened", "per_page=100", "issue_type=issue", "order_by=created_at", "sort=asc"} {
		if !strings.Contains(q, want) {
			t.Errorf("first request query = %q, want %s", q, want)
		}
	}
}

func TestGitLabSync_ListIssuesSinceAndState(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	a := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "A"})
	fake.AddIssue(glProject, testutil.GitLabIssue{Title: "B"})
	c := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "C"})
	moved := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Transferida"})
	fake.EditIssue(glProject, a, func(i *testutil.GitLabIssue) { i.State = "closed" })
	fake.EditIssue(glProject, c, func(i *testutil.GitLabIssue) { i.Title = "C editada" })
	fake.MoveIssue(glProject, moved)
	since := time.Now().Add(-time.Minute)

	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{State: "all", Since: since, ByUpdated: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var numbers []int
	for _, i := range list.Issues {
		numbers = append(numbers, i.Number)
	}
	// A B não foi mexida depois de criada: a que foi mexida há mais tempo vem primeiro. A transferida fica de fora.
	if !reflect.DeepEqual(numbers, []int{2, 1, 3}) {
		t.Fatalf("all since a minute ago = %v, want every issue but the moved one, oldest update first", numbers)
	}
	if list.Issues[1].State != "closed" || list.Issues[2].Title != "C editada" {
		t.Errorf("issues = %+v", list.Issues)
	}
	for _, q := range fake.Requests() {
		if !strings.Contains(q.Query, "updated_after=") || !strings.Contains(q.Query, "order_by=updated_at") || !strings.Contains(q.Query, "state=all") {
			t.Errorf("query = %q", q.Query)
		}
	}

	later, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{State: "all", Since: time.Now().Add(time.Hour)})
	if err != nil || len(later.Issues) != 0 {
		t.Errorf("since in the future = %d issues, %v", len(later.Issues), err)
	}
}

// O endereço da próxima página que aponta para outro servidor nunca recebe o token.
func TestGitLabSync_PaginationStaysOnTheServer(t *testing.T) {
	var foreignHits int
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignHits++ }))
	defer foreign.Close()
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<`+foreign.URL+`/steal>; rel="next"`)
		_, _ = w.Write([]byte(`[]`)) // o servidor de teste não tem a quem avisar do erro
	}))
	defer evil.Close()

	g := &GitLabIntegration{BaseURL: evil.URL}
	_, err := g.ListIssues(context.Background(), Connection{Token: "secret", Metadata: map[string]any{"project_url": glProject}}, ListIssuesOptions{})
	if !errors.Is(err, ErrUnexpectedResponse) || foreignHits != 0 {
		t.Errorf("err = %v, foreign hits = %d; the next link of another server must be refused", err, foreignHits)
	}
}

// Uma issue apagada, inexistente ou transferida para outro projeto é "sumiu".
func TestGitLabSync_GoneIssues(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	deleted := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Apagada"})
	moved := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Transferida"})
	fake.DeleteIssue(glProject, deleted)
	fake.MoveIssue(glProject, moved)

	title := "novo"
	for name, number := range map[string]int{"deleted": deleted, "moved": moved, "never existed": 999} {
		if _, err := g.GetIssue(ctx, conn, strconv.Itoa(number)); !errors.Is(err, ErrIssueGone) {
			t.Errorf("get %s: err = %v, want gone", name, err)
		}
		if name == "moved" {
			continue // o GitLab aceita editar a issue que ficou no projeto antigo; a sincronização a lê antes
		}
		if _, err := g.UpdateIssue(ctx, conn, strconv.Itoa(number), IssuePatch{Title: &title}); !errors.Is(err, ErrIssueGone) {
			t.Errorf("update %s: err = %v, want gone", name, err)
		}
	}
}

func TestGitLabSync_GetIssue(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	n := fake.AddIssue(glProject, testutil.GitLabIssue{
		Title: "Corrigir", Description: "linha 1\r\nlinha 2", Labels: []string{"bug", "Urgente"}, Assignees: []string{testutil.GitLabLogin},
	})

	issue, err := g.GetIssue(context.Background(), conn, strconv.Itoa(n))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if issue.Title != "Corrigir" || issue.Body != "linha 1\r\nlinha 2" || issue.State != "open" ||
		!reflect.DeepEqual(issue.Labels, []string{"bug", "Urgente"}) || !reflect.DeepEqual(issue.Assignees, []string{testutil.GitLabLogin}) {
		t.Errorf("issue = %+v", issue)
	}
	empty := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Sem corpo"})
	if issue, _ = g.GetIssue(context.Background(), conn, strconv.Itoa(empty)); issue.Body != "" || issue.Labels == nil || issue.Assignees == nil {
		t.Errorf("an empty issue must come with an empty body and empty (not nil) lists: %+v", issue)
	}
	if _, err := g.GetIssue(context.Background(), conn, "abc"); !errors.Is(err, ErrInvalidIssueNumber) {
		t.Errorf("a non-numeric id: %v", err)
	}
}

func TestGitLabSync_UpdateIssue(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "ana@example.com")
	fake.AddUser("bia", "")
	n := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Velha", Description: "texto", Labels: []string{"bug"}, Assignees: []string{"bia"}})
	id := strconv.Itoa(n)

	title, body, closed := "Nova", "outro texto", "closed"
	labels := []string{"bug", "feature"}
	assignees := []string{"ana-dev", "nobody"} // quem o GitLab não tem fica de fora
	got, err := g.UpdateIssue(ctx, conn, id, IssuePatch{Title: &title, Body: &body, State: &closed, Labels: &labels, Assignees: &assignees})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Title != "Nova" || got.Body != "outro texto" || got.State != "closed" ||
		!reflect.DeepEqual(got.Labels, []string{"bug", "feature"}) || !reflect.DeepEqual(got.Assignees, []string{"ana-dev"}) {
		t.Errorf("updated = %+v", got)
	}
	put := fake.Requests()[len(fake.Requests())-1]
	if put.Method != "PUT" || !strings.Contains(put.Body, `"state_event":"close"`) || !strings.Contains(put.Body, `"labels":"bug,feature"`) ||
		strings.Contains(put.Body, "state_reason") {
		t.Errorf("put body = %s", put.Body)
	}

	// Reabrir, e as listas vazias limpam.
	open, none := "open", []string{}
	got, err = g.UpdateIssue(ctx, conn, id, IssuePatch{State: &open, Labels: &none, Assignees: &none})
	if err != nil || got.State != "open" || len(got.Labels) != 0 || len(got.Assignees) != 0 {
		t.Errorf("reopen and clear = %+v, %v", got, err)
	}
	if last := fake.Requests()[len(fake.Requests())-1]; !strings.Contains(last.Body, `"assignee_ids":[]`) || !strings.Contains(last.Body, `"labels":""`) ||
		!strings.Contains(last.Body, `"state_event":"reopen"`) {
		t.Errorf("clear body = %s", last.Body)
	}

	// Um patch vazio não muda nada além do updated_at.
	if got, err = g.UpdateIssue(ctx, conn, id, IssuePatch{}); err != nil || got.Title != "Nova" {
		t.Errorf("empty patch = %+v, %v", got, err)
	}

	// Sem escrita (Guest), o GitLab recusa.
	fake.SetAccess(glProject, testutil.GitLabGuest)
	if _, err := g.UpdateIssue(ctx, conn, id, IssuePatch{Title: &title}); !errors.Is(err, ErrForbidden) {
		t.Errorf("a guest edits: err = %v, want forbidden", err)
	}
}

func TestGitLabSync_Labels(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	for n := 1; n <= 130; n++ {
		if err := g.CreateLabel(ctx, conn, fmt.Sprintf("etiqueta-%03d", n)); err != nil {
			t.Fatalf("create label %d: %v", n, err)
		}
	}
	// A que já existe não é erro (409).
	if err := g.CreateLabel(ctx, conn, "ETIQUETA-001"); err != nil {
		t.Errorf("an existing label: %v", err)
	}
	names, err := g.ListLabels(ctx, conn)
	if err != nil || len(names) != 130 {
		t.Fatalf("labels = %d, %v", len(names), err)
	}
	if !strings.Contains(fake.Requests()[0].Body, `"color":"#ededed"`) {
		t.Errorf("create label body = %s", fake.Requests()[0].Body)
	}
	fake.LabelCreateStatus(glProject, http.StatusForbidden)
	if err := g.CreateLabel(ctx, conn, "nova"); !errors.Is(err, ErrForbidden) {
		t.Errorf("refused label: err = %v, want forbidden", err)
	}
}

// O responsável se liga pelo e-mail público, que no GitLab só vem lendo o usuário pelo id. A busca também casa
// o username, então o candidato só vale se o e-mail público dele for o procurado.
func TestGitLabSync_Users(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "ana@example.com")
	fake.AddUser("ghost", "")
	fake.AddUser("dup1", "dup@example.com")
	fake.AddUser("dup2", "dup@example.com")
	// O username contém o e-mail que se procura, mas o e-mail público é outro.
	fake.AddUser("bia@example.com", "")

	if email, err := g.UserEmail(ctx, conn, "ana-dev"); err != nil || email != "ana@example.com" {
		t.Errorf("ana's email = %q, %v", email, err)
	}
	for _, login := range []string{"ghost", "nobody", ""} {
		if email, err := g.UserEmail(ctx, conn, login); err != nil || email != "" {
			t.Errorf("%q's email = %q, %v; want none", login, email, err)
		}
	}
	if login, err := g.FindLoginByEmail(ctx, conn, "ANA@example.com"); err != nil || login != "ana-dev" {
		t.Errorf("login of ana's email = %q, %v", login, err)
	}
	for _, email := range []string{"dup@example.com", "bia@example.com", "nobody@example.com", "", "a b@example.com"} {
		if login, err := g.FindLoginByEmail(ctx, conn, email); err != nil || login != "" {
			t.Errorf("login of %q = %q, %v; want none", email, login, err)
		}
	}
}

func TestGitLabSync_RateLimitAndForbidden(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	fake.RateLimitedFor(90 * time.Second)
	_, err := g.ListIssues(ctx, conn, ListIssuesOptions{})
	until, limited := RateLimitedUntil(err)
	if !limited || until.Before(time.Now().Add(80*time.Second)) || until.After(time.Now().Add(100*time.Second)) {
		t.Errorf("rate limited: err = %v, until %v", err, until)
	}
	if !StopsSync(err) {
		t.Error("the rate limit must stop the round")
	}

	fake.RateLimitedFor(0)
	fake.Fail("GET", "/api/v4/projects/group%2Fproject/issues", http.StatusForbidden, 1)
	if _, err := g.ListIssues(ctx, conn, ListIssuesOptions{}); !errors.Is(err, ErrForbidden) || StopsSync(err) {
		t.Errorf("forbidden: err = %v (stops %v)", err, StopsSync(err))
	}
	fake.Fail("GET", "/api/v4/projects/group%2Fproject/issues", http.StatusBadGateway, 1)
	if _, err := g.ListIssues(ctx, conn, ListIssuesOptions{}); !errors.Is(err, ErrProviderStatus) {
		t.Errorf("bad gateway: err = %v", err)
	}
	conn.Metadata = map[string]any{"project_url": "group/missing"}
	if _, err := g.ListIssues(ctx, conn, ListIssuesOptions{}); !errors.Is(err, ErrGitLabProjectMissing) || !StopsSync(err) {
		t.Errorf("missing project: err = %v (stops %v)", err, StopsSync(err))
	}
}

func TestGitLabSync_ListStopsAtTheRateReserve(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	for n := 1; n <= 150; n++ {
		fake.AddIssue(glProject, testutil.GitLabIssue{Title: "x"})
	}
	fake.Remaining(gitlabRateReserve - 1)
	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{})
	if _, limited := RateLimitedUntil(err); !limited || len(list.Issues) != 100 {
		t.Errorf("err = %v, %d issues; want the first page and a rate limit", err, len(list.Issues))
	}
}

func TestGitLabSync_SendsTheBearerToken(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"iid":1,"state":"opened"}`)) // o servidor de teste não tem a quem avisar do erro
	}))
	defer srv.Close()
	g := &GitLabIntegration{BaseURL: srv.URL}
	if _, err := g.GetIssue(context.Background(), Connection{Token: "glpat-x", Metadata: map[string]any{"project_url": glProject}}, "1"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Get("Authorization") != "Bearer glpat-x" || got.Get("PRIVATE-TOKEN") != "" || got.Get("User-Agent") == "" {
		t.Errorf("headers = %v", got)
	}
}

func TestGitLabSync_CancelledContext(t *testing.T) {
	g, conn, _ := gitlabFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.ListIssues(ctx, conn, ListIssuesOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want canceled", err)
	}
}

func TestGitLabSync_CreateIssue(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "ana@example.com")

	issue, err := g.CreateIssue(ctx, conn, NewIssue{Title: "Nova", Body: "corpo", Labels: []string{"bug", "feature"}, Assignees: []string{"ana-dev"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if issue.ID != "1" || issue.Title != "Nova" || issue.Body != "corpo" || issue.State != "open" ||
		!reflect.DeepEqual(issue.Labels, []string{"bug", "feature"}) || !reflect.DeepEqual(issue.Assignees, []string{"ana-dev"}) {
		t.Errorf("created = %+v", issue)
	}

	// Guest abre a issue, mas as etiquetas e os responsáveis são descartados sem aviso.
	fake.SetAccess(glProject, testutil.GitLabGuest)
	issue, err = g.CreateIssue(ctx, conn, NewIssue{Title: "De quem só lê", Labels: []string{"bug"}, Assignees: []string{"ana-dev"}})
	if err != nil || len(issue.Labels) != 0 || len(issue.Assignees) != 0 {
		t.Errorf("a guest's issue = %+v, %v", issue, err)
	}

	fake.SetAccess(glProject, testutil.GitLabOwner)
	fake.DisableIssues(glProject)
	if _, err := g.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrIssuesDisabled) {
		t.Errorf("issues disabled: err = %v", err)
	}
	conn.Metadata = map[string]any{"project_url": "group/missing"}
	if _, err := g.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrGitLabProjectMissing) {
		t.Errorf("missing project: err = %v", err)
	}
}

// Só o Owner apaga a issue; os outros a fecham. A que já não existe não é erro.
func TestGitLabSync_RemoveItem(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	ctx := context.Background()

	owned := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "Do Owner"})
	if outcome, err := g.RemoveItem(ctx, conn, strconv.Itoa(owned)); err != nil || outcome != RemoveDeleted {
		t.Errorf("owner: %q, %v", outcome, err)
	}
	if i, _ := fake.Issue(glProject, owned); !i.Deleted {
		t.Error("the owner's issue was not deleted")
	}

	fake.SetAccess(glProject, testutil.GitLabDeveloper)
	dev := fake.AddIssue(glProject, testutil.GitLabIssue{Title: "De quem não é Owner"})
	if outcome, err := g.RemoveItem(ctx, conn, strconv.Itoa(dev)); err != nil || outcome != RemoveClosed {
		t.Errorf("developer: %q, %v", outcome, err)
	}
	if i, _ := fake.Issue(glProject, dev); i.Deleted || i.State != "closed" {
		t.Errorf("the developer's issue = %+v, want it closed", i)
	}

	for name, id := range map[string]string{"deleted": strconv.Itoa(owned), "never existed": "999"} {
		if outcome, err := g.RemoveItem(ctx, conn, id); err != nil || outcome != RemoveGone {
			t.Errorf("%s: %q, %v", name, outcome, err)
		}
	}
}

func TestGitLab_AccountAndProjects(t *testing.T) {
	g, conn, fake := gitlabFixture(t)
	if login, err := g.Account(conn); err != nil || login != testutil.GitLabLogin {
		t.Errorf("account = %q, %v", login, err)
	}
	if _, err := g.Account(Connection{Token: testutil.InvalidToken}); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("invalid token: %v", err)
	}
	fake.SetAccess("group/other", 0)
	repos, err := g.ListRepositories(conn)
	if err != nil || len(repos) != 1 || repos[0].FullName != glProject || !repos[0].Private {
		t.Errorf("projects = %+v, %v; want only the ones the account is a member of", repos, err)
	}
	if q := fake.Requests()[len(fake.Requests())-1].Query; !strings.Contains(q, "membership=true") {
		t.Errorf("query = %q", q)
	}
	second := Connection{Token: testutil.GitLabSecondOAuthToken, Metadata: conn.Metadata}
	if repos, _ := g.ListRepositories(second); len(repos) != 1 || repos[0].FullName != "group/second" || repos[0].Private {
		t.Errorf("the second account's projects = %+v", repos)
	}
}
