package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// O perfil de um colaborador é a tela da pessoa para quem paga: as horas, o valor e os pagamentos dela. Só admins
// abrem (é dinheiro): quem cuida de pessoas sem ser admin, o membro e quem é de outra organização recebem o 404.
func TestPages_CollaboratorProfileIsForAdmins(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	otherAdmin := invite(t, e, owner, "eva@test.com", "admin")
	bia := invite(t, e, owner, "bia@test.com", "member")
	carol := invite(t, e, owner, "carol@test.com", "member")
	outsider := signup(t, e, "Outra", "zoe@outra.com")
	if rec := do(e, "PATCH", "/api/persons/"+carol.id+"/permissions", `{"permissions":["people.manage"]}`, owner.session); rec.Code != http.StatusOK {
		t.Fatalf("grant people.manage = %d", rec.Code)
	}
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
		{"people.manage without admin", people + "/" + bia.id, carol.session, http.StatusNotFound},
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
		`x-text="person ? person.name : $el.textContent">Convidado</h1>`, `<span class="avatar avatar-lg" aria-hidden="true">C</span>`,
		`href="` + people + `"`, `@click="openEdit(person)"`, `x-show="$store.modal.name === 'person-edit'"`,
		`id="person-tab-payment"`, `id="person-tab-permissions"`, `id="person-tab-projects"`,
		"Neste período", "Próximo pagamento", "Desde o início", "Projetos e valores", "Histórico de pagamentos", "Próximos pagamentos", "Definir regra",
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
	// As permissões da organização seguem só do dono, também aqui.
	if asAdmin := do(e, "GET", people+"/"+bia.id, "", otherAdmin.session).Body.String(); strings.Contains(asAdmin, `id="person-tab-permissions"`) || !strings.Contains(asAdmin, `id="person-tab-projects"`) {
		t.Error("an admin who is not the owner should get the payment and projects tabs, and not the permissions one")
	}

	// A lista de colaboradores leva ao perfil só para admins: quem só cuida de pessoas continua com a lista e o lápis.
	if list := do(e, "GET", people, "", owner.session).Body.String(); !strings.Contains(list, `class="row-link"`) || !strings.Contains(list, `<a :href="profileHref(p)" x-text="p.name"></a>`) {
		t.Error("the collaborators list does not link each row to the person's profile for an admin")
	}
	list := do(e, "GET", people, "", carol.session)
	if body := list.Body.String(); list.Code != http.StatusOK || strings.Contains(body, "profileHref") || strings.Contains(body, `class="row-link"`) || !strings.Contains(body, `@click="openEdit(p)"`) {
		t.Errorf("people.manage alone = %d: the list should keep the pencil and not link to the profiles", list.Code)
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
		`x-data="profileSettings"`, `<h1 data-me-name x-text="person ? person.name : $el.textContent">Convidado</h1>`,
		`x-text="summaryLine()"`, `@click="openProfileEdit()"`, "Editar perfil",
		"Neste período", "Próximo pagamento", "Desde o início", "Projetos e valores", "Histórico de pagamentos", "Próximos pagamentos",
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

// Pagamentos é a visão de quem paga: só do dono e dos admins, e só com a equipe. O que cada pessoa recebe ficou no
// perfil dela, e a linha da tabela leva até lá.
func TestPages_PaymentsIsTheTeamViewForAdmins(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	otherAdmin := invite(t, e, owner, "eva@test.com", "admin")
	bia := invite(t, e, owner, "bia@test.com", "member")
	carol := invite(t, e, owner, "carol@test.com", "member")
	if rec := do(e, "PATCH", "/api/persons/"+carol.id+"/permissions", `{"permissions":["people.manage"]}`, owner.session); rec.Code != http.StatusOK {
		t.Fatalf("grant people.manage = %d", rec.Code)
	}
	path := "/orgs/" + owner.orgID + "/payments"

	for who, c := range map[string]struct {
		session string
		want    int
	}{
		"the owner":                   {owner.session, http.StatusOK},
		"another admin":               {otherAdmin.session, http.StatusOK},
		"a member":                    {bia.session, http.StatusNotFound},
		"people.manage without admin": {carol.session, http.StatusNotFound},
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
