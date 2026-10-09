package server_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// projectNames devolve os nomes de uma lista de projetos (o array de sempre ou os itens de uma página).
func projectNames(t *testing.T, items []map[string]any) []string {
	t.Helper()
	names := []string{}
	for _, it := range items {
		names = append(names, it["name"].(string))
	}
	slices.Sort(names)
	return names
}

// Quem não é admin só vê os projetos em que foi posto (com valor por hora): a lista da organização, as rotas
// de API e as páginas do projeto, das tarefas e dos times dele. Num projeto em que não está, a resposta é a
// de um projeto que não existe (404), para não revelar que ele existe. O dono e os admins veem todos.
func TestProjectAccess_MemberSeesOnlyTheirProjects(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member") // não está em projeto nenhum
	other := signup(t, e, "Outra", "zed@outra.com")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	beta := createEmptyProject(t, e, admin, "Projeto Beta")
	gama := createEmptyProject(t, e, admin, "Projeto Gama")
	allocate(t, e, admin, alfa, bia.id, 2000)
	allocate(t, e, admin, gama, bia.id, 3000)

	// O Beta tem uma tarefa e um time; a Bia não está nele.
	betaTask := decode(t, do(e, "POST", "/api/projects/"+beta+"/tasks", `{"name":"Tarefa do Beta"}`, admin.session))["id"].(string)
	betaTeam := decode(t, do(e, "POST", "/api/projects/"+beta+"/teams", `{"name":"Time do Beta"}`, admin.session))["id"].(string)
	alfaTask := decode(t, do(e, "POST", "/api/projects/"+alfa+"/tasks", `{"name":"Tarefa do Alfa"}`, admin.session))["id"].(string)

	list := "/api/orgs/" + admin.orgID + "/projects"
	// A lista: a Bia vê o Alfa e o Gama, o Caio nenhum (um array vazio, e não nulo) e o admin os três.
	if got := projectNames(t, decodeList(t, do(e, "GET", list, "", bia.session))); !slices.Equal(got, []string{"Projeto Alfa", "Projeto Gama"}) {
		t.Errorf("Bia's projects = %v, want only Alfa and Gama", got)
	}
	if rec := do(e, "GET", list, "", caio.session); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("Caio's projects = %d %q, want 200 []", rec.Code, rec.Body.String())
	}
	if got := projectNames(t, decodeList(t, do(e, "GET", list, "", admin.session))); len(got) != 3 {
		t.Errorf("admin's projects = %v, want the three of the organization", got)
	}
	// A página traz só os dela, e o total também.
	page := decode(t, do(e, "GET", list+"?page=1&per_page=1", "", bia.session))
	if page["total"] != float64(2) || len(page["items"].([]any)) != 1 {
		t.Errorf("Bia's first page = %v, want total 2 and one item", page)
	}
	page = decode(t, do(e, "GET", list+"?page=1&per_page=5", "", caio.session))
	if items, ok := page["items"].([]any); page["total"] != float64(0) || !ok || len(items) != 0 {
		t.Errorf("Caio's page = %v, want total 0 and an empty list", page)
	}

	// As rotas de API do Beta: tudo é 404 para a Bia, e no Alfa funciona.
	denied := []struct{ method, path, body string }{
		{"GET", "/api/projects/" + beta, ""},
		{"GET", "/api/projects/" + beta + "/tasks", ""},
		{"POST", "/api/projects/" + beta + "/tasks", `{"name":"Intrusa"}`},
		{"GET", "/api/projects/" + beta + "/members", ""},
		{"GET", "/api/projects/" + beta + "/collaborators", ""},
		{"GET", "/api/projects/" + beta + "/teams", ""},
		{"GET", "/api/projects/" + beta + "/work-sessions", ""},
		{"POST", "/api/projects/" + beta + "/work-sessions/clock-in", `{"task_id":"` + betaTask + `"}`},
		{"GET", "/api/tasks/" + betaTask, ""},
		{"PATCH", "/api/tasks/" + betaTask, `{"name":"Renomeada"}`},
		{"POST", "/api/tasks/" + betaTask + "/claim", ""},
		{"GET", "/api/teams/" + betaTeam, ""},
		{"GET", "/api/teams/" + betaTeam + "/members", ""},
	}
	for _, d := range denied {
		if rec := do(e, d.method, d.path, d.body, bia.session); rec.Code != http.StatusNotFound {
			t.Errorf("Bia %s %s = %d, want 404 (she is not in the project)", d.method, d.path, rec.Code)
		}
		// O Caio, que não está em projeto nenhum, também não entra.
		if rec := do(e, d.method, d.path, d.body, caio.session); rec.Code != http.StatusNotFound {
			t.Errorf("Caio %s %s = %d, want 404", d.method, d.path, rec.Code)
		}
	}
	// A tarefa do Beta não foi tocada pelas tentativas.
	if rec := do(e, "GET", "/api/tasks/"+betaTask, "", admin.session); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Tarefa do Beta") {
		t.Errorf("admin GET the Beta task = %d %s, want it untouched", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/projects/" + alfa, "/api/projects/" + alfa + "/tasks", "/api/tasks/" + alfaTask} {
		if rec := do(e, "GET", path, "", bia.session); rec.Code != http.StatusOK {
			t.Errorf("Bia GET %s = %d, want 200 (she is in the project)", path, rec.Code)
		}
		if rec := do(e, "GET", path, "", other.session); rec.Code != http.StatusNotFound {
			t.Errorf("another org GET %s = %d, want 404", path, rec.Code)
		}
	}
	for _, path := range []string{"/api/projects/" + beta, "/api/projects/" + beta + "/tasks", "/api/tasks/" + betaTask, "/api/teams/" + betaTeam} {
		if rec := do(e, "GET", path, "", admin.session); rec.Code != http.StatusOK {
			t.Errorf("admin GET %s = %d, want 200 (admins are in every project)", path, rec.Code)
		}
	}

	// As páginas: a do Beta e a da tarefa dele são 404 para a Bia; as do Alfa abrem. A primeira tela abre para todos.
	// (A raiz do projeto redireciona para a visão geral: 303 é abrir, 404 é recusar.)
	for path, want := range map[string]int{
		"/projects/" + alfa + "/overview":      http.StatusOK,
		"/projects/" + alfa + "/tasks":         http.StatusOK,
		"/tasks/" + alfaTask:                   http.StatusOK,
		"/projects/" + beta:                    http.StatusNotFound,
		"/projects/" + beta + "/tasks":         http.StatusNotFound,
		"/projects/" + beta + "/collaborators": http.StatusNotFound,
		"/tasks/" + betaTask:                   http.StatusNotFound,
		"/orgs/" + admin.orgID:                 http.StatusOK,
	} {
		if rec := do(e, "GET", path, "", bia.session); rec.Code != want {
			t.Errorf("Bia page %s = %d, want %d", path, rec.Code, want)
		}
	}
	if rec := do(e, "GET", "/orgs/"+admin.orgID, "", caio.session); rec.Code != http.StatusOK {
		t.Errorf("Caio's first page = %d, want 200 even with no project", rec.Code)
	}
	if rec := do(e, "GET", "/projects/"+beta+"/overview", "", admin.session); rec.Code != http.StatusOK {
		t.Errorf("admin page of the Beta = %d, want 200", rec.Code)
	}

	// Tirada do Alfa, a Bia perde o acesso a ele na hora; posta de volta, recupera.
	if rec := do(e, "DELETE", "/api/projects/"+alfa+"/collaborators/"+bia.id, "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("remove Bia from the Alfa = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/projects/"+alfa, "", bia.session); rec.Code != http.StatusNotFound {
		t.Errorf("Bia GET the Alfa after being removed = %d, want 404", rec.Code)
	}
	if got := projectNames(t, decodeList(t, do(e, "GET", list, "", bia.session))); !slices.Equal(got, []string{"Projeto Gama"}) {
		t.Errorf("Bia's projects after leaving the Alfa = %v, want only the Gama", got)
	}
	allocate(t, e, admin, alfa, bia.id, 2000)
	if rec := do(e, "GET", "/api/projects/"+alfa, "", bia.session); rec.Code != http.StatusOK {
		t.Errorf("Bia GET the Alfa after being added again = %d, want 200", rec.Code)
	}
}

// Quem convida para a organização e não põe a pessoa em projeto nenhum não lhe dá projeto nenhum: o convite com
// projeto, ao ser aceito, já a põe nele, e só nele.
func TestProjectAccess_InviteWithProjectOpensOnlyThatProject(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	createEmptyProject(t, e, admin, "Projeto Beta")

	rec := do(e, "POST", "/api/projects/"+alfa+"/invites", `{"email":"dani@test.com","pay_rate_cents":2500}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("project invite = %d: %s", rec.Code, rec.Body.String())
	}
	token := decode(t, rec)["token"].(string)
	rec = do(e, "POST", "/api/auth/invites/"+token+"/accept", `{"name":"Dani","email":"dani@test.com","password":"senha-forte-2"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept = %d: %s", rec.Code, rec.Body.String())
	}
	dani := sessionFrom(t, rec)

	got := projectNames(t, decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/projects", "", dani)))
	if !slices.Equal(got, []string{"Projeto Alfa"}) {
		t.Errorf("Dani's projects = %v, want only the one the invite carried", got)
	}
}

// Um membro que a organização deixou criar projetos não perde de vista o que acabou de criar: entra nele, e só
// nele (os outros projetos continuam fora da lista dele).
func TestProjectAccess_MemberWhoCreatesAProjectStaysInIt(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	createEmptyProject(t, e, admin, "Projeto da Ana")
	if rec := do(e, "PATCH", "/api/persons/"+bia.id+"/permissions", `{"permissions":["projects.create"]}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("grant projects.create = %d: %s", rec.Code, rec.Body.String())
	}

	mine := createProject(t, e, bia, "Projeto da Bia")
	list := "/api/orgs/" + admin.orgID + "/projects"
	if got := projectNames(t, decodeList(t, do(e, "GET", list, "", bia.session))); !slices.Equal(got, []string{"Projeto da Bia"}) {
		t.Errorf("Bia's projects = %v, want only the one she created", got)
	}
	if rec := do(e, "GET", "/api/projects/"+mine, "", bia.session); rec.Code != http.StatusOK {
		t.Errorf("Bia GET her own new project = %d, want 200", rec.Code)
	}
	// Criar um projeto não faz dela admin de coisa nenhuma: ela continua sem ver os dos outros.
	if got := projectNames(t, decodeList(t, do(e, "GET", list, "", admin.session))); len(got) != 2 {
		t.Errorf("admin's projects = %v, want both", got)
	}
}

// O painel da primeira tela não lista a tarefa de um projeto de que a pessoa saiu: ela não conseguiria abri-la.
// O admin, que está em todos, as vê.
func TestProjectAccess_DashboardOnlyCountsTasksOfMyProjects(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	beta := createEmptyProject(t, e, admin, "Projeto Beta")
	allocate(t, e, admin, alfa, bia.id, 2000)
	allocate(t, e, admin, beta, bia.id, 2000)
	for project, name := range map[string]string{alfa: "Tarefa do Alfa", beta: "Tarefa do Beta"} {
		for _, assignee := range []string{bia.id, admin.id} {
			body := `{"name":"` + name + `","assignee_id":"` + assignee + `"}`
			if rec := do(e, "POST", "/api/projects/"+project+"/tasks", body, admin.session); rec.Code != http.StatusCreated {
				t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
			}
		}
	}
	// A Bia sai do Beta, mas as tarefas dela lá continuam atribuídas a ela.
	if rec := do(e, "DELETE", "/api/projects/"+beta+"/collaborators/"+bia.id, "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("remove Bia from the Beta = %d: %s", rec.Code, rec.Body.String())
	}

	base := "/api/orgs/" + admin.orgID + "/me/"
	bodyOf := func(path, session string) map[string]any {
		t.Helper()
		rec := do(e, "GET", base+path, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}
	taskNames := func(session string) []string {
		var out []string
		for _, it := range bodyOf("tasks", session)["items"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		slices.Sort(out)
		return out
	}

	if got := taskNames(bia.session); !slices.Equal(got, []string{"Tarefa do Alfa"}) {
		t.Errorf("Bia's tasks = %v, want only the one of the project she is in", got)
	}
	if tasks := bodyOf("overview", bia.session)["tasks"].(map[string]any); tasks["total"] != float64(1) || tasks["open"] != float64(1) {
		t.Errorf("Bia's counts = %v, want one task", tasks)
	}
	if got := taskNames(admin.session); !slices.Equal(got, []string{"Tarefa do Alfa", "Tarefa do Beta"}) {
		t.Errorf("admin's tasks = %v, want both (admins are in every project)", got)
	}
	if tasks := bodyOf("overview", admin.session)["tasks"].(map[string]any); tasks["total"] != float64(2) {
		t.Errorf("admin's counts = %v, want two tasks", tasks)
	}
}
