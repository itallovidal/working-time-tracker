package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A visão geral da organização soma o dinheiro de todos os projetos e o tempo de cada pessoa,
// então a rota é só de admins: o membro recebe 403, outra organização, 404, e sem sessão, 401.
func TestOrgOverview_AdminOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	createEmptyProject(t, e, admin, "Projeto Beta")
	prj := "/api/projects/" + projectID

	// A Bia bate o ponto no Projeto Alfa, que cobra 100,00 por hora e paga 20,00 a ela.
	if rec := do(e, "PUT", prj+"/billing", `{"bill_rate_cents":10000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	allocate(t, e, admin, projectID, bia.id, 2000)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Tarefa","assignee_id":"`+bia.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, bia.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}

	path := "/api/orgs/" + admin.orgID + "/overview"
	rec := do(e, "GET", path, "", admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	section := func(from map[string]any, key string) map[string]any {
		t.Helper()
		m, ok := from[key].(map[string]any)
		if !ok {
			t.Fatalf("%s has no %q object: %s", path, key, rec.Body.String())
		}
		return m
	}
	// A Bia está com o ponto aberto, e o dono e ela são as duas pessoas; os dois projetos contam.
	for key, want := range map[string]map[string]any{
		"people":   {"total": float64(2), "working_now": float64(1)},
		"projects": {"total": float64(2)},
	} {
		got := section(body, key)
		for field, v := range want {
			if got[field] != v {
				t.Errorf("%s.%s = %v, want %v", key, field, got[field], v)
			}
		}
	}
	if _, ok := body["generated_at"].(string); !ok {
		t.Errorf("overview has no generated_at: %s", rec.Body.String())
	}
	periods := section(body, "periods")
	for _, name := range []string{"last_7_days", "last_30_days", "all_time"} {
		p := section(periods, name)
		if p["session_count"] != float64(1) {
			t.Errorf("periods.%s.session_count = %v, want 1", name, p["session_count"])
		}
		// A sessão tem os dois valores, então custo, receita e margem saem preenchidos.
		money := section(p, "money")
		for _, field := range []string{"pay_amount_cents", "bill_amount_cents", "margin_cents"} {
			if money[field] == nil {
				t.Errorf("periods.%s.money.%s is null, and the session has both rates", name, field)
			}
		}
		// O dono não bateu ponto: o tempo dele é zero, e o da Bia, não.
		if p["my_seconds"] != float64(0) {
			t.Errorf("periods.%s.my_seconds = %v, want 0 for the owner", name, p["my_seconds"])
		}
	}
	rows, _ := body["by_person"].([]any)
	if len(rows) != 2 {
		t.Fatalf("by_person = %v, want the owner and Bia", body["by_person"])
	}
	if first := rows[0].(map[string]any); first["person"].(map[string]any)["id"] != bia.id || first["working_now"] != true {
		t.Errorf("by_person[0] = %v, want Bia, working now (she has the time)", first)
	}
	if second := rows[1].(map[string]any); second["person"].(map[string]any)["id"] != admin.id || second["total_seconds"] != float64(0) {
		t.Errorf("by_person[1] = %v, want the owner with no time", second)
	}
	// A tela mostra no que cada um trabalha agora: a Bia, na tarefa e no projeto em que bateu o
	// ponto (com id e nome, para o link); o dono, que está sem ponto, com a lista vazia e não nula.
	workingOn, _ := rows[0].(map[string]any)["working_on"].([]any)
	if len(workingOn) != 1 {
		t.Fatalf("Bia's working_on = %v, want her one task", rows[0].(map[string]any)["working_on"])
	}
	on := workingOn[0].(map[string]any)
	if task, project := section(on, "task"), section(on, "project"); task["id"] != taskID || task["name"] != "Tarefa" || project["id"] != projectID || project["name"] != "Projeto Alfa" {
		t.Errorf("Bia's working_on = %v, want the task %s (Tarefa) of the project %s (Projeto Alfa)", on, taskID, projectID)
	}
	if ownerOn, ok := rows[1].(map[string]any)["working_on"].([]any); !ok || len(ownerOn) != 0 {
		t.Errorf("the owner's working_on = %v, want an empty list (not null)", rows[1].(map[string]any)["working_on"])
	}

	// Quem pede é quem tem o "meu tempo": a Bia não abre a rota, mas o dono vê o dele depois de bater o ponto.
	if rec := do(e, "GET", path, "", bia.session); rec.Code != http.StatusForbidden {
		t.Errorf("member GET %s = %d, want 403", path, rec.Code)
	}
	if rec := do(e, "GET", path, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another org GET %s = %d, want 404", path, rec.Code)
	}
	if rec := do(e, "GET", path, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET %s without session = %d, want 401", path, rec.Code)
	}
	if rec := do(e, "GET", "/api/orgs/not-a-uuid/overview", "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("GET overview of a malformed org id = %d, want 404", rec.Code)
	}
}

// A lista de quem trabalha agora é a da visão geral sem o tempo e sem o dinheiro: só quem está com o ponto
// aberto, em qualquer projeto, e as tarefas em que está. Quem pede é admin (o membro recebe 403, outra
// organização 404 e sem sessão 401), e a lista nunca vem nula.
func TestOrgWorkingNow_AdminOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	projectID := createEmptyProject(t, e, admin, "Projeto Alfa")
	prj := "/api/projects/" + projectID
	path := "/api/orgs/" + admin.orgID + "/working-now"

	// Ninguém bateu o ponto: a lista vem vazia, e não nula.
	rec := do(e, "GET", path, "", admin.session)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("admin GET %s with nobody working = %d %q, want 200 []", path, rec.Code, rec.Body.String())
	}

	allocate(t, e, admin, projectID, bia.id, 2000)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Tarefa","assignee_id":"`+bia.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, bia.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(e, "GET", path, "", admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	var list []struct {
		PersonID  string `json:"person_id"`
		WorkingOn []struct {
			Task    struct{ ID, Name string }
			Project struct{ ID, Name string }
		} `json:"working_on"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode %s: %v: %s", path, err, rec.Body.String())
	}
	if len(list) != 1 || list[0].PersonID != bia.id || len(list[0].WorkingOn) != 1 {
		t.Fatalf("working now = %s, want only Bia, on her one task", rec.Body.String())
	}
	if on := list[0].WorkingOn[0]; on.Task.ID != taskID || on.Task.Name != "Tarefa" || on.Project.ID != projectID || on.Project.Name != "Projeto Alfa" {
		t.Errorf("Bia's working_on = %+v, want the task %s (Tarefa) of the project %s (Projeto Alfa)", on, taskID, projectID)
	}
	// A lista não leva tempo nem dinheiro.
	for _, leak := range []string{"seconds", "cents", "email"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("working now leaks %q: %s", leak, rec.Body.String())
		}
	}

	if rec := do(e, "GET", path, "", bia.session); rec.Code != http.StatusForbidden {
		t.Errorf("member GET %s = %d, want 403", path, rec.Code)
	}
	if rec := do(e, "GET", path, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another org GET %s = %d, want 404", path, rec.Code)
	}
	if rec := do(e, "GET", path, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET %s without session = %d, want 401", path, rec.Code)
	}
	if rec := do(e, "GET", "/api/orgs/not-a-uuid/working-now", "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("GET working-now of a malformed org id = %d, want 404", rec.Code)
	}
}

// A lista de projetos da organização: sem page vem inteira num array (como sempre veio);
// com page vem uma página com o total, do mais novo para o mais antigo, sem repetir nem
// pular projeto entre as páginas.
func TestProjects_ListPages(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	other := signup(t, e, "Outra", "caio@outra.com")
	for i := 1; i <= 5; i++ {
		createProject(t, e, admin, fmt.Sprintf("Projeto %d", i))
	}
	createProject(t, e, other, "Projeto de outra organização")
	list := "/api/orgs/" + admin.orgID + "/projects"

	// Sem page, o array inteiro de sempre.
	if all := decodeList(t, do(e, "GET", list, "", admin.session)); len(all) != 5 {
		t.Fatalf("GET without page = %d projects, want the 5 of the organization", len(all))
	}

	type page struct {
		Items   []string
		Total   int
		Page    int
		PerPage int
	}
	get := func(query, session string) page {
		t.Helper()
		rec := do(e, "GET", list+query, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", query, rec.Code, rec.Body.String())
		}
		body := decode(t, rec)
		p := page{Total: int(body["total"].(float64)), Page: int(body["page"].(float64)), PerPage: int(body["per_page"].(float64))}
		items, ok := body["items"].([]any)
		if !ok {
			t.Fatalf("GET %s has no items list: %s", query, rec.Body.String())
		}
		for _, it := range items {
			p.Items = append(p.Items, it.(map[string]any)["name"].(string))
		}
		return p
	}

	// Três páginas de dois: os mais novos primeiro, e o conjunto fecha nos cinco.
	var seen []string
	for n, want := range map[int]int{1: 2, 2: 2, 3: 1} {
		p := get(fmt.Sprintf("?page=%d&per_page=2", n), admin.session)
		if p.Total != 5 || p.Page != n || p.PerPage != 2 || len(p.Items) != want {
			t.Errorf("page %d = %+v, want total 5, page %d of 2, %d items", n, p, n, want)
		}
		seen = append(seen, p.Items...)
	}
	if len(seen) != 5 {
		t.Errorf("the three pages hold %d projects, want 5", len(seen))
	}
	dup := map[string]bool{}
	for _, name := range seen {
		if dup[name] {
			t.Errorf("project %q appears on two pages", name)
		}
		dup[name] = true
	}
	if first := get("?page=1&per_page=2", admin.session); strings.Join(first.Items, ",") != "Projeto 5,Projeto 4" {
		t.Errorf("first page = %v, want the newest two (Projeto 5, Projeto 4)", first.Items)
	}

	// Além da última volta a última; per_page acima do teto vale o teto; o padrão é 10.
	if p := get("?page=9&per_page=2", admin.session); p.Page != 3 || len(p.Items) != 1 {
		t.Errorf("page 9 = %+v, want the last page (3) with its one project", p)
	}
	if p := get("?page=1&per_page=1000", admin.session); p.PerPage != 100 || len(p.Items) != 5 {
		t.Errorf("per_page=1000 = %+v, want a page of 100 with all 5", p)
	}
	if p := get("?page=1", admin.session); p.PerPage != 10 {
		t.Errorf("page=1 alone = %+v, want the default of 10 per page", p)
	}
	// Todo membro lista os projetos; a lista é da organização dele.
	if p := get("?page=1&per_page=2", bia.session); p.Total != 5 {
		t.Errorf("member page = %+v, want the 5 projects of the organization", p)
	}
	if rec := do(e, "GET", list+"?page=1", "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("another org GET %s = %d, want 404", list, rec.Code)
	}

	for query, code := range map[string]string{
		"?page=0":                         "project.invalid_page",
		"?page=-1":                        "project.invalid_page",
		"?page=abc":                       "project.invalid_page",
		"?page=1&per_page=0":              "project.invalid_per_page",
		"?page=1&per_page=x":              "project.invalid_per_page",
		"?per_page=-3":                    "project.invalid_per_page",
		"?page=" + url.QueryEscape("1.5"): "project.invalid_page",
	} {
		rec := do(e, "GET", list+query, "", admin.session)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", query, rec.Code)
			continue
		}
		if got := decode(t, rec)["error"].(map[string]any)["code"]; got != code {
			t.Errorf("GET %s error = %v, want %s", query, got, code)
		}
	}
}
