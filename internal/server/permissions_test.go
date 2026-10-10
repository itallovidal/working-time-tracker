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
	customerID := decode(t, do(e, "POST", "/api/orgs/"+admin.orgID+"/customers", `{"name":"Cliente"}`, admin.session))["id"].(string)

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
		{"set the billing", "PUT", base + "/billing", `{"customer_id":"` + customerID + `","bill_rate_cents":9000}`, only("finance")},
		{"read the overview", "GET", base + "/overview", "", only("manager", "finance")},
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
	// E quem nem está no projeto não passa em nada dele: para ela o projeto não existe (404).
	for _, path := range []string{base + "/billing", base + "/overview"} {
		if got := status(e, "GET", path, "", stranger.session); got != http.StatusNotFound {
			t.Errorf("a person outside the project GET %s = %d, want 404", path, got)
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
	setBilling(t, e, admin, "/api/projects/"+prj, 9000)
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
	if got := status(e, "POST", "/api/projects/"+prj+"/teams", `{"name":"Outro"}`, newcomer.session); got != http.StatusNotFound {
		t.Errorf("a person removed from the project creating a team = %d, want 404 (the project is no longer theirs)", got)
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

// O modal do colaborador mexe nos valores de uma pessoa em vários projetos, e isso é só de admins: o membro não lê
// os valores dos outros nem põe alguém num projeto ou o tira dele, enquanto um admin que não é o dono faz as três
// coisas (rates.manage está nas permissões de projeto do cargo).
func TestPermissions_PersonRatesFromTheOrganizationAreForAdmins(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "bia@test.com", "admin")
	member := invite(t, e, owner, "pam@test.com", "member")
	target := invite(t, e, owner, "caio@test.com", "member")
	alfa := createEmptyProject(t, e, owner, "Alfa")
	beta := createEmptyProject(t, e, owner, "Beta")
	allocate(t, e, owner, alfa, target.id, 2000)

	list := "/api/persons/" + target.id + "/allocations"
	if code := status(e, "GET", list, "", member.session); code != http.StatusForbidden {
		t.Errorf("a member reading another person's rates = %d, want 403", code)
	}
	if code, _ := putAllocation(e, member, beta, target.id, `{"pay_rate_cents":3000}`); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("a member putting someone on a project = %d, want 403 or 404", code)
	}
	if code := status(e, "DELETE", "/api/projects/"+alfa+"/collaborators/"+target.id, "", member.session); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("a member taking someone off a project = %d, want 403 or 404", code)
	}

	if code := status(e, "GET", list, "", admin.session); code != http.StatusOK {
		t.Errorf("an admin reading another person's rates = %d, want 200", code)
	}
	if code, out := putAllocation(e, admin, alfa, target.id, `{"pay_rate_cents":2500}`); code != http.StatusOK {
		t.Errorf("an admin changing a rate = %d: %s", code, out)
	}
	if code, out := putAllocation(e, admin, beta, target.id, `{"pay_rate_cents":3000}`); code != http.StatusOK {
		t.Errorf("an admin putting someone on a project = %d: %s", code, out)
	}
	if code := status(e, "DELETE", "/api/projects/"+alfa+"/collaborators/"+target.id, "", admin.session); code != http.StatusNoContent {
		t.Errorf("an admin taking someone off a project = %d, want 204", code)
	}
	left := decodeList(t, do(e, "GET", list, "", admin.session))
	if len(left) != 1 || left[0]["project_id"] != beta || left[0]["pay_rate_cents"] != float64(3000) {
		t.Errorf("after the changes the person's rates = %v, want only Beta at 3000", left)
	}
}

// As permissões da organização decorrem do cargo: o dono tem todas, o admin cria projeto (só interno) e cuida das
// pessoas, e o membro não tem nenhuma. O dinheiro (clientes, cobrança, pagamentos) e os dados da organização são do
// dono, e não há mais rota para dar permissão avulsa.
func TestPermissions_OrganizationOnesFollowTheRole(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "bia@test.com", "admin")
	member := invite(t, e, owner, "caio@test.com", "member")
	org := "/api/orgs/" + owner.orgID
	customerID := decode(t, do(e, "POST", org+"/customers", `{"name":"Cliente"}`, owner.session))["id"].(string)

	for _, c := range []struct {
		name, method, path, body string
		who                      string
		session                  string
		want                     int
	}{
		{"create an internal project", "POST", org + "/projects", `{"name":"X"}`, "member", member.session, http.StatusForbidden},
		{"list customers", "GET", org + "/customers", "", "member", member.session, http.StatusForbidden},
		{"create an invite", "POST", org + "/invites", `{"role":"member"}`, "member", member.session, http.StatusForbidden},
		{"payments of the team", "GET", org + "/payments", "", "member", member.session, http.StatusForbidden},

		{"create an internal project", "POST", org + "/projects", `{"name":"Y"}`, "admin", admin.session, http.StatusCreated},
		{"create a project with a customer", "POST", org + "/projects", `{"name":"Z","customer_id":"` + customerID + `","bill_rate_cents":10000}`, "admin", admin.session, http.StatusForbidden},
		{"create a project with a billed rate", "POST", org + "/projects", `{"name":"W","bill_rate_cents":10000}`, "admin", admin.session, http.StatusForbidden},
		{"list customers", "GET", org + "/customers", "", "admin", admin.session, http.StatusForbidden},
		{"create a customer", "POST", org + "/customers", `{"name":"Outro"}`, "admin", admin.session, http.StatusForbidden},
		{"payments of the team", "GET", org + "/payments", "", "admin", admin.session, http.StatusForbidden},
		{"edit the organization", "PATCH", org, `{"summary":"X"}`, "admin", admin.session, http.StatusForbidden},
		{"set a payment rule", "PATCH", "/api/persons/" + member.id + "/payment", `{"frequency":"monthly","day":5}`, "admin", admin.session, http.StatusForbidden},
		{"set the weekly hours", "PATCH", "/api/persons/" + member.id + "/weekly-hours", `{"weekly_hours":30}`, "admin", admin.session, http.StatusOK},
		{"invite a member", "POST", org + "/invites", `{"role":"member"}`, "admin", admin.session, http.StatusCreated},
		{"invite an admin", "POST", org + "/invites", `{"role":"admin"}`, "admin", admin.session, http.StatusForbidden},
		{"change a role", "PATCH", "/api/persons/" + member.id + "/role", `{"role":"admin"}`, "admin", admin.session, http.StatusForbidden},
		{"delete the organization", "DELETE", org, "", "admin", admin.session, http.StatusForbidden},

		{"create a project with a customer", "POST", org + "/projects", `{"name":"Com cliente","customer_id":"` + customerID + `","bill_rate_cents":10000}`, "owner", owner.session, http.StatusCreated},
		{"list customers", "GET", org + "/customers", "", "owner", owner.session, http.StatusOK},
		{"payments of the team", "GET", org + "/payments", "", "owner", owner.session, http.StatusOK},
		{"set a payment rule", "PATCH", "/api/persons/" + member.id + "/payment", `{"frequency":"monthly","day":5}`, "owner", owner.session, http.StatusOK},
		{"edit the organization", "PATCH", org, `{"summary":"X"}`, "owner", owner.session, http.StatusOK},
	} {
		if got := status(e, c.method, c.path, c.body, c.session); got != c.want {
			t.Errorf("%s: %s = %d, want %d", c.who, c.name, got, c.want)
		}
	}

	// Dar permissão avulsa deixou de existir, para todos.
	if got := status(e, "PATCH", "/api/persons/"+member.id+"/permissions", `{"permissions":["projects.create"]}`, owner.session); got == http.StatusOK {
		t.Errorf("the owner giving a loose permission = %d, want the route gone", got)
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
	if p, _ := cat["project"].([]any); len(p) != 10 {
		t.Errorf("project permissions = %v, want 10", cat["project"])
	}
	if o, _ := cat["organization"].([]any); len(o) != 4 {
		t.Errorf("organization permissions = %v, want 4", cat["organization"])
	}
	if presets, _ := cat["presets"].([]any); len(presets) != 4 {
		t.Errorf("presets = %v, want 4", cat["presets"])
	}
	if got := status(e, "GET", "/api/permissions", "", ""); got != http.StatusUnauthorized {
		t.Errorf("the catalog without a session = %d, want 401", got)
	}

	// A identidade não traz mais a lista de permissões da organização: ela decorre do cargo, e a tela lê o que pode
	// em BOOT.can.
	me := decode(t, do(e, "GET", "/api/auth/me", "", member.session))
	if _, has := me["permissions"]; has {
		t.Errorf("/auth/me still carries permissions: %v", me["permissions"])
	}
}

// A lista de colaboradores diz o grupo de cada um: é o que o modal do colaborador marca.
func TestPermissions_CollaboratorsCarryTheirGroup(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	manager := invite(t, e, admin, "gabi@test.com", "member")
	plain := invite(t, e, admin, "caio@test.com", "member")
	prj := createProject(t, e, admin, "Alfa")
	withPreset(t, e, admin, prj, manager.id, "manager")
	allocate(t, e, admin, prj, plain.id, 2000)

	got := map[string]any{}
	for _, c := range decodeList(t, do(e, "GET", "/api/projects/"+prj+"/collaborators", "", plain.session)) {
		got[c["person"].(map[string]any)["id"].(string)] = c["preset"]
	}
	if got[manager.id] != "manager" || got[plain.id] != "member" || got[admin.id] != "member" {
		t.Errorf("groups = %v, want the manager as manager and the others as member", got)
	}
}
