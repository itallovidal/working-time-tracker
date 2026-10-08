package adapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"working-time-tracker/testutil"
)

func init() { testutil.BlockExternalHTTP() }

// syncFixture sobe o GitHub fake com o repositório owner/repo vazio e devolve o adapter ligado a ele.
func syncFixture(t *testing.T) (*GitHubIntegration, Connection, *testutil.GitHub) {
	t.Helper()
	fake := testutil.NewGitHub()
	fake.AddRepo("owner/repo") // recria vazio
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return &GitHubIntegration{BaseURL: srv.URL},
		Connection{Token: testutil.GitHubOAuthToken, Metadata: map[string]any{"repo": "owner/repo"}},
		fake
}

func TestGitHubSync_Repo(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()

	repo, err := g.Repo(ctx, conn)
	if err != nil || repo.FullName != "owner/repo" || !repo.CanPush || repo.Archived {
		t.Fatalf("repo = %+v, %v", repo, err)
	}
	fake.SetPush("owner/repo", false)
	fake.SetArchived("owner/repo", true)
	if repo, _ = g.Repo(ctx, conn); repo.CanPush || !repo.Archived {
		t.Errorf("a read-only archived repo = %+v", repo)
	}

	conn.Metadata = map[string]any{"repo": "owner/missing"}
	if _, err := g.Repo(ctx, conn); !errors.Is(err, ErrGitHubRepoNotFound) {
		t.Errorf("missing repo: err = %v", err)
	}
	conn.Token = testutil.InvalidToken
	conn.Metadata = map[string]any{"repo": "owner/repo"}
	if _, err := g.Repo(ctx, conn); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("invalid token: err = %v", err)
	}
}

// A lista junta todas as páginas, deixa os pull requests de fora e informa o relógio do GitHub.
func TestGitHubSync_ListIssuesPagesAndPullRequests(t *testing.T) {
	g, conn, fake := syncFixture(t)
	for n := 1; n <= 230; n++ {
		fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: fmt.Sprintf("Issue %d", n), PullRequest: n%50 == 0})
	}
	fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Fechada", State: "closed"})

	before := time.Now().Add(-5 * time.Second)
	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Issues) != 230-4 { // 4 pull requests (50, 100, 150, 200); a fechada não é pedida
		t.Errorf("got %d issues, want %d", len(list.Issues), 230-4)
	}
	if first := list.Issues[0]; first.Number != 1 || first.Title != "Issue 1" || first.State != "open" ||
		first.URL != "https://github.com/owner/repo/issues/1" || first.UpdatedAt.IsZero() {
		t.Errorf("first issue = %+v", first)
	}
	for _, i := range list.Issues {
		if i.Number%50 == 0 && i.Number <= 200 {
			t.Errorf("pull request #%d came in the list", i.Number)
		}
	}
	if list.ServerTime.Before(before) || list.ServerTime.After(time.Now().Add(5*time.Second)) {
		t.Errorf("server time = %v, want about now", list.ServerTime)
	}
	if pages := fake.Count("GET", "/repos/owner/repo/issues"); pages != 3 {
		t.Errorf("%d requests for 226 issues, want 3 pages of 100", pages)
	}
	if got := fake.Requests()[0]; !strings.Contains(got.Query, "state=open") || !strings.Contains(got.Query, "per_page=100") {
		t.Errorf("first request query = %q", got.Query)
	}
}

func TestGitHubSync_ListIssuesSinceAndState(t *testing.T) {
	g, conn, fake := syncFixture(t)
	a := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "A"})
	fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "B"})
	c := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "C"})
	fake.EditIssue("owner/repo", a, func(i *testutil.GitHubIssue) { i.State = "closed" })
	fake.EditIssue("owner/repo", c, func(i *testutil.GitHubIssue) { i.Title = "C editada" })
	since := time.Now().Add(-time.Minute)

	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{State: "all", Since: since, ByUpdated: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var numbers []int
	for _, i := range list.Issues {
		numbers = append(numbers, i.Number)
	}
	// A B não foi mexida depois de criada: a que foi mexida há mais tempo vem primeiro.
	if !reflect.DeepEqual(numbers, []int{2, 1, 3}) {
		t.Fatalf("all since a minute ago = %v, want every issue, oldest update first", numbers)
	}
	if list.Issues[1].State != "closed" || list.Issues[2].Title != "C editada" {
		t.Errorf("issues = %+v", list.Issues)
	}
	for _, q := range fake.Requests() {
		if !strings.Contains(q.Query, "since=") || !strings.Contains(q.Query, "sort=updated") || !strings.Contains(q.Query, "direction=asc") {
			t.Errorf("query = %q", q.Query)
		}
	}

	later, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{State: "all", Since: time.Now().Add(time.Hour)})
	if err != nil || len(later.Issues) != 0 {
		t.Errorf("since in the future = %d issues, %v", len(later.Issues), err)
	}
}

// O endereço da próxima página que aponta para outro servidor nunca recebe o token.
func TestGitHubSync_PaginationStaysOnTheServer(t *testing.T) {
	var foreignHits int
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignHits++ }))
	defer foreign.Close()
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<`+foreign.URL+`/steal>; rel="next"`)
		w.Write([]byte(`[]`))
	}))
	defer evil.Close()

	g := &GitHubIntegration{BaseURL: evil.URL}
	_, err := g.ListIssues(context.Background(), Connection{Token: "secret", Metadata: map[string]any{"repo": "owner/repo"}}, ListIssuesOptions{})
	if !errors.Is(err, ErrUnexpectedResponse) || foreignHits != 0 {
		t.Errorf("err = %v, foreign hits = %d; the next link of another server must be refused", err, foreignHits)
	}
}

// Uma issue apagada (410), inexistente (404) ou transferida (301) é "sumiu"; o redirecionamento não é
// seguido, porque o net/http refaria um PATCH como GET e devolveria 200 sem gravar.
func TestGitHubSync_GoneIssues(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()
	deleted := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Apagada"})
	moved := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Transferida"})
	fake.DeleteIssue("owner/repo", deleted)
	fake.MoveIssue("owner/repo", moved)

	title := "novo"
	for name, number := range map[string]int{"deleted": deleted, "moved": moved, "never existed": 999} {
		if _, err := g.GetIssue(ctx, conn, number); !errors.Is(err, ErrIssueGone) {
			t.Errorf("get %s: err = %v, want gone", name, err)
		}
		if _, err := g.UpdateIssue(ctx, conn, number, IssuePatch{Title: &title}); !errors.Is(err, ErrIssueGone) {
			t.Errorf("update %s: err = %v, want gone", name, err)
		}
	}
	if n := fake.Count("GET", "/repositories"); n != 0 {
		t.Errorf("the redirect was followed %d times", n)
	}
}

func TestGitHubSync_GetIssue(t *testing.T) {
	g, conn, fake := syncFixture(t)
	n := fake.AddIssue("owner/repo", testutil.GitHubIssue{
		Title: "Corrigir", Body: "linha 1\r\nlinha 2", Labels: []string{"bug", "Urgente"}, Assignees: []string{"octocat"},
	})
	pr := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "PR", PullRequest: true})

	issue, err := g.GetIssue(context.Background(), conn, n)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if issue.Title != "Corrigir" || issue.Body != "linha 1\r\nlinha 2" || issue.State != "open" ||
		!reflect.DeepEqual(issue.Labels, []string{"bug", "Urgente"}) || !reflect.DeepEqual(issue.Assignees, []string{"octocat"}) || issue.PullRequest {
		t.Errorf("issue = %+v", issue)
	}
	empty := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Sem corpo"})
	if issue, _ = g.GetIssue(context.Background(), conn, empty); issue.Body != "" || issue.Labels == nil || issue.Assignees == nil {
		t.Errorf("an empty issue must come with an empty body and empty (not nil) lists: %+v", issue)
	}
	if issue, _ = g.GetIssue(context.Background(), conn, pr); !issue.PullRequest {
		t.Error("a pull request number must say so")
	}
}

func TestGitHubSync_UpdateIssue(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "ana@example.com")
	fake.AddUser("bia", "")
	n := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Velha", Body: "texto", Labels: []string{"bug"}, Assignees: []string{"bia"}})

	title, body, state, reason := "Nova", "outro texto", "closed", "completed"
	labels, assignees := []string{"bug"}, []string{"ana-dev"}
	issue, err := g.UpdateIssue(ctx, conn, n, IssuePatch{Title: &title, Body: &body, State: &state, StateReason: &reason, Labels: &labels, Assignees: &assignees})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if issue.Title != "Nova" || issue.Body != "outro texto" || issue.State != "closed" || issue.StateReason != "completed" ||
		!reflect.DeepEqual(issue.Labels, []string{"bug"}) || !reflect.DeepEqual(issue.Assignees, []string{"ana-dev"}) {
		t.Errorf("issue after update = %+v", issue)
	}

	// Só os campos pedidos vão: o resto não é tocado.
	fake.Reset()
	empty := []string{}
	if _, err := g.UpdateIssue(ctx, conn, n, IssuePatch{Labels: &empty, Assignees: &empty}); err != nil {
		t.Fatalf("clear lists: %v", err)
	}
	req := fake.Requests()[0]
	if req.Method != "PATCH" || strings.Contains(req.Body, "title") || !strings.Contains(req.Body, `"labels":[]`) || !strings.Contains(req.Body, `"assignees":[]`) {
		t.Errorf("request = %s %s %s", req.Method, req.Path, req.Body)
	}
	if after, _ := fake.Issue("owner/repo", n); len(after.Labels) != 0 || len(after.Assignees) != 0 || after.Title != "Nova" {
		t.Errorf("after clearing = %+v", after)
	}

	// Uma etiqueta que o repositório não tem é descartada pelo GitHub; o resultado diz.
	unknown := []string{"nao-existe"}
	issue, _ = g.UpdateIssue(ctx, conn, n, IssuePatch{Labels: &unknown})
	if len(issue.Labels) != 0 {
		t.Errorf("the label the repo does not have must not stick: %+v", issue.Labels)
	}
	// E um responsável que o GitHub não aceita também.
	fake.Unassignable("bia")
	who := []string{"bia", "ana-dev"}
	issue, _ = g.UpdateIssue(ctx, conn, n, IssuePatch{Assignees: &who})
	if !reflect.DeepEqual(issue.Assignees, []string{"ana-dev"}) {
		t.Errorf("assignees = %v, want the one GitHub accepts", issue.Assignees)
	}

	// Descarte silencioso: 200, issue como estava.
	fake.Discard("owner/repo", "title")
	other := "ignorado"
	if issue, err = g.UpdateIssue(ctx, conn, n, IssuePatch{Title: &other}); err != nil || issue.Title != "Nova" {
		t.Errorf("silently discarded title = %q, %v", issue.Title, err)
	}

	// Sem permissão de escrita é 403.
	fake.SetPush("owner/repo", false)
	if _, err := g.UpdateIssue(ctx, conn, n, IssuePatch{Title: &other}); !errors.Is(err, ErrForbidden) {
		t.Errorf("no push access: err = %v, want forbidden", err)
	}
}

func TestGitHubSync_Labels(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()
	for n := 1; n <= 130; n++ {
		if err := g.CreateLabel(ctx, conn, fmt.Sprintf("etiqueta-%03d", n)); err != nil {
			t.Fatalf("create %d: %v", n, err)
		}
	}
	names, err := g.ListLabels(ctx, conn)
	if err != nil || len(names) != 130 {
		t.Fatalf("labels = %d, %v", len(names), err)
	}
	// Uma etiqueta que já existe (com outra caixa) não é erro.
	if err := g.CreateLabel(ctx, conn, "ETIQUETA-001"); err != nil {
		t.Errorf("an existing label must not fail: %v", err)
	}
	if len(fake.LabelNames("owner/repo")) != 130 {
		t.Errorf("labels in the repo = %d, want 130", len(fake.LabelNames("owner/repo")))
	}
	fake.LabelCreateStatus("owner/repo", http.StatusForbidden)
	if err := g.CreateLabel(ctx, conn, "outra"); !errors.Is(err, ErrForbidden) {
		t.Errorf("refused label: err = %v, want forbidden", err)
	}
}

func TestGitHubSync_Users(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "Ana@Example.com")
	fake.AddUser("bia", "")
	fake.AddUser("twin-1", "twin@example.com")
	fake.AddUser("twin-2", "twin@example.com")

	for login, want := range map[string]string{"ana-dev": "Ana@Example.com", "bia": "", "ninguem": ""} {
		if got, err := g.UserEmail(ctx, conn, login); err != nil || got != want {
			t.Errorf("email of %s = %q, %v, want %q", login, got, err, want)
		}
	}
	fake.Reset()
	for _, login := range []string{"../orgs", "a/b", "", "x y", strings.Repeat("a", 40)} {
		if got, err := g.UserEmail(ctx, conn, login); err != nil || got != "" {
			t.Errorf("email of %q = %q, %v; a login that is not one must not reach the API", login, got, err)
		}
	}
	if len(fake.Requests()) != 0 {
		t.Errorf("a malformed login produced requests: %v", fake.Requests())
	}

	for email, want := range map[string]string{"ana@example.com": "ana-dev", "bia@example.com": "", "twin@example.com": ""} {
		if got, err := g.FindLoginByEmail(ctx, conn, email); err != nil || got != want {
			t.Errorf("login of %s = %q, %v, want %q", email, got, err, want)
		}
	}
	if got, _ := g.FindLoginByEmail(ctx, conn, "  "); got != "" {
		t.Errorf("blank email found %q", got)
	}
}

func TestGitHubSync_RateLimitAndForbidden(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()

	fake.RateLimitedFor(time.Hour)
	_, err := g.Repo(ctx, conn)
	until, ok := RateLimitedUntil(err)
	if !errors.Is(err, ErrRateLimited) || !ok || until.Before(time.Now().Add(50*time.Minute)) {
		t.Errorf("rate limited: err = %v, until = %v", err, until)
	}

	g2, conn2, f2 := syncFixture(t)
	f2.Fail("GET", "/repos/owner/repo", http.StatusTooManyRequests, 1)
	if _, err := g2.Repo(ctx, conn2); !errors.Is(err, ErrRateLimited) {
		t.Errorf("429: err = %v", err)
	}
	f2.Fail("GET", "/repos/owner/repo", http.StatusForbidden, 1)
	if _, err := g2.Repo(ctx, conn2); !errors.Is(err, ErrForbidden) || errors.Is(err, ErrRateLimited) {
		t.Errorf("403 without the limit: err = %v, want forbidden", err)
	}
	f2.Fail("GET", "/repos/owner/repo", http.StatusBadGateway, 1)
	if _, err := g2.Repo(ctx, conn2); !errors.Is(err, ErrProviderStatus) {
		t.Errorf("502: err = %v", err)
	}
	if _, ok := RateLimitedUntil(errors.New("outro")); ok {
		t.Error("another error is not a rate limit")
	}
}

// A listagem para antes de gastar o que resta do limite, e devolve o que já tinha lido.
func TestGitHubSync_ListStopsAtTheRateReserve(t *testing.T) {
	g, conn, fake := syncFixture(t)
	for n := 1; n <= 250; n++ {
		fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: fmt.Sprintf("Issue %d", n)})
	}
	fake.Remaining(3)
	list, err := g.ListIssues(context.Background(), conn, ListIssuesOptions{})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want rate limited", err)
	}
	if len(list.Issues) != 100 || fake.Count("GET", "/repos/owner/repo/issues") != 1 {
		t.Errorf("got %d issues in %d requests, want the first page only", len(list.Issues), fake.Count("GET", "/repos/owner/repo/issues"))
	}
}

func TestGitHubSync_SendsTheHeadersTheAPIAsksFor(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Write([]byte(`{"full_name":"owner/repo","permissions":{"push":true}}`))
	}))
	defer srv.Close()
	g := &GitHubIntegration{BaseURL: srv.URL}
	if _, err := g.Repo(context.Background(), Connection{Token: "tok", Metadata: map[string]any{"repo": "owner/repo"}}); err != nil {
		t.Fatalf("repo: %v", err)
	}
	if got.Get("Authorization") != "Bearer tok" || got.Get("X-GitHub-Api-Version") != "2022-11-28" ||
		got.Get("Accept") != "application/vnd.github+json" || got.Get("User-Agent") == "" {
		t.Errorf("headers = %v", got)
	}
}

func TestGitHubSync_CancelledContext(t *testing.T) {
	g, conn, _ := syncFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Repo(ctx, conn); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context canceled", err)
	}
}

func TestGitHubSync_CreateIssue(t *testing.T) {
	g, conn, fake := syncFixture(t)
	ctx := context.Background()
	fake.AddUser("ana-dev", "ana@test.com")
	if err := g.CreateLabel(ctx, conn, "bug"); err != nil {
		t.Fatal(err)
	}

	// A etiqueta que o repositório tem e o usuário que existe ficam; a etiqueta que ele não tem é descartada.
	issue, err := g.CreateIssue(ctx, conn, NewIssue{Title: "Corrigir login", Body: "texto", Labels: []string{"bug", "ux"}, Assignees: []string{"ana-dev", "ninguem"}})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Number != 1 || issue.Title != "Corrigir login" || issue.Body != "texto" || issue.State != "open" ||
		!reflect.DeepEqual(issue.Labels, []string{"bug"}) || !reflect.DeepEqual(issue.Assignees, []string{"ana-dev"}) {
		t.Errorf("issue = %+v", issue)
	}
	if stored, _ := fake.Issue("owner/repo", 1); stored.Title != "Corrigir login" {
		t.Errorf("the fake stored %+v", stored)
	}

	// Sem os campos opcionais, a issue sai só com título e corpo.
	if issue, err = g.CreateIssue(ctx, conn, NewIssue{Title: "Só o título"}); err != nil || issue.Number != 2 || len(issue.Labels) != 0 {
		t.Errorf("minimal issue = %+v, %v", issue, err)
	}

	// Sem permissão de escrita, o GitHub abre a issue e joga fora as etiquetas e o responsável sem avisar.
	fake.SetPush("owner/repo", false)
	issue, err = g.CreateIssue(ctx, conn, NewIssue{Title: "De quem só lê", Labels: []string{"bug"}, Assignees: []string{"ana-dev"}})
	if err != nil || len(issue.Labels) != 0 || len(issue.Assignees) != 0 {
		t.Errorf("read-only issue = %+v, %v", issue, err)
	}
	fake.SetPush("owner/repo", true)

	// As issues desligadas, um repositório que não existe e um token recusado.
	fake.DisableIssues("owner/repo")
	if _, err := g.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrIssuesDisabled) {
		t.Errorf("issues disabled: err = %v", err)
	}
	conn.Metadata = map[string]any{"repo": "owner/missing"}
	if _, err := g.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrGitHubRepoNotFound) {
		t.Errorf("missing repo: err = %v", err)
	}
	conn.Metadata = map[string]any{"repo": "owner/repo"}
	conn.Token = testutil.InvalidToken
	if _, err := g.CreateIssue(ctx, conn, NewIssue{Title: "x"}); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("bad token: err = %v", err)
	}
}
