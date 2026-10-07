package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// putAllocation faz o PUT do vínculo como quem chama, devolvendo a resposta.
func putAllocation(e *echo.Echo, by account, projectID, personID, body string) (int, string) {
	rec := do(e, "PUT", "/api/projects/"+projectID+"/allocations/"+personID, body, by.session)
	return rec.Code, rec.Body.String()
}

// withPreset põe a pessoa no projeto, como o admin, já com o grupo.
func withPreset(t *testing.T, e *echo.Echo, admin account, projectID, personID, preset string) {
	t.Helper()
	body := fmt.Sprintf(`{"pay_rate_cents":3000,"preset":%q}`, preset)
	if code, out := putAllocation(e, admin, projectID, personID, body); code != http.StatusOK {
		t.Fatalf("give %s the %s preset = %d: %s", personID, preset, code, out)
	}
}

// status devolve só o código HTTP da requisição.
func status(e *echo.Echo, method, path, body, session string) int {
	return do(e, method, path, body, session).Code
}

// Quem não é admin faz o que o grupo dele no projeto libera, e nada além: o gerente cuida do
// projeto, o financeiro, do dinheiro, e o colaborador comum, de nada disso.
func TestPermissions_PresetsDecideWhatEachPersonCanDo(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	finance := invite(t, e, admin, "fabio@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	stranger := invite(t, e, admin, "zeca@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	other := createProject(t, e, admin, "Beta")
	base := "/api/projects/" + prj

	withPreset(t, e, admin, prj, manager.id, "manager")
	withPreset(t, e, admin, prj, finance.id, "finance")
	withPreset(t, e, admin, prj, plain.id, "member")
	// O gerente do Alfa é um colaborador comum no Beta.
	withPreset(t, e, admin, other, manager.id, "member")

	type call struct {
		name, method, path, body string
		allowed                  map[string]bool // quem passa: manager, finance, plain
	}
	only := func(who ...string) map[string]bool {
		m := map[string]bool{}
		for _, w := range who {
			m[w] = true
		}
		return m
	}
	calls := []call{
		{"edit the project", "PATCH", base, `{"name":"Alfa 2"}`, only("manager")},
		{"read the billing", "GET", base + "/billing", "", only("finance")},
		{"set the billing", "PUT", base + "/billing", `{"bill_rate_cents":9000}`, only("finance")},
		{"read the overview", "GET", base + "/overview", "", only("finance")},
		{"create a team", "POST", base + "/teams", `{"name":"Time"}`, only("manager")},
		{"create a label", "POST", base + "/labels", `{"name":"Bug"}`, only("manager")},
		{"create an integration", "POST", base + "/integrations", `{"type":"github","display_name":"GH"}`, only("manager")},
		{"read every allocation", "GET", base + "/allocations", "", only("manager", "finance")},
	}
	sessions := map[string]string{"manager": manager.session, "finance": finance.session, "plain": plain.session}
	for _, c := range calls {
		for who, session := range sessions {
			got := status(e, c.method, c.path, c.body, session)
			denied := got == http.StatusForbidden
			if c.allowed[who] && denied {
				t.Errorf("%s: %s was refused (%d), but its group allows it", c.name, who, got)
			}
			if !c.allowed[who] && !denied && c.name != "read every allocation" {
				t.Errorf("%s: %s got %d, want 403", c.name, who, got)
			}
		}
	}

	// A permissão é do projeto: no Beta o gerente é um colaborador comum.
	if got := status(e, "PATCH", "/api/projects/"+other, `{"name":"Beta 2"}`, manager.session); got != http.StatusForbidden {
		t.Errorf("a manager of Alfa editing Beta = %d, want 403", got)
	}
	// E quem nem está no projeto não passa em nada dele.
	for _, path := range []string{base + "/billing", base + "/overview"} {
		if got := status(e, "GET", path, "", stranger.session); got != http.StatusForbidden {
			t.Errorf("a person outside the project GET %s = %d, want 403", path, got)
		}
	}
}

// Ver o valor dos outros e o valor cobrado também seguem as permissões: o gerente vê o que
// os colegas recebem, mas não o que o cliente paga; o financeiro vê os dois.
func TestPermissions_ValuesFollowThePermissions(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	finance := invite(t, e, admin, "fabio@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	if code := status(e, "PUT", "/api/projects/"+prj+"/billing", `{"bill_rate_cents":9000}`, admin.session); code != http.StatusOK {
		t.Fatalf("set billing = %d", code)
	}
	withPreset(t, e, admin, prj, manager.id, "manager")
	withPreset(t, e, admin, prj, finance.id, "finance")
	withPreset(t, e, admin, prj, plain.id, "member")

	taskID := decode(t, do(e, "POST", "/api/projects/"+prj+"/tasks", `{"name":"T","assignee_id":"`+plain.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, plain.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	do(e, "POST", "/api/projects/"+prj+"/work-sessions/clock-out", `{}`, plain.session)

	sessionsOf := func(who account) []map[string]any {
		rec := do(e, "GET", "/api/projects/"+prj+"/work-sessions", "", who.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("sessions as %s = %d: %s", who.id, rec.Code, rec.Body.String())
		}
		return decodeList(t, rec)
	}
	// Quem vê o valor dos outros (gerente e financeiro) vê as horas de todos; o colaborador
	// comum, só as dele.
	for name, who := range map[string]account{"manager": manager, "finance": finance} {
		got := sessionsOf(who)
		if len(got) != 1 || got[0]["pay_rate_cents"] != float64(3000) {
			t.Errorf("%s sees %v, want the plain member's session with its pay rate", name, got)
		}
	}
	if got := sessionsOf(manager); got[0]["bill_rate_cents"] != nil {
		t.Errorf("the manager sees the billed rate %v, want it hidden", got[0]["bill_rate_cents"])
	}
	if got := sessionsOf(finance); got[0]["bill_rate_cents"] != float64(9000) {
		t.Errorf("the finance group sees the billed rate %v, want 9000", got[0]["bill_rate_cents"])
	}
	other := sessionsOf(plain)
	if len(other) != 1 || other[0]["pay_rate_cents"] != float64(3000) || other[0]["bill_rate_cents"] != nil {
		t.Errorf("the member sees %v, want only their own pay rate and no billed rate", other)
	}

	// Os valores dos colegas na lista de colaboradores.
	rates := func(who account) map[string]any {
		out := map[string]any{}
		for _, c := range decodeList(t, do(e, "GET", "/api/projects/"+prj+"/collaborators", "", who.session)) {
			out[c["person"].(map[string]any)["id"].(string)] = c["pay_rate_cents"]
		}
		return out
	}
	if got := rates(manager); got[plain.id] != float64(3000) || got[finance.id] != float64(3000) {
		t.Errorf("the manager sees the colleagues' rates as %v, want them", got)
	}
	if got := rates(plain); got[manager.id] != nil || got[plain.id] != float64(3000) {
		t.Errorf("the plain member sees %v, want only their own rate", got)
	}
}

// Dar um grupo exige as permissões do grupo: o gerente põe pessoas no projeto como
// colaboradoras, mas não dá o grupo financeiro nem o de administrador, que têm o que ele não tem.
func TestPermissions_NobodyGrantsWhatTheyLack(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	newcomer := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, manager.id, "manager")

	// O gerente tem collaborators.manage e rates.manage: põe e dá o valor.
	if code, out := putAllocation(e, manager, prj, newcomer.id, `{"pay_rate_cents":4000}`); code != http.StatusOK {
		t.Fatalf("a manager adding a person = %d: %s", code, out)
	}
	// Mas o grupo financeiro traz billing.manage, que ele não tem.
	for _, preset := range []string{"finance", "admin"} {
		code, out := putAllocation(e, manager, prj, newcomer.id, fmt.Sprintf(`{"preset":%q}`, preset))
		if code != http.StatusForbidden || !strings.Contains(out, "allocation.preset_above_yours") {
			t.Errorf("a manager giving the %s group = %d %s, want 403 allocation.preset_above_yours", preset, code, out)
		}
	}
	// Um grupo com o que ele já tem passa.
	if code, out := putAllocation(e, manager, prj, newcomer.id, `{"preset":"manager"}`); code != http.StatusOK {
		t.Errorf("a manager giving the manager group = %d: %s", code, out)
	}
	if code, out := putAllocation(e, manager, prj, newcomer.id, `{"preset":"nope"}`); code != http.StatusBadRequest || !strings.Contains(out, "allocation.invalid_preset") {
		t.Errorf("an unknown group = %d %s, want 400 allocation.invalid_preset", code, out)
	}
	// Quem recebeu o grupo de gerente agora gerencia.
	if got := status(e, "POST", "/api/projects/"+prj+"/teams", `{"name":"Novo"}`, newcomer.session); got != http.StatusCreated {
		t.Errorf("the new manager creating a team = %d, want 201", got)
	}
	// O grupo é de cada projeto: tirar a pessoa do projeto tira o que ela podia nele.
	if got := status(e, "DELETE", "/api/projects/"+prj+"/collaborators/"+newcomer.id, "", manager.session); got != http.StatusNoContent {
		t.Fatalf("removing the new manager = %d, want 204", got)
	}
	if got := status(e, "POST", "/api/projects/"+prj+"/teams", `{"name":"Outro"}`, newcomer.session); got != http.StatusForbidden {
		t.Errorf("a person removed from the project creating a team = %d, want 403", got)
	}
}

// Sem permissão de criar valor ou de pôr pessoas, o PUT é recusado, e quem define o valor mas
// não põe gente também não escolhe grupos.
func TestPermissions_AllocationNeedsTheRightPermissionForEachChange(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	finance := invite(t, e, admin, "fabio@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	newcomer := invite(t, e, admin, "bia@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, finance.id, "finance")
	withPreset(t, e, admin, prj, plain.id, "member")

	// O financeiro troca o valor de quem já está no projeto, mas não põe ninguém nem muda grupo.
	if code, out := putAllocation(e, finance, prj, plain.id, `{"pay_rate_cents":5000}`); code != http.StatusOK {
		t.Errorf("finance changing a rate = %d: %s", code, out)
	}
	if code, _ := putAllocation(e, finance, prj, newcomer.id, `{"pay_rate_cents":5000}`); code != http.StatusForbidden {
		t.Errorf("finance adding a person = %d, want 403", code)
	}
	if code, _ := putAllocation(e, finance, prj, plain.id, `{"preset":"manager"}`); code != http.StatusForbidden {
		t.Errorf("finance changing a group = %d, want 403", code)
	}
	// O colaborador comum não muda nada, nem o próprio valor.
	if code, _ := putAllocation(e, plain, prj, plain.id, `{"pay_rate_cents":9000}`); code != http.StatusForbidden {
		t.Errorf("a plain member changing their own rate = %d, want 403", code)
	}
	// Sem valor nem grupo, não há o que gravar.
	if code, out := putAllocation(e, admin, prj, plain.id, `{}`); code != http.StatusBadRequest || !strings.Contains(out, "allocation.rate_required") {
		t.Errorf("an empty PUT = %d %s, want 400 allocation.rate_required", code, out)
	}
	// Quem entra precisa do valor.
	if code, out := putAllocation(e, admin, prj, newcomer.id, `{"preset":"manager"}`); code != http.StatusBadRequest || !strings.Contains(out, "allocation.rate_required") {
		t.Errorf("adding a person with only a group = %d %s, want 400 allocation.rate_required", code, out)
	}
	// O grupo fica na alocação, com a lista de permissões dele.
	rec := do(e, "GET", "/api/projects/"+prj+"/allocations", "", admin.session)
	for _, a := range decodeList(t, rec) {
		if a["person_id"] == finance.id {
			perms, _ := a["permissions"].([]any)
			if a["preset"] != "finance" || len(perms) != 4 {
				t.Errorf("the finance allocation = preset %v, permissions %v", a["preset"], a["permissions"])
			}
		}
	}
}

// As permissões da organização são do dono: ele libera criar projetos, cuidar dos clientes e
// das pessoas a quem não é admin, e os admins não precisam delas.
func TestPermissions_OrganizationOnesBelongToTheOwner(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "bia@test.com", "admin")
	member := invite(t, e, owner, "caio@test.com", "member")
	org := "/api/orgs/" + owner.orgID

	// Sem nada liberado, o membro não faz nada de gestão da organização.
	for name, c := range map[string][3]string{
		"create a project": {"POST", org + "/projects", `{"name":"X"}`},
		"list customers":   {"GET", org + "/customers", ""},
		"create an invite": {"POST", org + "/invites", `{"role":"member"}`},
	} {
		if got := status(e, c[0], c[1], c[2], member.session); got != http.StatusForbidden {
			t.Errorf("a member without permissions: %s = %d, want 403", name, got)
		}
	}

	// Só o dono libera, e só a quem não é admin.
	grant := func(by account, personID, body string) (int, string) {
		rec := do(e, "PATCH", "/api/persons/"+personID+"/permissions", body, by.session)
		return rec.Code, rec.Body.String()
	}
	if code, _ := grant(admin, member.id, `{"permissions":["projects.create"]}`); code != http.StatusForbidden {
		t.Errorf("an admin giving permissions = %d, want 403", code)
	}
	if code, _ := grant(member, member.id, `{"permissions":["projects.create"]}`); code != http.StatusForbidden {
		t.Errorf("a member giving themselves permissions = %d, want 403", code)
	}
	if code, out := grant(owner, admin.id, `{"permissions":["projects.create"]}`); code != http.StatusBadRequest || !strings.Contains(out, "person.admin_has_all_permissions") {
		t.Errorf("giving permissions to an admin = %d %s, want 400 person.admin_has_all_permissions", code, out)
	}
	if code, out := grant(owner, member.id, `{"permissions":["billing.view"]}`); code != http.StatusBadRequest || !strings.Contains(out, "person.invalid_permission") {
		t.Errorf("a project permission at the organization scope = %d %s, want 400 person.invalid_permission", code, out)
	}
	code, out := grant(owner, member.id, `{"permissions":["projects.create","customers.manage","projects.create"]}`)
	if code != http.StatusOK || !strings.Contains(out, `"permissions":["projects.create","customers.manage"]`) {
		t.Fatalf("the owner giving permissions = %d %s", code, out)
	}

	// Agora o membro cria projeto e cuida de clientes, mas ainda não convida nem muda papéis.
	if got := status(e, "POST", org+"/projects", `{"name":"X"}`, member.session); got != http.StatusCreated {
		t.Errorf("the member creating a project = %d, want 201", got)
	}
	if got := status(e, "GET", org+"/customers", "", member.session); got != http.StatusOK {
		t.Errorf("the member listing customers = %d, want 200", got)
	}
	if got := status(e, "POST", org+"/invites", `{"role":"member"}`, member.session); got != http.StatusForbidden {
		t.Errorf("the member inviting = %d, want 403", got)
	}
	if got := status(e, "PATCH", "/api/persons/"+admin.id+"/role", `{"role":"member"}`, member.session); got != http.StatusForbidden {
		t.Errorf("the member changing a role = %d, want 403", got)
	}
	// O admin faz tudo isso sem permissão nenhuma, menos o que é do dono.
	if got := status(e, "POST", org+"/projects", `{"name":"Y"}`, admin.session); got != http.StatusCreated {
		t.Errorf("the admin creating a project = %d, want 201", got)
	}
	if got := status(e, "POST", org+"/invites", `{"role":"admin"}`, admin.session); got != http.StatusForbidden {
		t.Errorf("an admin inviting another admin = %d, want 403 (it is the owner's)", got)
	}
	if got := status(e, "POST", org+"/invites", `{"role":"member"}`, admin.session); got != http.StatusCreated {
		t.Errorf("an admin inviting a member = %d, want 201", got)
	}
	if got := status(e, "DELETE", org, "", admin.session); got != http.StatusForbidden {
		t.Errorf("an admin deleting the organization = %d, want 403 (it is the owner's)", got)
	}
}

// O catálogo das permissões é público para quem está logado, e a identidade traz o que a
// pessoa pode na organização.
func TestPermissions_CatalogAndIdentity(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, owner, "caio@test.com", "member")

	rec := do(e, "GET", "/api/permissions", "", member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/permissions = %d", rec.Code)
	}
	cat := decode(t, rec)
	if p, _ := cat["project"].([]any); len(p) != 9 {
		t.Errorf("project permissions = %v, want 9", cat["project"])
	}
	if o, _ := cat["organization"].([]any); len(o) != 3 {
		t.Errorf("organization permissions = %v, want 3", cat["organization"])
	}
	if presets, _ := cat["presets"].([]any); len(presets) != 4 {
		t.Errorf("presets = %v, want 4", cat["presets"])
	}
	if got := status(e, "GET", "/api/permissions", "", ""); got != http.StatusUnauthorized {
		t.Errorf("the catalog without a session = %d, want 401", got)
	}

	do(e, "PATCH", "/api/persons/"+member.id+"/permissions", `{"permissions":["people.manage"]}`, owner.session)
	me := decode(t, do(e, "GET", "/api/auth/me", "", member.session))
	if perms, _ := me["permissions"].([]any); len(perms) != 1 || perms[0] != "people.manage" {
		t.Errorf("/auth/me permissions = %v, want [people.manage]", me["permissions"])
	}
}
