package server_test

import (
	"regexp"
	"strings"
	"testing"
)

// A página inicial é um painel: os admins veem a visão geral da organização (tempo, receita, custo e margem de todos
// os projetos, e a equipe) e todos veem o próprio painel. Os projetos têm a página deles, com o item na barra.
func TestPages_HomeIsAnOverview(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	home := "/orgs/" + admin.orgID

	adminPage := do(e, "GET", home, "", admin.session).Body.String()
	for _, want := range []string{
		`x-data="orgHome"`, `id="home-stats-title"`, "Visão geral", `class="kpis"`, `class="segmented"`,
		"Suas horas", "Horas da equipe", "Trabalhando agora", "Receita", "Custo", "Margem",
		`id="home-team-title"`, `href="` + home + `/people"`, "Ver colaboradores", "Adicionar colaborador",
		`id="home-me-title"`,
	} {
		if !strings.Contains(adminPage, want) {
			t.Errorf("admin home page does not contain %q", want)
		}
	}
	// Os cartões de projeto e o Novo projeto foram para a página Projetos.
	for _, gone := range []string{`class="project-grid"`, `class="project-card"`, `openCreate()`, `id="project-name"`} {
		if strings.Contains(adminPage, gone) {
			t.Errorf("the home page still has %q, which belongs to the projects page", gone)
		}
	}
	team := adminPage[strings.Index(adminPage, `id="home-team-title"`):]
	team = team[:strings.Index(team, `class="team-list"`)]
	// A equipe mostra quem trabalha agora e em quê, e não compara o tempo de ninguém: sem horas,
	// sem barras e sem a ordem por quem trabalhou mais, que ficaram guardadas no parcial team_hours.
	teamCard := adminPage[strings.Index(adminPage, `id="home-team-title"`):strings.Index(adminPage, `id="home-me-title"`)]
	for _, want := range []string{"Trabalhando agora", "Na tarefa", `teamView()`, `'/tasks/' + p.working_on[0].task.id`} {
		if !strings.Contains(teamCard, want) {
			t.Errorf("the team card does not contain %q", want)
		}
	}
	for _, gone := range []string{`team-bar`, `team-hours`, `barWidth`, `secondsOf`, `rankingView`, "Horas de cada pessoa", "quem mais trabalhou", "Ninguém bateu ponto neste período"} {
		if strings.Contains(teamCard, gone) {
			t.Errorf("the team card still has %q: the home page must not compare how long each person worked", gone)
		}
	}
	if !strings.Contains(team, `href="`+home+`/people"`) || !strings.Contains(team, `href="`+home+`/people?add=1"`) {
		t.Errorf("the team block does not have both Ver colaboradores and Adicionar colaborador links: %s", team)
	}

	// O dinheiro e as horas de todos são só de admins: o membro não recebe nem o bloco, nem os rótulos.
	memberPage := do(e, "GET", home, "", member.session).Body.String()
	for _, want := range []string{`x-data="orgHome"`, `id="home-me-title"`} {
		if !strings.Contains(memberPage, want) {
			t.Errorf("member home page does not contain %q", want)
		}
	}
	for _, gone := range []string{`id="home-stats-title"`, `class="kpis"`, "Receita", "Horas da equipe", "Ver colaboradores", "Adicionar colaborador", `class="project-card"`} {
		if strings.Contains(memberPage, gone) {
			t.Errorf("member home page contains %q, which is for admins or for the projects page", gone)
		}
	}
}

// A página Clientes e Projetos é de todos: os projetos em cartões coloridos, agrupados pelo cliente, sem os horários da
// rotina que antes enchiam o cartão nem a paginação da antiga página Projetos. Só quem pode criar projeto vê o Novo
// projeto, no cabeçalho da página, e o formulário dele; só o dono vê o Novo cliente e o formulário do cliente.
func TestPages_ProjectsPageHasTheCards(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "caio@test.com", "admin")
	member := invite(t, e, owner, "bia@test.com", "member")
	projects := "/orgs/" + owner.orgID + "/projects"

	rec := do(e, "GET", projects, "", owner.session)
	if rec.Code != 200 {
		t.Fatalf("owner GET %s = %d", projects, rec.Code)
	}
	ownerPage := rec.Body.String()
	for _, want := range []string{
		`x-data="customersProjects"`, "<h1>Clientes e Projetos</h1>", `class="catalog-group"`, `x-for="g in groups()"`,
		`class="project-card"`, `class="project-mark"`, `markOf(p.name)`, `'hue-' + hueOf(p.id)`,
		"Novo projeto", "Novo cliente", `x-teleport="#modal-root"`, `id="project-name"`, `id="project-customer"`, `id="customer-name"`,
		`openCreate(g.customer ? g.customer.id : '')`, `openDetails(g.customer)`,
		// No celular as ações do grupo ficam num menu de três pontinhos, com as mesmas duas.
		`class="catalog-more"`, `fa-ellipsis-vertical`, `class="catalog-menu"`,
	} {
		if !strings.Contains(ownerPage, want) {
			t.Errorf("the owner's page does not contain %q", want)
		}
	}
	head := ownerPage[strings.Index(ownerPage, `class="page-head"`):strings.Index(ownerPage, `class="catalog"`)]
	for _, want := range []string{`openCreate()`, `openForm()`} {
		if !strings.Contains(head, want) {
			t.Errorf("the page header lacks %s", want)
		}
	}
	for _, gone := range []string{"<table", "<th>Projeto</th>", `class="pager"`} {
		if strings.Contains(ownerPage, gone) {
			t.Errorf("the page still has %q from the old pages", gone)
		}
	}
	// O cartão não carrega a rotina (sprint, daily, weekly, reunião) nem o cliente, que já é o do grupo.
	cards := ownerPage[strings.Index(ownerPage, `x-for="p in g.projects"`):]
	cards = cards[:strings.Index(cards, "</template>")]
	for _, gone := range []string{"Sprint de", "Daily ", "Weekly ", "Reunião ", "sprint_duration_days) }", "project-customer"} {
		if strings.Contains(cards, gone) {
			t.Errorf("a project card still shows %q", gone)
		}
	}

	// O admin cria projeto (só interno), mas não vê os clientes.
	adminPage := do(e, "GET", projects, "", admin.session).Body.String()
	for _, want := range []string{`x-data="customersProjects"`, "Novo projeto", `id="project-name"`} {
		if !strings.Contains(adminPage, want) {
			t.Errorf("admin page does not contain %q", want)
		}
	}
	for _, gone := range []string{"Novo cliente", `id="customer-name"`, `customer-details`, `id="project-customer"`} {
		if strings.Contains(adminPage, gone) {
			t.Errorf("admin page contains %q, which is the owner's", gone)
		}
	}

	rec = do(e, "GET", projects, "", member.session)
	if rec.Code != 200 {
		t.Fatalf("member GET %s = %d, want 200: the page is for everyone", projects, rec.Code)
	}
	memberPage := rec.Body.String()
	for _, want := range []string{`x-data="customersProjects"`, `class="project-card"`, "Você ainda não foi adicionado a nenhum projeto", "Os seus projetos, agrupados pelo cliente de cada um."} {
		if !strings.Contains(memberPage, want) {
			t.Errorf("member page does not contain %q", want)
		}
	}
	for _, gone := range []string{"Novo projeto", "Novo cliente", `openCreate()`, `id="project-name"`, `id="customer-name"`, `class="catalog-billing"`, `class="catalog-more"`} {
		if strings.Contains(memberPage, gone) {
			t.Errorf("member page contains %q, which is for who can create projects or customers", gone)
		}
	}
	// A lista vem inteira (sem page) e o componente agrupa pelo cliente; os clientes só vêm para quem os gerencia.
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	for _, want := range []string{"Alpine.data('customersProjects'", "api('GET', '/api/orgs/' + orgId + '/projects')", "this.manage ? api('GET', '/api/orgs/' + orgId + '/customers')"} {
		if !strings.Contains(script, want) {
			t.Errorf("org.js does not contain %q", want)
		}
	}
	for _, gone := range []string{"Alpine.data('orgProjects'", "Alpine.data('orgCustomers'"} {
		if strings.Contains(script, gone) {
			t.Errorf("org.js still has %q", gone)
		}
	}
}

// O atalho Adicionar colaborador da página inicial leva à página de colaboradores com um
// parâmetro na URL, e o componente dela abre o modal do convite quando o recebe. Os dois lados
// têm de falar do mesmo parâmetro, e a página de colaboradores, chamada com ele, é a mesma.
func TestPages_HomeAddPersonShortcutOpensTheInviteModal(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	people := "/orgs/" + admin.orgID + "/people"

	home := do(e, "GET", "/orgs/"+admin.orgID, "", admin.session).Body.String()
	m := regexp.MustCompile(`href="` + regexp.QuoteMeta(people) + `\?([a-z]+)=1"`).FindStringSubmatch(home)
	if m == nil {
		t.Fatalf("the home page has no shortcut to %s?<param>=1", people)
	}
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	if !strings.Contains(script, `.has('`+m[1]+`')`) {
		t.Errorf("org.js does not look for the %q parameter that the shortcut sends", m[1])
	}

	page := do(e, "GET", people+"?"+m[1]+"=1", "", admin.session)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `x-data="orgPeople"`) || !strings.Contains(page.Body.String(), `id="invite-email"`) {
		t.Errorf("the people page with ?%s=1 answered %d without the invite form", m[1], page.Code)
	}
}
