package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// A escada de cargos de ponta a ponta: cada linha é uma rota, e cada coluna, quem a chama. Os quatro cargos são o dono,
// o admin (que não é o dono), o administrador de projeto (o grupo manager, num projeto) e o colaborador. O que a tabela
// de _docs/permissions.md diz de cada capacidade está aqui, célula a célula: 200/201/204 passa, 403 é recusa por
// permissão e 404 é página ou recurso que não existe para quem pede.
func TestRoles_Matrix(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	manager := invite(t, e, owner, "isabela@test.com", "member")
	plain := invite(t, e, owner, "bruno@test.com", "member")
	org := "/api/orgs/" + owner.orgID

	alfa := createEmptyProject(t, e, owner, "Alfa")
	withPreset(t, e, owner, alfa, manager.id, "manager")
	allocate(t, e, owner, alfa, plain.id, 2000)
	prj := "/api/projects/" + alfa
	customerID := decode(t, do(e, "POST", org+"/customers", `{"name":"Cliente"}`, owner.session))["id"].(string)

	type who struct {
		name    string
		session string
	}
	people := []who{{"owner", owner.session}, {"admin", admin.session}, {"manager", manager.session}, {"plain", plain.session}}
	// ok é "qualquer 2xx"; a ordem das colunas é owner, admin, manager e plain.
	const ok = 0
	rows := []struct {
		name, method, path, body string
		want                     [4]int
	}{
		// Organização e pessoas.
		{"read the organization", "GET", org, "", [4]int{ok, ok, ok, ok}},
		{"edit the organization", "PATCH", org, `{"summary":"X"}`, [4]int{ok, 403, 403, 403}},
		{"list the people", "GET", org + "/persons", "", [4]int{ok, ok, ok, ok}},
		{"list customers", "GET", org + "/customers", "", [4]int{ok, 403, 403, 403}},
		{"create a customer", "POST", org + "/customers", `{"name":"Outro"}`, [4]int{ok, 403, 403, 403}},
		{"the team payments", "GET", org + "/payments", "", [4]int{ok, 403, 403, 403}},
		{"the organization overview", "GET", org + "/overview", "", [4]int{ok, ok, 403, 403}},
		{"set the weekly hours", "PATCH", "/api/persons/" + plain.id + "/weekly-hours", `{"weekly_hours":30}`, [4]int{ok, ok, 403, 403}},
		{"set a payment rule", "PATCH", "/api/persons/" + plain.id + "/payment", `{"frequency":"monthly","day":5}`, [4]int{ok, 403, 403, 403}},
		{"the payments of the owner", "GET", "/api/persons/" + owner.id + "/payments", "", [4]int{ok, ok, 403, 403}},
		{"invite a member to the organization", "POST", org + "/invites", `{"role":"member"}`, [4]int{ok, ok, 403, 403}},
		{"invite an admin", "POST", org + "/invites", `{"role":"admin"}`, [4]int{ok, 403, 403, 403}},
		{"change a role", "PATCH", "/api/persons/" + plain.id + "/role", `{"role":"member"}`, [4]int{ok, 403, 403, 403}},

		// Projetos.
		{"create an internal project", "POST", org + "/projects", `{"name":"Interno"}`, [4]int{ok, ok, 403, 403}},
		{"create a project with a customer", "POST", org + "/projects", `{"name":"Cliente","customer_id":"` + customerID + `","bill_rate_cents":10000}`, [4]int{ok, 403, 403, 403}},
		{"read the billing", "GET", prj + "/billing", "", [4]int{ok, 403, 403, 403}},
		{"set the billing", "PUT", prj + "/billing", `{"customer_id":"` + customerID + `","bill_rate_cents":10000}`, [4]int{ok, 403, 403, 403}},
		{"the project overview", "GET", prj + "/overview", "", [4]int{ok, ok, ok, 403}},
		{"edit the project", "PATCH", prj, `{"name":"Alfa 2"}`, [4]int{ok, ok, ok, 403}},
		{"create a team", "POST", prj + "/teams", `{"name":"Time"}`, [4]int{ok, ok, ok, 403}},
		{"read every allocation", "GET", prj + "/allocations", "", [4]int{ok, ok, ok, ok}},
		{"put someone on the project", "PUT", prj + "/allocations/" + admin.id, `{"pay_rate_cents":3000}`, [4]int{ok, ok, ok, 403}},
		{"invite to the project", "POST", prj + "/invites", `{"email":"novo@test.com","pay_rate_cents":5000}`, [4]int{201, 201, 201, 403}},
	}
	for _, r := range rows {
		for i, p := range people {
			got := status(e, r.method, r.path, r.body, p.session)
			if r.want[i] == ok {
				if got < 200 || got > 299 {
					t.Errorf("%s: %s got %d, want 2xx", r.name, p.name, got)
				}
				continue
			}
			if got != r.want[i] {
				t.Errorf("%s: %s got %d, want %d", r.name, p.name, got, r.want[i])
			}
		}
	}

	// A exclusão do projeto vai por último: só o dono e o admin excluem.
	for _, c := range []struct {
		name    string
		session string
		want    int
	}{{"plain", plain.session, 403}, {"manager", manager.session, 403}, {"admin", admin.session, 204}} {
		target := createEmptyProject(t, e, owner, "Para excluir "+c.name)
		withPreset(t, e, owner, target, manager.id, "manager")
		allocate(t, e, owner, target, plain.id, 2000)
		if got := status(e, "DELETE", "/api/projects/"+target, "", c.session); got != c.want {
			t.Errorf("delete the project: %s got %d, want %d", c.name, got, c.want)
		}
	}
	target := createEmptyProject(t, e, owner, "Do dono")
	if got := status(e, "DELETE", "/api/projects/"+target, "", owner.session); got != http.StatusNoContent {
		t.Errorf("delete the project: owner got %d, want 204", got)
	}
}

// As páginas do navegador seguem o mesmo cargo: o que não é do cargo responde 404.
func TestRoles_Pages(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	manager := invite(t, e, owner, "isabela@test.com", "member")
	plain := invite(t, e, owner, "bruno@test.com", "member")
	alfa := createEmptyProject(t, e, owner, "Alfa")
	withPreset(t, e, owner, alfa, manager.id, "manager")
	allocate(t, e, owner, alfa, plain.id, 2000)
	home := "/orgs/" + owner.orgID

	for _, r := range []struct {
		name, path string
		want       [4]int // owner, admin, manager, plain
	}{
		{"settings (edit the organization)", home + "/settings", [4]int{200, 404, 404, 404}},
		{"the team payments", home + "/payments", [4]int{200, 404, 404, 404}},
		{"customers", home + "/customers", [4]int{200, 404, 404, 404}},
		{"collaborators", home + "/people", [4]int{200, 200, 404, 404}},
		{"a collaborator's profile", home + "/people/" + plain.id, [4]int{200, 200, 404, 404}},
		{"the project overview in the management", "/projects/" + alfa + "/management/overview", [4]int{200, 200, 200, 404}},
		{"the project settings in the management", "/projects/" + alfa + "/management/settings", [4]int{200, 200, 200, 404}},
	} {
		for i, p := range []struct{ name, session string }{{"owner", owner.session}, {"admin", admin.session}, {"manager", manager.session}, {"plain", plain.session}} {
			if got := status(e, "GET", r.path, "", p.session); got != r.want[i] {
				t.Errorf("%s: %s got %d, want %d", r.name, p.name, got, r.want[i])
			}
		}
	}
}

// A cobrança é do dono: a visão geral da organização e a do projeto chegam ao admin e ao administrador de projeto
// sem a receita, a margem, o cliente nem o valor cobrado; o custo e as horas ficam.
func TestRoles_OverviewsHideTheBilling(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	manager := invite(t, e, owner, "isabela@test.com", "member")
	bia := invite(t, e, owner, "bia@test.com", "member")
	alfa := createEmptyProject(t, e, owner, "Alfa")
	prj := "/api/projects/" + alfa
	setBilling(t, e, owner, prj, 10000)
	withPreset(t, e, owner, alfa, manager.id, "manager")
	allocate(t, e, owner, alfa, bia.id, 2000)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"T","assignee_id":"`+bia.id+`"}`, owner.session))["id"].(string)
	if rec := do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, bia.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	do(e, "POST", prj+"/work-sessions/clock-out", `{}`, bia.session)

	money := func(body map[string]any) map[string]any {
		t.Helper()
		m, _ := body["money"].(map[string]any)
		return m
	}
	// A do projeto: o dono vê tudo; o administrador de projeto, o custo e não a cobrança.
	full := decode(t, do(e, "GET", prj+"/overview", "", owner.session))
	if m := money(full); m["bill_amount_cents"] == nil || m["margin_cents"] == nil {
		t.Fatalf("the owner's project overview has no billing: %v", m)
	}
	if p, _ := full["project"].(map[string]any); p["bill_rate_cents"] == nil || p["customer"] == nil {
		t.Errorf("the owner's project overview has no customer or billed rate: %v", p)
	}
	for name, who := range map[string]account{"admin": admin, "manager": manager} {
		got := decode(t, do(e, "GET", prj+"/overview", "", who.session))
		m := money(got)
		if m["bill_amount_cents"] != nil || m["margin_cents"] != nil || m["pay_amount_cents"] == nil {
			t.Errorf("%s: project money = %v, want the cost and no billing", name, m)
		}
		if p, _ := got["project"].(map[string]any); p["bill_rate_cents"] != nil || p["customer"] != nil {
			t.Errorf("%s: project = %v, want no customer or billed rate", name, p)
		}
		for _, person := range got["by_person"].([]any) {
			if person.(map[string]any)["bill_amount_cents"] != nil {
				t.Errorf("%s: a person row has a billed amount: %v", name, person)
			}
		}
	}
	// A da organização: o admin recebe as horas e o custo, sem a receita nem a margem.
	orgFull := decode(t, do(e, "GET", "/api/orgs/"+owner.orgID+"/overview", "", owner.session))
	allTime := func(body map[string]any) map[string]any {
		return body["periods"].(map[string]any)["all_time"].(map[string]any)["money"].(map[string]any)
	}
	if m := allTime(orgFull); m["bill_amount_cents"] == nil || m["margin_cents"] == nil {
		t.Fatalf("the owner's organization overview has no billing: %v", m)
	}
	orgAdmin := decode(t, do(e, "GET", "/api/orgs/"+owner.orgID+"/overview", "", admin.session))
	if m := allTime(orgAdmin); m["bill_amount_cents"] != nil || m["margin_cents"] != nil || m["pay_amount_cents"] == nil {
		t.Errorf("the admin's organization overview money = %v, want the cost and no billing", m)
	}
	if body := do(e, "GET", "/api/orgs/"+owner.orgID+"/overview", "", admin.session).Body.String(); strings.Contains(body, `"bill_amount_cents":1`) {
		t.Errorf("the admin's organization overview leaks a billed amount: %s", body)
	}
}
