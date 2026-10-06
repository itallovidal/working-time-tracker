package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// A visão geral soma o dinheiro do projeto, então a rota é só de admins: o
// membro recebe 403, e outra organização, 404, como se o projeto não existisse.
func TestOverview_AdminOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	prj := "/api/projects/" + projectID

	// A Bia entra no projeto (o valor antes do time), ganha uma tarefa e bate o ponto.
	if rec := do(e, "PUT", prj+"/billing", `{"bill_rate_cents":10000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	allocate(t, e, admin, projectID, bia.id, 2000)
	teamID := decode(t, do(e, "POST", prj+"/teams", `{"name":"Time"}`, admin.session))["id"].(string)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+bia.id+`"}`, admin.session)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Tarefa","assignee_id":"`+bia.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, bia.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	do(e, "POST", prj+"/work-sessions/clock-out", `{}`, bia.session)

	rec := do(e, "GET", prj+"/overview", "", admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET overview = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	section := func(key string) map[string]any {
		t.Helper()
		m, ok := body[key].(map[string]any)
		if !ok {
			t.Fatalf("overview has no %q object: %s", key, rec.Body.String())
		}
		return m
	}
	for key, want := range map[string]map[string]any{
		"project": {"name": "Projeto Alfa", "bill_rate_cents": float64(10000), "customer": nil},
		// O admin não está no projeto: só a Bia conta.
		"people": {"total": float64(1), "without_team": float64(0), "without_rate": float64(0), "working_now": float64(0)},
		"teams":  {"total": float64(1)},
		"tasks":  {"total": float64(1), "overdue": float64(0)},
		"time":   {"session_count": float64(1)},
	} {
		got := section(key)
		for field, v := range want {
			if got[field] != v {
				t.Errorf("%s.%s = %v, want %v", key, field, got[field], v)
			}
		}
	}
	if _, ok := section("project")["age"].(map[string]any); !ok {
		t.Errorf("project has no age: %s", rec.Body.String())
	}
	if _, ok := body["generated_at"].(string); !ok {
		t.Errorf("overview has no generated_at: %s", rec.Body.String())
	}
	// A sessão tem os dois valores, então custo, receita e margem saem preenchidos.
	for _, field := range []string{"pay_amount_cents", "bill_amount_cents", "margin_cents"} {
		if section("money")[field] == nil {
			t.Errorf("money.%s is null, and the session has both rates", field)
		}
	}
	rows, _ := body["by_person"].([]any)
	if len(rows) != 1 {
		t.Fatalf("by_person = %v, want one row for Bia", body["by_person"])
	}
	if row := rows[0].(map[string]any); row["person"].(map[string]any)["id"] != bia.id || row["in_project"] != true || row["session_count"] != float64(1) {
		t.Errorf("by_person[0] = %v, want Bia, in the project, with one session", row)
	}
	// Sem integração a lista vem vazia, não null.
	if items, ok := section("integrations")["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("integrations.items = %v, want an empty list", section("integrations")["items"])
	}

	if rec := do(e, "GET", prj+"/overview", "", bia.session); rec.Code != http.StatusForbidden {
		t.Errorf("member GET overview = %d, want 403", rec.Code)
	}
	if rec := do(e, "GET", prj+"/overview", "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another org GET overview = %d, want 404", rec.Code)
	}
	if rec := do(e, "GET", prj+"/overview", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET overview without session = %d, want 401", rec.Code)
	}
	if rec := do(e, "GET", "/api/projects/not-a-uuid/overview", "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("GET overview of a malformed id = %d, want 404", rec.Code)
	}
}

// A aba Visão geral é a primeira do projeto e só existe para admins: o membro
// não vê o link em aba nenhuma e recebe "Página não encontrada" no endereço.
func TestPages_ProjectOverviewTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	overview := "/projects/" + projectID + "/overview"
	link := `href="` + overview + `"`

	rec := do(e, "GET", overview, "", admin.session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("admin GET %s = %d %s, want 200 html", overview, rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, link+` aria-current="page"`) || !strings.Contains(body, " Visão geral</a>") {
		t.Error("the overview page does not mark Visão geral as the current tab")
	}
	first, tasks := strings.Index(body, link), strings.Index(body, `href="/projects/`+projectID+`/tasks"`)
	if first < 0 || tasks < 0 || first > tasks {
		t.Errorf("the Visão geral tab is not the first one (at %d, Tarefas at %d)", first, tasks)
	}
	for _, want := range []string{
		"Projeto Alfa", `x-data="projectOverview"`, "Atualizar",
		"Horas registradas", `<div class="k">Receita</div>`, `<div class="k">Custo</div>`, "Margem (receita menos custo)",
		"<h2>Tempo de projeto</h2>", "<h2>Atividade</h2>", "<h2>Integrações</h2>", "<h2>Tarefas e cliente</h2>", "<h2>Horas por pessoa</h2>",
		`href="/projects/` + projectID + `/tasks?due=overdue"`, `class="pager"`,
		// Os tipos de integração vão no BOOT, para a lista mostrar o nome da plataforma.
		`"integration_types"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the overview page does not contain %q", want)
		}
	}
	if n := strings.Count(body, `id="modal-root"`); n != 1 {
		t.Errorf("the overview page has %d modal hosts, want 1", n)
	}
	if !strings.Contains(body, "/font-awesome/") || !strings.Contains(body, `integrity="sha512-`) {
		t.Error("the overview page does not load the icon font with SRI")
	}

	// Nas outras abas, o admin tem o link para chegar aqui, e o membro não.
	for _, path := range pagePaths(admin.orgID, projectID) {
		if !strings.HasPrefix(path, "/projects/") {
			continue
		}
		asAdmin := do(e, "GET", path, "", admin.session).Body.String()
		if !strings.Contains(asAdmin, link) || strings.Contains(asAdmin, link+` aria-current="page"`) {
			t.Errorf("admin GET %s does not link to the overview tab, or marks it as current", path)
		}
		if asMember := do(e, "GET", path, "", member.session).Body.String(); strings.Contains(asMember, link) || strings.Contains(asMember, "Visão geral") {
			t.Errorf("member GET %s shows the overview tab", path)
		}
	}

	if rec := do(e, "GET", overview, "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("member GET %s = %d, want 404", overview, rec.Code)
	}
	if rec := do(e, "GET", overview, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another org GET %s = %d, want 404", overview, rec.Code)
	}
	if rec := do(e, "GET", overview, "", ""); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
		t.Errorf("GET %s without session = %d to %q, want 303 to /login", overview, rec.Code, rec.Header().Get("Location"))
	}
}

// O projeto abre na Visão geral para o admin e nas Tarefas para o membro, que
// não tem a primeira aba.
func TestPages_ProjectOpensOnTheRightTab(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")

	for who, tc := range map[string]struct{ session, tab string }{
		"admin":  {admin.session, "overview"},
		"member": {member.session, "tasks"},
	} {
		rec := do(e, "GET", "/projects/"+projectID, "", tc.session)
		want := "/projects/" + projectID + "/" + tc.tab
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("%s GET /projects/:id = %d to %q, want 303 to %s", who, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}
