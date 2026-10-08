package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"working-time-tracker/internal/adapter"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

// syncServer sobe o servidor ligado a um GitHub fake vazio (o repositório owner/repo) onde Ana e Bia têm
// usuário com e-mail público.
func syncServer(t *testing.T) (*server.App, *testutil.GitHub) {
	t.Helper()
	fake := testutil.NewGitHub()
	fake.AddRepo("owner/repo")
	fake.AddUser("ana-dev", "ana@test.com")
	fake.AddUser("bia-dev", "bia@test.com")
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: srv.URL} })
	t.Cleanup(func() {
		adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{} })
	})

	testutil.Truncate(t, testDB)
	app, err := server.Build(testClient, server.Options{EncryptKey: "test-key", AuthRateLimit: 1000})
	if err != nil {
		t.Fatalf("server.Build: %v", err)
	}
	return app, fake
}

// A sincronização pela API, de ponta a ponta: ligar, o botão, e o gancho levando ao GitHub o que se faz nas
// tarefas (editar, tirar o responsável, bater o ponto).
func TestIssueSync_EndToEnd(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	allocate(t, e, admin, prj, member.id, 5000)
	one := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Corrigir login", Body: "texto", Labels: []string{"bug"}, Assignees: []string{"ana-dev"}})
	two := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Livre"})

	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create integration = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	integ := created["id"].(string)
	if created["sync_issues"] != false || created["last_synced_at"] != nil || created["last_sync_error"] != "" {
		t.Errorf("a new integration has the sync off: %v", created)
	}
	syncURL := "/api/integrations/" + integ + "/sync"

	// Desligada, o botão avisa.
	rec = do(e, "POST", syncURL, "", admin.session)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_off" {
		t.Errorf("sync while off = %d %s", rec.Code, rec.Body.String())
	}

	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, member.session)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member turned the sync on: %d", rec.Code)
	}
	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, admin.session)
	if rec.Code != http.StatusOK || decode(t, rec)["sync_issues"] != true {
		t.Fatalf("turn on = %d: %s", rec.Code, rec.Body.String())
	}

	// O botão é de quem cuida das integrações.
	if rec = do(e, "POST", syncURL, "", member.session); rec.Code != http.StatusForbidden {
		t.Errorf("a member pressed Sincronizar agora: %d", rec.Code)
	}
	other := signup(t, e, "Outra org", "outro@test.com")
	if rec = do(e, "POST", syncURL, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization pressed it: %d, want 404", rec.Code)
	}
	if rec = do(e, "POST", syncURL, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", rec.Code)
	}

	rec = do(e, "POST", syncURL, "", admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync = %d: %s", rec.Code, rec.Body.String())
	}
	sum := decode(t, rec)
	for key, want := range map[string]any{"created": 2.0, "updated": 0.0, "closed": 0.0, "pushed": 0.0, "unmapped": 0.0, "errors": 0.0, "partial": false} {
		if sum[key] != want {
			t.Errorf("summary[%s] = %v, want %v (%v)", key, sum[key], want, sum)
		}
	}
	if fake.Writes() != 0 {
		t.Errorf("importing wrote to GitHub: %v", fake.Requests())
	}

	rec = do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session)
	tasks := decodeList(t, rec)
	if len(tasks) != 2 {
		t.Fatalf("%d tasks, want 2", len(tasks))
	}
	byName := map[string]map[string]any{}
	for _, tk := range tasks {
		byName[tk["name"].(string)] = tk
	}
	login, free := byName["Corrigir login"], byName["Livre"]
	if login == nil || free == nil {
		t.Fatalf("tasks = %v", byName)
	}
	if login["assignee_id"] != admin.id || login["external_item_id"] != "1" || login["status"] != "backlog" {
		t.Errorf("imported task = %v", login)
	}
	if free["assignee_id"] != nil {
		t.Errorf("the issue with no assignee must be an available task: %v", free)
	}

	rec = do(e, "GET", "/api/integrations/"+integ, "", admin.session)
	it := decode(t, rec)
	if it["sync_issues"] != true || it["last_synced_at"] == nil || it["last_sync_error"] != "" || it["sync_unmatched"] != 0.0 {
		t.Errorf("integration after the sync = %v", it)
	}

	// Editar a tarefa leva a mudança à issue (o gancho avisa, o Flush empurra).
	loginID, freeID := login["id"].(string), free["id"].(string)
	fake.Reset()
	rec = do(e, "PATCH", "/api/tasks/"+loginID, `{"name":"Corrigir o login","description":"outro texto","assignee_id":"`+admin.id+`"}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit the task = %d: %s", rec.Code, rec.Body.String())
	}
	if app.Sync.Pending() == 0 {
		t.Fatal("editing a task must tell the sync")
	}
	app.Sync.Flush(context.Background())
	if issue, _ := fake.Issue("owner/repo", one); issue.Title != "Corrigir o login" || issue.Body != "outro texto" {
		t.Errorf("issue after the edit = %+v", issue)
	}

	// Tirar o responsável tira da issue.
	rec = do(e, "PATCH", "/api/tasks/"+loginID, `{"name":"Corrigir o login","description":"outro texto","assignee_id":""}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("unassign = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(context.Background())
	if issue, _ := fake.Issue("owner/repo", one); len(issue.Assignees) != 0 {
		t.Errorf("assignees on GitHub = %v, want none", issue.Assignees)
	}

	// Bater o ponto numa tarefa livre a torna de quem bateu, e isso vai para a issue.
	if rec = do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-in", `{"task_id":"`+freeID+`"}`, member.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock in = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(context.Background())
	if issue, _ := fake.Issue("owner/repo", two); len(issue.Assignees) != 1 || issue.Assignees[0] != "bia-dev" {
		t.Errorf("assignees on GitHub after the clock in = %v, want bia-dev", issue.Assignees)
	}
	// Em progresso é só daqui: o estado da issue não muda.
	if issue, _ := fake.Issue("owner/repo", two); issue.State != "open" {
		t.Errorf("issue state = %q", issue.State)
	}

	// Fechar a tarefa fecha a issue.
	if rec = do(e, "PATCH", "/api/tasks/"+freeID+"/attributes", `{"status":"closed"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("close = %d: %s", rec.Code, rec.Body.String())
	}
	app.Sync.Flush(context.Background())
	if issue, _ := fake.Issue("owner/repo", two); issue.State != "closed" {
		t.Errorf("issue state after closing the task = %q", issue.State)
	}

	// Com a sincronização ligada o repositório não troca.
	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"metadata":{"repo":"owner/other"}}`, admin.session)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_repo_locked" {
		t.Errorf("change the repository with the sync on = %d %s", rec.Code, rec.Body.String())
	}
	// Desligar e trocar vale.
	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":false,"metadata":{"repo":"owner/other"}}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Errorf("turn off and change the repository = %d: %s", rec.Code, rec.Body.String())
	}
}

// Só roda uma rodada de cada vez: o botão responde 409 enquanto outra está em andamento.
func TestIssueSync_ButtonRefusesWhileRunning(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Uma"})
	gate, started := make(chan struct{}), make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/issues" {
			select {
			case started <- struct{}{}:
			default:
			}
			<-gate
		}
		fake.ServeHTTP(w, r)
	}))
	defer slow.Close()
	adapter.Register("github", func() adapter.Integration { return &adapter.GitHubIntegration{BaseURL: slow.URL} })

	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")
	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	integ := decode(t, rec)["id"].(string)
	do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, admin.session)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session) }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the first round did not start")
	}
	rec = do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "integration.sync_running" {
		t.Errorf("second press = %d %s, want 409 integration.sync_running", rec.Code, rec.Body.String())
	}
	close(gate)
	if first := <-done; first.Code != http.StatusOK {
		t.Errorf("first press = %d: %s", first.Code, first.Body.String())
	}
}

// Ligar a sincronização pede o que ela precisa.
func TestIssueSync_TurningItOnNeedsTheBasics(t *testing.T) {
	app, _ := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	prj := createProject(t, e, admin, "Alfa")

	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Parada","token":"tok","metadata":{"repo":"owner/repo"},"enabled":false}`, admin.session)
	integ := decode(t, rec)["id"].(string)
	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, admin.session)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_needs_enabled" {
		t.Errorf("turn on a disabled integration = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "PATCH", "/api/integrations/"+integ, `{"enabled":true,"sync_issues":true}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Errorf("enable and turn on together = %d: %s", rec.Code, rec.Body.String())
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	detail, _ := decode(t, rec)["error"].(map[string]any)
	code, _ := detail["code"].(string)
	return code
}
