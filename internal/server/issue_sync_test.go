package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	if login["assignee_id"] != admin.id || !linkedItem(login, "1") || login["status"] != "backlog" {
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

// O botão Sincronizar da tela da tarefa: relê a issue dela no GitHub, sem esperar a rodada de fundo.
func TestIssueSync_TaskButton(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	allocate(t, e, admin, prj, member.id, 5000)
	one := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Corrigir login", Labels: []string{"bug"}})

	rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	integ := decode(t, rec)["id"].(string)
	do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":true}`, admin.session)
	if rec = do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("first round = %d: %s", rec.Code, rec.Body.String())
	}
	tasks := decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session))
	if len(tasks) != 1 {
		t.Fatalf("%d tasks, want 1", len(tasks))
	}
	id := tasks[0]["id"].(string)
	syncURL := "/api/tasks/" + id + "/sync"

	// Mudou no GitHub, e o botão traz. Qualquer pessoa do projeto pode apertá-lo.
	fake.EditIssue("owner/repo", one, func(i *testutil.GitHubIssue) { i.Title = "Corrigir o login"; i.Labels = []string{"bug", "ux"} })
	rec = do(e, "POST", syncURL, "", member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("task sync = %d: %s", rec.Code, rec.Body.String())
	}
	if sum := decode(t, rec); sum["updated"] != 1.0 || sum["pushed"] != 0.0 || sum["errors"] != 0.0 || sum["problem"] != nil {
		t.Errorf("summary = %v, want one task updated", sum)
	}
	got := decode(t, do(e, "GET", "/api/tasks/"+id, "", admin.session))
	if got["name"] != "Corrigir o login" || len(got["labels"].([]any)) != 2 {
		t.Errorf("task after the sync = %v", got)
	}

	// Igual dos dois lados: nada vai nem vem.
	fake.Reset()
	rec = do(e, "POST", syncURL, "", admin.session)
	if sum := decode(t, rec); rec.Code != http.StatusOK || sum["updated"] != 0.0 || sum["pushed"] != 0.0 {
		t.Errorf("second press = %d %v, want nothing to do", rec.Code, sum)
	}
	if fake.Writes() != 0 {
		t.Errorf("a press with nothing different wrote to GitHub: %v", fake.Requests())
	}

	// Fechada no GitHub, fecha a tarefa.
	fake.EditIssue("owner/repo", one, func(i *testutil.GitHubIssue) { i.State = "closed" })
	if rec = do(e, "POST", syncURL, "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("press after the close = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, do(e, "GET", "/api/tasks/"+id, "", admin.session)); got["status"] != "closed" {
		t.Errorf("status after the issue closed = %v", got["status"])
	}

	// Uma tarefa ligada à mão a uma issue aberta passa a ser sincronizada.
	two := fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Livre"})
	rec = do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Ligada à mão"}`, admin.session)
	hand := decode(t, rec)["id"].(string)
	link := `{"integration_id":"` + integ + `","external_item_id":"` + strconv.Itoa(two) + `","external_item_url":"https://github.com/owner/repo/issues/` + strconv.Itoa(two) + `"}`
	if rec = do(e, "POST", "/api/tasks/"+hand+"/link-external-item", link, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("link = %d: %s", rec.Code, rec.Body.String())
	}
	if rec = do(e, "POST", "/api/tasks/"+hand+"/sync", "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("press on a hand-linked task = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, do(e, "GET", "/api/tasks/"+hand, "", admin.session)); got["name"] != "Livre" {
		t.Errorf("hand-linked task after the press = %v, want the issue's title", got["name"])
	}

	// A issue que sumiu do GitHub não tem o que sincronizar.
	fake.DeleteIssue("owner/repo", two)
	rec = do(e, "POST", "/api/tasks/"+hand+"/sync", "", admin.session)
	if rec.Code != http.StatusNotFound || errorCode(t, rec) != "integration.issue_gone" {
		t.Errorf("press on a deleted issue = %d %s", rec.Code, rec.Body.String())
	}

	// Sem item externo, não há o que sincronizar; de outra organização, a tarefa não existe.
	rec = do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"Solta"}`, admin.session)
	loose := decode(t, rec)["id"].(string)
	if rec = do(e, "POST", "/api/tasks/"+loose+"/sync", "", admin.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "task.no_external_item" {
		t.Errorf("press on a task with no link = %d %s", rec.Code, rec.Body.String())
	}
	other := signup(t, e, "Outra org", "outro@test.com")
	if rec = do(e, "POST", syncURL, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization pressed it: %d, want 404", rec.Code)
	}
	if rec = do(e, "POST", syncURL, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", rec.Code)
	}

	// Com a sincronização desligada, o botão avisa.
	do(e, "PATCH", "/api/integrations/"+integ, `{"sync_issues":false}`, admin.session)
	if rec = do(e, "POST", syncURL, "", admin.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_off" {
		t.Errorf("press with the sync off = %d %s", rec.Code, rec.Body.String())
	}
}

// O botão Sincronizar das listas de tarefas: uma rodada completa em cada integração do projeto com a
// sincronização ligada, apertado por qualquer pessoa do projeto.
func TestIssueSync_ProjectButton(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	allocate(t, e, admin, prj, member.id, 5000)
	fake.AddRepo("owner/other")
	fake.AddIssue("owner/repo", testutil.GitHubIssue{Title: "Do primeiro"})
	fake.AddIssue("owner/other", testutil.GitHubIssue{Title: "Do segundo"})
	syncURL := "/api/projects/" + prj + "/sync"

	// Sem integração com a sincronização ligada, o botão avisa.
	if rec := do(e, "POST", syncURL, "", member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_off" {
		t.Errorf("press with no sync on = %d %s", rec.Code, rec.Body.String())
	}

	// Duas integrações com a sincronização ligada e uma terceira, desligada, que não conta.
	for _, repo := range []string{"owner/repo", "owner/other"} {
		rec := do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"`+repo+`","token":"tok","metadata":{"repo":"`+repo+`"}}`, admin.session)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", repo, rec.Code, rec.Body.String())
		}
		if rec = do(e, "PATCH", "/api/integrations/"+decode(t, rec)["id"].(string), `{"sync_issues":true}`, admin.session); rec.Code != http.StatusOK {
			t.Fatalf("turn on = %d: %s", rec.Code, rec.Body.String())
		}
	}
	do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Parada","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)

	// Qualquer pessoa do projeto aperta, e a soma das duas rodadas volta.
	rec := do(e, "POST", syncURL, "", member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("project sync = %d: %s", rec.Code, rec.Body.String())
	}
	if sum := decode(t, rec); sum["created"] != 2.0 || sum["errors"] != 0.0 || sum["partial"] != false {
		t.Errorf("summary = %v, want two tasks created across the two repositories", sum)
	}
	if tasks := decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session)); len(tasks) != 2 {
		t.Errorf("%d tasks after the press, want 2", len(tasks))
	}

	// Uma segunda rodada sem diferença não escreve nada.
	fake.Reset()
	if rec = do(e, "POST", syncURL, "", member.session); rec.Code != http.StatusOK || decode(t, rec)["created"] != 0.0 {
		t.Errorf("second press = %d %s", rec.Code, rec.Body.String())
	}
	if fake.Writes() != 0 {
		t.Errorf("a press with nothing different wrote to GitHub: %v", fake.Requests())
	}

	// Quem não é do projeto não aperta; sem sessão também não.
	other := signup(t, e, "Outra org", "outro@test.com")
	if rec = do(e, "POST", syncURL, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization pressed it: %d, want 404", rec.Code)
	}
	if rec = do(e, "POST", syncURL, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", rec.Code)
	}
}

// O passo Integrações do modal Nova tarefa: postar a tarefa como uma issue nova e ligá-la a ela.
func TestIssueSync_PublishTask(t *testing.T) {
	app, fake := syncServer(t)
	e := app.Echo
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	carla := invite(t, e, admin, "carla@test.com", "member") // sem usuário no GitHub
	prj := createProject(t, e, admin, "Alfa")
	allocate(t, e, admin, prj, member.id, 5000)
	allocate(t, e, admin, prj, carla.id, 5000)

	rec := do(e, "POST", "/api/projects/"+prj+"/labels", `{"name":"bug"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create label = %d: %s", rec.Code, rec.Body.String())
	}
	label := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/projects/"+prj+"/integrations", `{"type":"github","display_name":"Repo","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	integ := decode(t, rec)["id"].(string)

	newTask := func(body string) string {
		t.Helper()
		rec := do(e, "POST", "/api/projects/"+prj+"/tasks", body, admin.session)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
		}
		return decode(t, rec)["id"].(string)
	}
	publish := func(task, integration, session string) *httptest.ResponseRecorder {
		return do(e, "POST", "/api/tasks/"+task+"/publish", `{"integration_id":"`+integration+`"}`, session)
	}
	withSync := `{"sync_issues":true}`

	first := newTask(`{"name":"Corrigir login","description":"texto longo","assignee_id":"` + admin.id + `","priority":"high","deadline":"2030-01-02T00:00:00Z","label_ids":["` + label + `"]}`)

	// Com a sincronização desligada, não há o que postar: é ela que mantém as duas iguais.
	if rec = publish(first, integ, member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.sync_off" {
		t.Errorf("publish with the sync off = %d %s", rec.Code, rec.Body.String())
	}
	do(e, "PATCH", "/api/integrations/"+integ, withSync, admin.session)

	// A integração de outro projeto não vale.
	otherPrj := createProject(t, e, admin, "Beta")
	rec = do(e, "POST", "/api/projects/"+otherPrj+"/integrations", `{"type":"github","display_name":"Outro","token":"tok","metadata":{"repo":"owner/repo"}}`, admin.session)
	if rec = publish(first, decode(t, rec)["id"].(string), member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "task.integration_other_project" {
		t.Errorf("publish to another project's integration = %d %s", rec.Code, rec.Body.String())
	}

	// Postar: título, corpo, etiqueta (criada no repositório) e o responsável achado pelo e-mail público.
	rec = publish(first, integ, member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish = %d: %s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	linked := out["task"].(map[string]any)
	links := linksOf(linked)
	if len(links) != 1 || links[0]["item_id"] != "1" || links[0]["integration_id"] != integ || linked["priority"] != "high" || out["problem"] != nil {
		t.Errorf("published task = %v, problem %v", linked, out["problem"])
	}
	if len(links) == 1 {
		if url, _ := links[0]["url"].(string); url == "" {
			t.Errorf("the task has no issue URL: %v", linked)
		}
	}
	issue, ok := fake.Issue("owner/repo", 1)
	if !ok || issue.Title != "Corrigir login" || issue.Body != "texto longo" || issue.State != "open" ||
		len(issue.Labels) != 1 || issue.Labels[0] != "bug" || len(issue.Assignees) != 1 || issue.Assignees[0] != "ana-dev" {
		t.Errorf("issue = %+v", issue)
	}

	// A tarefa e a issue já saem em acordo: uma rodada completa não escreve nada nem importa a issue de novo.
	fake.Reset()
	rec = do(e, "POST", "/api/integrations/"+integ+"/sync", "", admin.session)
	if sum := decode(t, rec); rec.Code != http.StatusOK || sum["created"] != 0.0 || sum["pushed"] != 0.0 || sum["updated"] != 0.0 {
		t.Errorf("round after publishing = %d %v, want nothing to do", rec.Code, sum)
	}
	if fake.Writes() != 0 {
		t.Errorf("the round after publishing wrote to GitHub: %v", fake.Requests())
	}
	if tasks := decodeList(t, do(e, "GET", "/api/projects/"+prj+"/tasks", "", admin.session)); len(tasks) != 1 {
		t.Errorf("%d tasks after publishing and a round, want 1", len(tasks))
	}

	// Postar de novo a mesma tarefa não vale.
	if rec = publish(first, integ, member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "task.already_linked" {
		t.Errorf("publish twice = %d %s", rec.Code, rec.Body.String())
	}

	// Um responsável sem usuário no GitHub: a issue sai sem responsável, a tarefa fica com ele e o aviso diz.
	second := newTask(`{"name":"Sem usuário","assignee_id":"` + carla.id + `"}`)
	rec = publish(second, integ, member.session)
	if rec.Code != http.StatusOK || decode(t, rec)["problem"] != "issue_sync.no_login" {
		t.Fatalf("publish with an unmapped assignee = %d %s", rec.Code, rec.Body.String())
	}
	if issue, _ = fake.Issue("owner/repo", 2); len(issue.Assignees) != 0 {
		t.Errorf("assignees on GitHub = %v, want none", issue.Assignees)
	}
	if got := decode(t, do(e, "GET", "/api/tasks/"+second, "", admin.session)); got["assignee_id"] != carla.id {
		t.Errorf("the task lost its assignee: %v", got["assignee_id"])
	}

	// Sem permissão de escrita a issue sairia diferente da tarefa: não posta, e a tarefa fica solta.
	third := newTask(`{"name":"Só leitura"}`)
	fake.SetPush("owner/repo", false)
	if rec = publish(third, integ, member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "issue_sync.publish_read_only" {
		t.Errorf("publish read-only = %d %s", rec.Code, rec.Body.String())
	}
	fake.SetPush("owner/repo", true)
	if _, ok := fake.Issue("owner/repo", 3); ok {
		t.Error("a read-only publish created an issue")
	}
	if got := decode(t, do(e, "GET", "/api/tasks/"+third, "", admin.session)); len(linksOf(got)) != 0 {
		t.Errorf("a refused publish linked the task: %v", got["links"])
	}

	// O repositório sem issues.
	fake.DisableIssues("owner/repo")
	if rec = publish(third, integ, member.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "integration.issues_disabled" {
		t.Errorf("publish with issues off = %d %s", rec.Code, rec.Body.String())
	}

	// De outra organização a tarefa não existe; sem sessão, nem entra.
	other := signup(t, e, "Outra org", "outro@test.com")
	if rec = publish(third, integ, other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization published: %d, want 404", rec.Code)
	}
	if rec = publish(third, integ, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", rec.Code)
	}
	if rec = do(e, "POST", "/api/tasks/"+third+"/publish", `{`, admin.session); rec.Code != http.StatusBadRequest {
		t.Errorf("broken body: %d", rec.Code)
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

// linksOf devolve os vínculos de uma tarefa do JSON da API.
func linksOf(task map[string]any) []map[string]any {
	raw, _ := task["links"].([]any)
	out := make([]map[string]any, len(raw))
	for i, l := range raw {
		out[i], _ = l.(map[string]any)
	}
	return out
}

// linkedItem diz se a tarefa tem um vínculo com este item (o número da issue, o link curto do cartão).
func linkedItem(task map[string]any, item string) bool {
	for _, l := range linksOf(task) {
		if l["item_id"] == item {
			return true
		}
	}
	return false
}
