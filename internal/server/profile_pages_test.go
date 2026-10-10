package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// O perfil de um colaborador é a tela da pessoa para quem paga: as horas, o valor e os pagamentos dela. Só admins
// abrem (sem os pagamentos nem a receita, que são do dono): o membro e quem é de outra organização recebem o 404.
func TestPages_CollaboratorProfileIsForAdmins(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	otherAdmin := invite(t, e, owner, "eva@test.com", "admin")
	bia := invite(t, e, owner, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zoe@outra.com")
	people := "/orgs/" + owner.orgID + "/people"

	for _, c := range []struct {
		who, path, session string
		want               int
	}{
		{"the owner opening a member", people + "/" + bia.id, owner.session, http.StatusOK},
		{"another admin opening a member", people + "/" + bia.id, otherAdmin.session, http.StatusOK},
		{"the owner opening themselves", people + "/" + owner.id, owner.session, http.StatusOK},
		{"a member opening themselves by this address", people + "/" + bia.id, bia.session, http.StatusNotFound},
		{"a member opening the owner", people + "/" + owner.id, bia.session, http.StatusNotFound},
		{"a person of another organization", people + "/" + outsider.id, owner.session, http.StatusNotFound},
		{"an id that is not an id", people + "/nada", owner.session, http.StatusNotFound},
		{"an id nobody has", people + "/00000000-0000-4000-8000-000000000000", owner.session, http.StatusNotFound},
		{"an admin of another organization", people + "/" + bia.id, outsider.session, http.StatusNotFound},
	} {
		if rec := do(e, "GET", c.path, "", c.session); rec.Code != c.want {
			t.Errorf("%s: GET %s = %d, want %d", c.who, c.path, rec.Code, c.want)
		}
	}
	if rec := do(e, "GET", people+"/"+bia.id, "", ""); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
		t.Errorf("without session = %d to %q, want 303 to /login", rec.Code, rec.Header().Get("Location"))
	}

	// A página: o nome vem do servidor, os blocos são os do perfil, e o Editar abre o modal do colaborador.
	body := do(e, "GET", people+"/"+bia.id, "", owner.session).Body.String()
	for _, want := range []string{
		`x-data="orgPerson"`, "<title>Colaborador · Convidado · Working Time Tracker</title>",
		`x-text="person ? person.name : $el.textContent">Convidado</strong>`, `<span class="avatar avatar-lg" aria-hidden="true">C</span>`,
		`href="` + people + `"`, `@click="openEdit(person)"`, `x-show="$store.modal.name === 'person-edit'"`,
		`id="person-tab-payment"`, `id="person-tab-projects"`,
		"Dados", "Próximo pagamento", "Horas trabalhadas", "A pagar", "Tarefas do período", "Desde o início", "Projetos e valores", "Histórico de pagamentos", "Definir regra",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the collaborator profile does not contain %q", want)
		}
	}
	if person, _ := bootOf(t, body)["person"].(map[string]any); person["id"] != bia.id || person["name"] != "Convidado" {
		t.Errorf("BOOT person = %v, want Bia", person)
	}
	// O modal de editar o próprio perfil (nome, email e senha) é só do /profile.
	for _, gone := range []string{`id="me-email"`, `id="pw-current"`, "openProfileEdit", "data-me-name x-text"} {
		if strings.Contains(body, gone) {
			t.Errorf("the collaborator profile has %q, which belongs to the viewer's own profile", gone)
		}
	}
	// A regra de pagamento é só do dono, também aqui: o admin que não é o dono abre o perfil, com a aba dos projetos
	// e a jornada, mas sem os campos da regra, sem os pagamentos e sem a receita.
	asAdmin := do(e, "GET", people+"/"+bia.id, "", otherAdmin.session)
	if asAdmin.Code != http.StatusOK || !strings.Contains(asAdmin.Body.String(), `id="person-tab-projects"`) || strings.Contains(asAdmin.Body.String(), "person-payment-frequency") {
		t.Errorf("an admin who is not the owner opening the profile = %d, want the page with the projects tab and no payment rule field", asAdmin.Code)
	}

	// A lista de colaboradores leva ao perfil para os admins.
	if list := do(e, "GET", people, "", owner.session).Body.String(); !strings.Contains(list, `class="row-link"`) || !strings.Contains(list, `<a :href="profileHref(p)" x-text="p.name"></a>`) {
		t.Error("the collaborators list does not link each row to the person's profile for an admin")
	}
}

// O meu perfil é para ler: o que fiz e quando sou pago, com os mesmos blocos que o admin vê ao abrir um colaborador
// (o que era "Meus pagamentos"). O nome, o email e a senha se alteram num modal, pelo Editar perfil.
func TestPages_ProfileShowsPaymentsAndEditsInAModal(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")

	rec := do(e, "GET", "/profile", "", bia.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /profile = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`x-data="profileSettings"`, `<strong class="profile-name" data-me-name x-text="person ? person.name : $el.textContent">Convidado</strong>`,
		`@click="openProfileEdit()"`, "Editar perfil",
		"Dados", "Próximo pagamento", "Horas trabalhadas", "A pagar", "Tarefas do período", "Desde o início", "Projetos e valores", "Histórico de pagamentos",
		`x-show="$store.modal.name === 'profile-edit'"`, `id="profile-tab-personal"`, `id="profile-tab-password"`,
		`id="me-name"`, `id="me-email"`, `id="pw-current"`, `id="pw-new"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the profile does not contain %q", want)
		}
	}
	// Os formulários ficam no modal, e não na página.
	if modal, field := strings.Index(body, `x-show="$store.modal.name === 'profile-edit'"`), strings.Index(body, `id="me-email"`); modal < 0 || field < modal {
		t.Error("the profile forms are on the page, outside the edit modal")
	}
	// O que é de quem abre o perfil de outra pessoa não vem: o modal do colaborador, o voltar e o definir regra.
	for _, gone := range []string{`x-show="$store.modal.name === 'person-edit'"`, `openEdit(person)`, "Definir regra", `href="/orgs/` + admin.orgID + `/people"`} {
		if strings.Contains(body, gone) {
			t.Errorf("the profile has %q, which is for an admin looking at a collaborator", gone)
		}
	}

	// O componente lê a pessoa, os pagamentos e os valores por hora dela.
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	for _, want := range []string{"const personProfile = ", "+ '/payments'", "+ '/allocations'", "'profile-edit'", "Alpine.data('orgPerson'", "afterPersonSaved"} {
		if !strings.Contains(script, want) {
			t.Errorf("org.js does not contain %q", want)
		}
	}
	// A resposta dos pagamentos traz o "desde o início", com ou sem regra.
	got := decode(t, do(e, "GET", "/api/persons/"+bia.id+"/payments", "", bia.session))
	life, _ := got["lifetime"].(map[string]any)
	if got["rule"] != nil || life == nil || life["seconds"] != float64(0) || life["amount_cents"] != float64(0) {
		t.Errorf("payments without a rule = %v, want rule null and an empty lifetime", got)
	}
	if projects, ok := life["projects"].([]any); !ok || len(projects) != 0 {
		t.Errorf("lifetime projects = %v, want an empty list (never null)", life["projects"])
	}
}

// Pagamentos é a visão de quem paga: só do dono, e só com a equipe. O que cada pessoa recebe ficou no
// perfil dela, e a linha da tabela leva até lá.
func TestPages_PaymentsIsTheTeamViewForAdmins(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	otherAdmin := invite(t, e, owner, "eva@test.com", "admin")
	bia := invite(t, e, owner, "bia@test.com", "member")
	path := "/orgs/" + owner.orgID + "/payments"

	for who, c := range map[string]struct {
		session string
		want    int
	}{
		"the owner":     {owner.session, http.StatusOK},
		"another admin": {otherAdmin.session, http.StatusNotFound},
		"a member":      {bia.session, http.StatusNotFound},
	} {
		if rec := do(e, "GET", path, "", c.session); rec.Code != c.want {
			t.Errorf("%s: GET %s = %d, want %d", who, path, rec.Code, c.want)
		}
	}

	body := do(e, "GET", path, "", owner.session).Body.String()
	for _, want := range []string{
		`x-data="orgPayments"`, "<h1>Pagamentos</h1>", "A pagar no período", "Fecha em 7 dias", "Horas da equipe", "Pessoas sem regra", "Datas de pagamento",
		`<tr class="row-link" @click="if (!$event.target.closest('a')) location.href = personHref(row.person)">`,
		`<a :href="personHref(row.person)" x-text="row.person.name"></a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the payments page does not contain %q", want)
		}
	}
	for _, gone := range []string{"Meus pagamentos", `role="tablist"`, "payments-mine", "openPerson(", "closePerson(", "setView("} {
		if strings.Contains(body, gone) {
			t.Errorf("the payments page still has %q from the old My payments view", gone)
		}
	}
	script := do(e, "GET", "/static/pages/payments.js", "", "").Body.String()
	if !strings.Contains(script, "+ '/people/' + person.id") || strings.Contains(script, "openPerson") || strings.Contains(script, "/api/persons/") {
		t.Error("payments.js should only read the team and link each person to their profile")
	}
}
