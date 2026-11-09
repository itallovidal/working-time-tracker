package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

// O modo solo (organizations.solo): uma organização de uma pessoa só. Ligado, o que é de equipe some (Colaboradores,
// Pagamentos, os convites, as abas de pessoas e times do projeto, o bloco da Equipe, o passo de convite das
// boas-vindas) e o menu ganha A receber. Desligado, nada mudou: estes testes põem sempre uma organização com equipe
// ao lado, para provar que ela continua exatamente igual.

// soloSignup cria uma organização que já nasce no modo solo.
func soloSignup(t *testing.T, e *echo.Echo, org, email string) account {
	t.Helper()
	rec := do(e, "POST", "/api/auth/signup",
		`{"organization_name":"`+org+`","solo":true,"name":"Admin","email":"`+email+`","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("solo signup = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["organization_solo"] != true {
		t.Fatalf("the solo signup identity = %v, want organization_solo true", body)
	}
	return account{session: sessionFrom(t, rec), orgID: body["organization_id"].(string), id: body["id"].(string)}
}

// setSoloAPI faz o PATCH da organização só com o modo solo.
func setSoloAPI(e *echo.Echo, who account, solo bool) *httptest.ResponseRecorder {
	body := `{"solo":false}`
	if solo {
		body = `{"solo":true}`
	}
	return do(e, "PATCH", "/api/orgs/"+who.orgID, body, who.session)
}

func meSolo(t *testing.T, e *echo.Echo, who account) bool {
	t.Helper()
	solo, ok := decode(t, do(e, "GET", "/api/auth/me", "", who.session))["organization_solo"].(bool)
	if !ok {
		t.Fatal("/api/auth/me has no organization_solo")
	}
	return solo
}

// errorField devolve o campo que um erro da API diz (params.field).
func errorField(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	detail, _ := decode(t, rec)["error"].(map[string]any)
	params, _ := detail["params"].(map[string]any)
	field, _ := params["field"].(string)
	return field
}

// O cadastro com solo cria a organização no modo; sem o campo, a organização é com equipe. O modo vai no /auth/me, na
// organização e no window.BOOT que as telas leem.
func TestSolo_SignupCreatesTheOrganizationInSoloMode(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")

	for _, c := range []struct {
		who  string
		a    account
		want bool
	}{{"solo", solo, true}, {"team", team, false}} {
		if got := meSolo(t, e, c.a); got != c.want {
			t.Errorf("%s: /api/auth/me organization_solo = %v, want %v", c.who, got, c.want)
		}
		org := decode(t, do(e, "GET", "/api/orgs/"+c.a.orgID, "", c.a.session))
		if org["solo"] != c.want {
			t.Errorf("%s: the organization solo = %v, want %v", c.who, org["solo"], c.want)
		}
		boot := bootOf(t, do(e, "GET", "/orgs/"+c.a.orgID, "", c.a.session).Body.String())
		me, _ := boot["me"].(map[string]any)
		if me["organization_solo"] != c.want {
			t.Errorf("%s: window.BOOT.me.organization_solo = %v, want %v", c.who, me["organization_solo"], c.want)
		}
	}
	// O login também devolve o modo.
	rec := do(e, "POST", "/api/auth/login", `{"email":"ana@solo.com","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusOK || decode(t, rec)["organization_solo"] != true {
		t.Errorf("login = %d %s, want organization_solo true", rec.Code, rec.Body.String())
	}
}

// Quem entra pelo Clerk escolhe o modo na volta, quando cria a organização.
func TestSolo_ClerkSignupCreatesTheOrganizationInSoloMode(t *testing.T) {
	e, fake := newClerkServer(t)
	fake.AddUser("user_solo", "ana@solo.com", "Ana", true)
	fake.AddUser("user_team", "bia@equipe.com", "Bia", true)

	for _, c := range []struct {
		user, body string
		want       bool
	}{
		{"user_solo", `{"organization_name":"Solo","solo":true}`, true},
		{"user_team", `{"organization_name":"Equipe"}`, false},
	} {
		rec := clerkDo(e, "/api/auth/clerk/signup", c.body, fake.Token(c.user))
		if rec.Code != http.StatusCreated {
			t.Fatalf("clerk signup %s = %d: %s", c.user, rec.Code, rec.Body.String())
		}
		identity, _ := decode(t, rec)["identity"].(map[string]any)
		if identity["organization_solo"] != c.want {
			t.Errorf("%s: identity organization_solo = %v, want %v", c.user, identity["organization_solo"], c.want)
		}
		if got := meSolo(t, e, account{session: sessionFrom(t, rec)}); got != c.want {
			t.Errorf("%s: /api/auth/me organization_solo = %v, want %v", c.user, got, c.want)
		}
	}
}

// Só o dono liga e desliga (PATCH da organização), e desligar não apaga nada: a organização volta a ser a de equipe.
func TestSolo_OwnerTurnsItOnAndOff(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")

	rec := setSoloAPI(e, owner, true)
	if rec.Code != http.StatusOK || decode(t, rec)["solo"] != true {
		t.Fatalf("turn on = %d %s, want 200 with solo true", rec.Code, rec.Body.String())
	}
	if !meSolo(t, e, owner) {
		t.Error("/api/auth/me must say the organization is solo after turning it on")
	}
	rec = setSoloAPI(e, owner, false)
	if rec.Code != http.StatusOK || decode(t, rec)["solo"] != false {
		t.Fatalf("turn off = %d %s, want 200 with solo false", rec.Code, rec.Body.String())
	}
	if meSolo(t, e, owner) {
		t.Error("/api/auth/me must say the organization is not solo after turning it off")
	}

	// Salvar o resto da organização, mandando o modo que ela já tem, não confere nem muda nada.
	setSoloAPI(e, owner, true)
	if rec := do(e, "PATCH", "/api/orgs/"+owner.orgID, `{"summary":"Freelancer","solo":true}`, owner.session); rec.Code != http.StatusOK {
		t.Errorf("PATCH with the same solo = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

// O PATCH da organização é do dono: o admin que não é o dono e o membro recebem 403, e o modo não muda.
func TestSolo_OnlyTheOwnerChangesIt(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	member := invite(t, e, owner, "bruno@test.com", "member")

	for name, who := range map[string]account{"admin": admin, "member": member} {
		rec := setSoloAPI(e, who, true)
		if rec.Code != http.StatusForbidden || errorCode(t, rec) != "auth.owner_only" {
			t.Errorf("%s turning solo on = %d %s, want 403 auth.owner_only", name, rec.Code, rec.Body.String())
		}
	}
	if meSolo(t, e, owner) {
		t.Error("a refused request must not turn solo on")
	}
}

// Ligar pede exatamente uma pessoa e nenhum convite pendente: senão 409, com o campo, e a organização fica como estava.
// Desligar sempre vale.
func TestSolo_TurnOnNeedsOnePersonAndNoPendingInvite(t *testing.T) {
	e := newServer(t)

	// Duas pessoas.
	owner := signup(t, e, "Org", "ana@test.com")
	invite(t, e, owner, "bia@test.com", "member")
	rec := setSoloAPI(e, owner, true)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "organization.solo_needs_single_person" || errorField(t, rec) != "solo" {
		t.Errorf("two people = %d %s, want 409 organization.solo_needs_single_person on the field solo", rec.Code, rec.Body.String())
	}
	if meSolo(t, e, owner) {
		t.Error("a refused request must not turn solo on")
	}

	// Uma pessoa e um convite pendente; revogado o convite, liga.
	other := signup(t, e, "Outra", "caio@test.com")
	rec = do(e, "POST", "/api/orgs/"+other.orgID+"/invites", `{"role":"member"}`, other.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite = %d: %s", rec.Code, rec.Body.String())
	}
	inviteID := decode(t, rec)["id"].(string)
	rec = setSoloAPI(e, other, true)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "organization.solo_needs_single_person" {
		t.Errorf("pending invite = %d %s, want 409 organization.solo_needs_single_person", rec.Code, rec.Body.String())
	}
	if rec := do(e, "DELETE", "/api/invites/"+inviteID, "", other.session); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke the invite = %d", rec.Code)
	}
	if rec := setSoloAPI(e, other, true); rec.Code != http.StatusOK {
		t.Errorf("after revoking the invite: turn on = %d %s, want 200", rec.Code, rec.Body.String())
	}

	// Desligar sempre vale, ainda que a organização já não coubesse no modo.
	solo := soloSignup(t, e, "Solo", "dani@test.com")
	testClient.Person.Create().SetName("Eva").SetEmail("eva@test.com").SetOrganizationID(uuid.MustParse(solo.orgID)).ExecX(context.Background())
	if rec := setSoloAPI(e, solo, false); rec.Code != http.StatusOK {
		t.Errorf("turn off with two people = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

// Convidar, da organização e do projeto, responde 409 numa organização solo, com qualquer corpo e antes de ler o
// valor do projeto; a lista de convites segue vazia. Numa organização com equipe, e depois de desligar o modo, convida.
func TestSolo_InvitesAreRefused(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")
	soloProject := createProject(t, e, solo, "Projeto Solo")
	teamProject := createProject(t, e, team, "Projeto Equipe")

	refused := []struct{ name, path, body string }{
		{"organization, no body", "/api/orgs/" + solo.orgID + "/invites", ``},
		{"organization, member", "/api/orgs/" + solo.orgID + "/invites", `{"role":"member"}`},
		{"organization, with an email", "/api/orgs/" + solo.orgID + "/invites", `{"email":"novo@solo.com"}`},
		{"organization, admin", "/api/orgs/" + solo.orgID + "/invites", `{"role":"admin"}`},
		{"project, complete", "/api/projects/" + soloProject + "/invites", `{"email":"novo@solo.com","pay_rate_cents":5000}`},
		{"project, without the rate", "/api/projects/" + soloProject + "/invites", `{"email":"novo@solo.com"}`},
		{"project, empty", "/api/projects/" + soloProject + "/invites", `{}`},
	}
	for _, c := range refused {
		rec := do(e, "POST", c.path, c.body, solo.session)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != "organization.solo_no_invites" {
			t.Errorf("solo %s = %d %s, want 409 organization.solo_no_invites", c.name, rec.Code, rec.Body.String())
		}
	}
	if rec := do(e, "GET", "/api/orgs/"+solo.orgID+"/invites", "", solo.session); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("the solo invite list = %d %s, want an empty list", rec.Code, rec.Body.String())
	}

	// A organização com equipe convida como sempre, para a organização e para o projeto.
	if rec := do(e, "POST", "/api/orgs/"+team.orgID+"/invites", `{"role":"member"}`, team.session); rec.Code != http.StatusCreated {
		t.Errorf("team invite = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if rec := do(e, "POST", "/api/projects/"+teamProject+"/invites", `{"email":"novo@equipe.com","pay_rate_cents":5000}`, team.session); rec.Code != http.StatusCreated {
		t.Errorf("team project invite = %d %s, want 201", rec.Code, rec.Body.String())
	}

	// Desligado o modo, a mesma sessão convida de novo.
	if rec := setSoloAPI(e, solo, false); rec.Code != http.StatusOK {
		t.Fatalf("turn off = %d", rec.Code)
	}
	if rec := do(e, "POST", "/api/orgs/"+solo.orgID+"/invites", `{"role":"member"}`, solo.session); rec.Code != http.StatusCreated {
		t.Errorf("invite after turning solo off = %d %s, want 201", rec.Code, rec.Body.String())
	}
}

// As páginas de equipe respondem 404 numa organização solo: Colaboradores, o perfil de um colaborador, Pagamentos e as
// abas Colaboradores e Times do projeto (a de fora da Gestão, a da Gestão e o caminho antigo). O resto abre. Na
// organização com equipe todas abrem, e desligar o modo devolve as páginas na mesma sessão.
func TestSolo_TeamPagesAre404(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")
	soloProject := createProject(t, e, solo, "Projeto Solo")
	teamProject := createProject(t, e, team, "Projeto Equipe")

	paths := func(a account, project string) (hidden, shown []string) {
		org := "/orgs/" + a.orgID
		hidden = []string{
			org + "/people", org + "/people?add=1", org + "/people/" + a.id, org + "/payments",
			"/projects/" + project + "/collaborators", "/projects/" + project + "/management/teams", "/projects/" + project + "/teams",
		}
		shown = []string{
			org, org + "/projects", org + "/about", org + "/settings", "/profile",
			"/projects/" + project + "/overview", "/projects/" + project + "/tasks", "/projects/" + project + "/my-tasks",
			"/projects/" + project + "/management/overview", "/projects/" + project + "/management/integrations",
			"/projects/" + project + "/management/settings",
		}
		return hidden, shown
	}

	hidden, shown := paths(solo, soloProject)
	for _, path := range hidden {
		rec := do(e, "GET", path, "", solo.session)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "Página não encontrada") {
			t.Errorf("solo GET %s = %d, want the 404 page", path, rec.Code)
		}
	}
	for _, path := range shown {
		if rec := do(e, "GET", path, "", solo.session); rec.Code != http.StatusOK {
			t.Errorf("solo GET %s = %d, want 200", path, rec.Code)
		}
	}
	// No solo a Visão geral é uma aba do projeto, fora da Gestão: a raiz da Gestão cai em Integrações.
	if rec := do(e, "GET", "/projects/"+soloProject+"/management", "", solo.session); rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "/management/integrations") {
		t.Errorf("solo GET management = %d to %q, want 303 to the integrations", rec.Code, rec.Header().Get("Location"))
	}

	// Com equipe, tudo abre (o legado /teams redireciona).
	hiddenTeam, shownTeam := paths(team, teamProject)
	for _, path := range append(hiddenTeam, shownTeam...) {
		rec := do(e, "GET", path, "", team.session)
		want := http.StatusOK
		if strings.HasSuffix(path, "/teams") && strings.HasPrefix(path, "/projects/") && !strings.Contains(path, "management") {
			want = http.StatusSeeOther
		}
		if rec.Code != want {
			t.Errorf("team GET %s = %d, want %d", path, rec.Code, want)
		}
	}

	// Desligar o modo traz as páginas de volta, na mesma sessão: nada foi apagado.
	if rec := setSoloAPI(e, solo, false); rec.Code != http.StatusOK {
		t.Fatalf("turn off = %d", rec.Code)
	}
	for _, path := range hidden {
		want := http.StatusOK
		if path == "/projects/"+soloProject+"/teams" {
			want = http.StatusSeeOther
		}
		if rec := do(e, "GET", path, "", solo.session); rec.Code != want {
			t.Errorf("after turning solo off GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

// O menu: sem Colaboradores nem Pagamentos, com A receber (para o dono, que gerencia os recebíveis). A organização com
// equipe mantém o menu de sempre, sem A receber. O texto segue o idioma.
func TestSolo_MenuHasReceivablesInsteadOfTheTeamPages(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")
	soloProject := createProject(t, e, solo, "Projeto Solo")
	home := "/orgs/" + solo.orgID

	soloNav := func(current string) []string {
		items := []string{"Início " + home, "Clientes e Projetos " + home + "/projects", "A receber " + home + "/receivables", "Configurações " + home + "/about"}
		for i, it := range items {
			if strings.HasPrefix(it, current+" ") {
				items[i] += " (current)"
			}
		}
		return items
	}
	for _, c := range []struct{ name, path, current string }{
		{"home", home, "Início"},
		{"customers and projects", home + "/projects", "Clientes e Projetos"},
		{"in a project", "/projects/" + soloProject + "/tasks", "Clientes e Projetos"},
		{"settings", home + "/about", "Configurações"},
		{"edit organization", home + "/settings", "Configurações"},
		{"profile", "/profile", ""},
	} {
		rec := do(e, "GET", c.path, "", solo.session)
		if rec.Code != http.StatusOK {
			t.Errorf("solo %s: GET %s = %d", c.name, c.path, rec.Code)
			continue
		}
		if got, want := strings.Join(mainNav(t, rec.Body.String()), " | "), strings.Join(soloNav(c.current), " | "); got != want {
			t.Errorf("solo %s: nav = %q, want %q", c.name, got, want)
		}
	}

	// Em inglês.
	en := strings.Join(mainNav(t, getPage(e, home, solo.session, "en", "").Body.String()), " | ")
	want := "Home " + home + " (current) | Customers &amp; Projects " + home + "/projects | Receivables " + home + "/receivables | Settings " + home + "/about"
	if en != want {
		t.Errorf("solo nav in English = %q, want %q", en, want)
	}

	// Com equipe: o menu de sempre, sem A receber.
	teamHome := "/orgs/" + team.orgID
	teamNav := strings.Join(mainNav(t, do(e, "GET", teamHome, "", team.session).Body.String()), " | ")
	wantTeam := "Início " + teamHome + " (current) | Clientes e Projetos " + teamHome + "/projects | Colaboradores " + teamHome + "/people | Pagamentos " + teamHome + "/payments | Configurações " + teamHome + "/about"
	if teamNav != wantTeam {
		t.Errorf("team nav = %q, want %q", teamNav, wantTeam)
	}

	// Desligar o modo devolve o menu de equipe na mesma sessão.
	setSoloAPI(e, solo, false)
	if got := strings.Join(mainNav(t, do(e, "GET", home, "", solo.session).Body.String()), " | "); strings.Contains(got, "A receber") || !strings.Contains(got, "Colaboradores") || !strings.Contains(got, "Pagamentos") {
		t.Errorf("nav after turning solo off = %q, want the team menu", got)
	}
}

// kpiLabel é o rótulo de um cartão da visão geral, como a página o escreve: a palavra solta (Custo) também está em outros
// textos (saveCustomer), e o cartão não.
func kpiLabel(text string) string { return `<span class="kpi-label">` + text + `</span>` }

// A tela inicial do solo não tem o bloco da Equipe nem o cartão Trabalhando agora, e a visão geral dela é a do solo
// (solo_overview_test.go): sem a equipe, o custo e a margem. O painel da pessoa fica. A organização com equipe mantém a
// visão geral e a Equipe de sempre.
func TestSolo_HomeLosesTheTeamBlock(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")

	soloPage := do(e, "GET", "/orgs/"+solo.orgID, "", solo.session).Body.String()
	for _, want := range []string{`x-data="orgHome"`, `id="home-solo-title"`, `id="home-me-title"`, "Receita projetada", "Valor por hora efetivo"} {
		if !strings.Contains(soloPage, want) {
			t.Errorf("the solo home does not contain %q", want)
		}
	}
	for _, gone := range []string{
		`id="home-stats-title"`, `id="home-team-title"`, `stats.people.working_now`, `teamView()`, "Ver colaboradores",
		"Adicionar colaborador", `/people`, kpiLabel("Horas da equipe"), kpiLabel("Trabalhando agora"), kpiLabel("Custo"), kpiLabel("Margem"),
		kpiLabel("Suas horas"),
	} {
		if strings.Contains(soloPage, gone) {
			t.Errorf("the solo home still contains %q", gone)
		}
	}

	teamPage := do(e, "GET", "/orgs/"+team.orgID, "", team.session).Body.String()
	for _, want := range []string{
		`id="home-stats-title"`, `id="home-team-title"`, `stats.people.working_now`, `teamView()`, "Ver colaboradores", "Adicionar colaborador",
		kpiLabel("Suas horas"), kpiLabel("Horas da equipe"), kpiLabel("Trabalhando agora"), kpiLabel("Receita"), kpiLabel("Custo"), kpiLabel("Margem"),
	} {
		if !strings.Contains(teamPage, want) {
			t.Errorf("the team home lost %q", want)
		}
	}
}

// As boas-vindas do solo: organização, primeiro cliente e fim, sem o passo de convidar e sem o link para Colaboradores.
// Com equipe são as quatro etapas de sempre.
func TestSolo_WelcomeHasNoInviteStep(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")

	soloPage := do(e, "GET", "/orgs/"+solo.orgID, "", solo.session).Body.String()
	if !strings.Contains(soloPage, onboardingMarkup) {
		t.Fatal("the solo owner must see the welcome")
	}
	welcome := soloPage[strings.Index(soloPage, onboardingMarkup):]
	welcome = welcome[:strings.Index(welcome, "</main>")]
	steps := welcome[strings.Index(welcome, `<ol class="wizard-steps"`):]
	steps = steps[:strings.Index(steps, "</ol>")]
	if n := strings.Count(steps, "<li "); n != 3 {
		t.Errorf("the solo welcome has %d steps, want 3 (organization, first customer, done): %s", n, steps)
	}
	for _, want := range []string{`<span class="n">1</span> Organização`, `<span class="n">2</span> Clientes`, `<span class="n">3</span> Pronto`} {
		if !strings.Contains(steps, want) {
			t.Errorf("the solo welcome steps lack %q: %s", want, steps)
		}
	}
	for _, gone := range []string{"ob-invite-email", "createInvite()", `/people`, "Convidar", "step === 3"} {
		if strings.Contains(welcome, gone) {
			t.Errorf("the solo welcome still has %q", gone)
		}
	}
	for _, want := range []string{"ob-customer-name", `/projects`, `/settings`} {
		if !strings.Contains(welcome, want) {
			t.Errorf("the solo welcome lost %q", want)
		}
	}

	teamPage := do(e, "GET", "/orgs/"+team.orgID, "", team.session).Body.String()
	welcome = teamPage[strings.Index(teamPage, onboardingMarkup):]
	welcome = welcome[:strings.Index(welcome, "</main>")]
	steps = welcome[strings.Index(welcome, `<ol class="wizard-steps"`):]
	steps = steps[:strings.Index(steps, "</ol>")]
	if n := strings.Count(steps, "<li "); n != 4 {
		t.Errorf("the team welcome has %d steps, want 4", n)
	}
	for _, want := range []string{`<span class="n">3</span> Colaboradores`, `<span class="n">4</span> Pronto`, "ob-invite-email", "createInvite()", `/people`} {
		if !strings.Contains(welcome, want) {
			t.Errorf("the team welcome lost %q", want)
		}
	}

	// O script pula o passo de convite no modo solo.
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	if !strings.Contains(script, "me.organization_solo && this.step === 2 ? 4 : this.step + 1") {
		t.Error("org.js: the welcome does not skip the invite step in solo mode")
	}
}

// As abas do projeto: sem Colaboradores (as pessoas e os times) no modo solo, na Gestão e fora dela; o resto fica.
func TestSolo_ProjectTabsLoseTheTeamTab(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	team := signup(t, e, "Equipe", "bia@equipe.com")
	soloProject := createProject(t, e, solo, "Projeto Solo")
	teamProject := createProject(t, e, team, "Projeto Equipe")

	tabs := func(a account, path string) string {
		t.Helper()
		body := do(e, "GET", path, "", a.session).Body.String()
		start := strings.Index(body, `<nav class="tabs"`)
		if start < 0 {
			t.Fatalf("GET %s has no tab strip", path)
		}
		return body[start : start+strings.Index(body[start:], "</nav>")]
	}
	// No solo a Visão geral mora na barra do projeto, e a Gestão fica com Integrações e Configurações; aberta, a Visão
	// geral mostra a barra do projeto, com ela marcada.
	href := func(project, path string) string { return `href="/projects/` + project + path + `"` }
	for _, c := range []struct {
		name, path string
		keep, gone []string
		current    string
	}{
		{"home", "/projects/%s/overview", []string{"/overview", "/my-tasks", "/tasks", "/management/overview", "/management"}, []string{"/management/integrations", "/management/settings"}, "/overview"},
		{"overview", "/projects/%s/management/overview", []string{"/overview", "/my-tasks", "/tasks", "/management/overview", "/management"}, []string{"/management/integrations", "/management/settings"}, "/management/overview"},
		{"management", "/projects/%s/management/integrations", []string{"/management/integrations", "/management/settings", "/overview"}, []string{"/management/overview", "/my-tasks", "/tasks"}, "/management/integrations"},
	} {
		soloTabs := tabs(solo, strings.Replace(c.path, "%s", soloProject, 1))
		for _, gone := range append([]string{"/collaborators", "/management/teams"}, c.gone...) {
			if strings.Contains(soloTabs, href(soloProject, gone)) {
				t.Errorf("solo %s: the tab strip still has %q: %s", c.name, gone, soloTabs)
			}
		}
		if strings.Contains(soloTabs, "Colaboradores no projeto") {
			t.Errorf("solo %s: the tab strip still has the teams tab: %s", c.name, soloTabs)
		}
		for _, want := range c.keep {
			if !strings.Contains(soloTabs, href(soloProject, want)) {
				t.Errorf("solo %s: the tab strip lost %q: %s", c.name, want, soloTabs)
			}
		}
		if !strings.Contains(soloTabs, href(soloProject, c.current)+` aria-current="page"`) || strings.Count(soloTabs, `aria-current="page"`) != 1 {
			t.Errorf("solo %s: %q is not the one current tab: %s", c.name, c.current, soloTabs)
		}
	}

	// Com equipe nada muda: a Visão geral fica dentro da Gestão, com Colaboradores.
	for _, c := range []struct{ name, path, want string }{
		{"outside the management", "/projects/%s/overview", "/collaborators"},
		{"inside the management", "/projects/%s/management/overview", "/management/teams"},
	} {
		teamTabs := tabs(team, strings.Replace(c.path, "%s", teamProject, 1))
		if !strings.Contains(teamTabs, href(teamProject, c.want)) || !strings.Contains(teamTabs, "Colaboradores no projeto") {
			t.Errorf("team %s: the tab strip lost the teams tab (%q): %s", c.name, c.want, teamTabs)
		}
		overview := strings.Contains(teamTabs, href(teamProject, "/management/overview"))
		if inside := strings.Contains(c.name, "inside"); overview != inside {
			t.Errorf("team %s: the overview tab shown = %v, want %v: %s", c.name, overview, inside, teamTabs)
		}
	}
}

// Os controles do modo solo: o check com a dica no cadastro, na volta do Clerk e em Configurações, e o JavaScript
// manda o campo. O texto vem do catálogo nos dois idiomas.
func TestSolo_ControlsAreOnTheScreens(t *testing.T) {
	// Com o Clerk ligado, para a página de retorno dele existir.
	e, _ := newClerkServer(t)
	owner := signup(t, e, "Org", "ana@test.com")

	pages := []struct {
		name, path, session string
		wants               []string
	}{
		{"sign-up", "/signup", "", []string{`id="signup-solo"`, `data-field="solo"`, `x-model="solo"`, `id="err-solo"`}},
		{"Clerk return", "/auth/clerk/continue", "", []string{`id="clerk-solo"`, `data-field="solo"`, `x-model="solo"`, `id="err-solo"`}},
		{"settings", "/orgs/" + owner.orgID + "/settings", owner.session, []string{`id="org-solo"`, `data-field="solo"`, `x-model="form.solo"`, `id="err-solo"`}},
	}
	for _, p := range pages {
		pt := getPage(e, p.path, p.session, "pt-BR", "").Body.String()
		en := getPage(e, p.path, p.session, "en", "").Body.String()
		for _, want := range append([]string{"Trabalho sozinho(a)", "freelancer ou PJ"}, p.wants...) {
			if !strings.Contains(pt, want) {
				t.Errorf("%s (pt-BR) does not contain %q", p.name, want)
			}
		}
		for _, want := range append([]string{"I work alone", "freelancers and solo contractors"}, p.wants...) {
			if !strings.Contains(en, want) {
				t.Errorf("%s (en) does not contain %q", p.name, want)
			}
		}
		if got := strings.Count(pt, `type="checkbox" data-field="solo"`); got != 1 {
			t.Errorf("%s has %d solo checkboxes, want 1", p.name, got)
		}
	}
	// Só a tela de Configurações diz a regra de ligar e que desligar não apaga.
	if settings := getPage(e, "/orgs/"+owner.orgID+"/settings", owner.session, "pt-BR", "").Body.String(); !strings.Contains(settings, "sem convites pendentes") || !strings.Contains(settings, "Desligar não apaga nada") {
		t.Error("the settings screen does not explain when solo mode turns on and that turning it off keeps everything")
	}

	// O JavaScript manda o campo nos três lugares.
	auth := do(e, "GET", "/static/pages/auth.js", "", "").Body.String()
	if n := strings.Count(auth, "solo: this.solo"); n != 2 {
		t.Errorf("auth.js sends solo %d times, want 2 (the sign-up and the Clerk return)", n)
	}
	org := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	for _, want := range []string{"f.solo = !!o.solo", "body.solo = !!f.solo"} {
		if !strings.Contains(org, want) {
			t.Errorf("org.js lacks %q: Settings would not read or send the solo mode", want)
		}
	}
}

// O modo solo não mexe na API de uma organização com equipe: as rotas de pessoas, pagamentos e convites respondem como
// antes, e o corpo estrito continua recusando chave desconhecida (o campo novo é o único que entrou).
func TestSolo_TeamOrganizationAPIIsUnchanged(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	org := "/api/orgs/" + owner.orgID

	for _, path := range []string{org + "/persons", org + "/payments", org + "/invites", org + "/customers"} {
		if rec := do(e, "GET", path, "", owner.session); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
	if rec := do(e, "POST", org+"/invites", `{"role":"member"}`, owner.session); rec.Code != http.StatusCreated {
		t.Errorf("invite = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if rec := do(e, "PATCH", org, `{"sol":true}`, owner.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "request.invalid_body" {
		t.Errorf("an unknown key = %d %s, want 400 request.invalid_body", rec.Code, rec.Body.String())
	}
	if rec := do(e, "PATCH", org, `{"solo":"sim"}`, owner.session); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "request.invalid_body" {
		t.Errorf("a solo that is not a boolean = %d %s, want 400 request.invalid_body", rec.Code, rec.Body.String())
	}
}
