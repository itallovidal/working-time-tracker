package server_test

import (
	"regexp"
	"strings"
	"testing"
)

// A página inicial é um painel: os admins veem a visão geral da organização (tempo, receita,
// custo e margem de todos os projetos, e a equipe) e todos veem os projetos em cartões
// coloridos, por páginas, sem os horários da rotina que antes enchiam o cartão.
func TestPages_HomeIsAnOverviewWithProjectCards(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	home := "/orgs/" + admin.orgID

	adminPage := do(e, "GET", home, "", admin.session).Body.String()
	for _, want := range []string{
		`x-data="orgHome"`, `id="home-stats-title"`, "Visão geral", `class="kpis"`, `class="segmented"`,
		"Suas horas", "Horas da equipe", "Trabalhando agora", "Receita", "Custo", "Margem",
		`id="home-team-title"`, `href="` + home + `/people"`, "Ver colaboradores",
		`id="home-projects-title"`, `class="project-card"`, `class="project-mark"`, `markOf(p.name)`, `'hue-' + hueOf(p.id)`,
		`aria-label="Páginas da lista de projetos"`, "Novo projeto", `id="project-name"`, `id="project-customer"`,
		"Adicionar colaborador",
	} {
		if !strings.Contains(adminPage, want) {
			t.Errorf("admin home page does not contain %q", want)
		}
	}
	// O botão Novo projeto fica junto da lista de projetos, não no cabeçalho da página; e o
	// Adicionar colaborador fica ao lado do Ver colaboradores, no bloco da equipe.
	projects := adminPage[strings.Index(adminPage, `id="home-projects-title"`):]
	if !strings.Contains(projects[:strings.Index(projects, `class="project-grid"`)], `openCreate()`) {
		t.Error("the New project button is not in the projects section, next to its title")
	}
	if strings.Contains(adminPage[:strings.Index(adminPage, `id="home-stats-title"`)], `openCreate()`) {
		t.Error("the New project button is still in the page header")
	}
	team := adminPage[strings.Index(adminPage, `id="home-team-title"`):]
	team = team[:strings.Index(team, `class="team-list"`)]
	if !strings.Contains(team, `href="`+home+`/people"`) || !strings.Contains(team, `href="`+home+`/people?add=1"`) {
		t.Errorf("the team block does not have both Ver colaboradores and Adicionar colaborador links: %s", team)
	}
	// O cartão não carrega a rotina (sprint, daily, weekly, reunião): só quem é, o cliente e o tamanho.
	for _, gone := range []string{"Sprint de", "Daily ", "Weekly ", "Reunião ", "sprint_duration_days) }"} {
		if strings.Contains(adminPage[strings.Index(adminPage, `class="project-grid"`):], gone) {
			t.Errorf("a project card still shows %q", gone)
		}
	}

	// O dinheiro e as horas de todos são só de admins: o membro não recebe nem o bloco, nem os rótulos.
	memberPage := do(e, "GET", home, "", member.session).Body.String()
	for _, want := range []string{`x-data="orgHome"`, `id="home-projects-title"`, `class="project-card"`, `aria-label="Páginas da lista de projetos"`} {
		if !strings.Contains(memberPage, want) {
			t.Errorf("member home page does not contain %q", want)
		}
	}
	for _, gone := range []string{`id="home-stats-title"`, `class="kpis"`, "Receita", "Horas da equipe", "Ver colaboradores", "Adicionar colaborador", "Novo projeto", `id="project-name"`} {
		if strings.Contains(memberPage, gone) {
			t.Errorf("member home page contains %q, which is for admins", gone)
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
