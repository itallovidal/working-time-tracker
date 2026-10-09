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

// A página Projetos é de todos: os projetos em cartões coloridos, por páginas, sem os horários da rotina que antes
// enchiam o cartão. Só quem pode criar projeto vê o Novo projeto, no cabeçalho da página, e o formulário dele. A
// tabela de gestão da antiga aba Projetos saiu: os cartões já mostram o cliente, as pessoas e as tarefas.
func TestPages_ProjectsPageHasTheCards(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projects := "/orgs/" + admin.orgID + "/projects"

	rec := do(e, "GET", projects, "", admin.session)
	if rec.Code != 200 {
		t.Fatalf("admin GET %s = %d", projects, rec.Code)
	}
	adminPage := rec.Body.String()
	for _, want := range []string{
		`x-data="orgProjects"`, "<h1>Projetos</h1>", `class="project-card"`, `class="project-mark"`, `markOf(p.name)`, `'hue-' + hueOf(p.id)`,
		`aria-label="Páginas da lista de projetos"`, "Novo projeto", `x-teleport="#modal-root"`, `id="project-name"`, `id="project-customer"`,
	} {
		if !strings.Contains(adminPage, want) {
			t.Errorf("the projects page does not contain %q", want)
		}
	}
	head := adminPage[strings.Index(adminPage, `class="page-head"`):strings.Index(adminPage, `class="project-grid"`)]
	if !strings.Contains(head, `openCreate()`) {
		t.Error("the New project button is not in the page header")
	}
	if strings.Contains(adminPage, "<table") || strings.Contains(adminPage, "<th>Projeto</th>") {
		t.Error("the projects page still has the management table")
	}
	// O cartão não carrega a rotina (sprint, daily, weekly, reunião): só quem é, o cliente e o tamanho.
	for _, gone := range []string{"Sprint de", "Daily ", "Weekly ", "Reunião ", "sprint_duration_days) }"} {
		grid := adminPage[strings.Index(adminPage, `class="project-grid"`):]
		if strings.Contains(grid[:strings.Index(grid, `class="pager"`)], gone) {
			t.Errorf("a project card still shows %q", gone)
		}
	}

	rec = do(e, "GET", projects, "", member.session)
	if rec.Code != 200 {
		t.Fatalf("member GET %s = %d, want 200: the page is for everyone", projects, rec.Code)
	}
	memberPage := rec.Body.String()
	for _, want := range []string{`x-data="orgProjects"`, `class="project-card"`, "Você ainda não foi adicionado a nenhum projeto"} {
		if !strings.Contains(memberPage, want) {
			t.Errorf("member projects page does not contain %q", want)
		}
	}
	for _, gone := range []string{"Novo projeto", `openCreate()`, `id="project-name"`} {
		if strings.Contains(memberPage, gone) {
			t.Errorf("member projects page contains %q, which is for who can create projects", gone)
		}
	}
	// A lista é paginada pelo servidor, e o componente lê a página do endereço.
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	for _, want := range []string{"Alpine.data('orgProjects'", "'/projects?page='", "searchParams.set('page'"} {
		if !strings.Contains(script, want) {
			t.Errorf("org.js does not contain %q", want)
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
