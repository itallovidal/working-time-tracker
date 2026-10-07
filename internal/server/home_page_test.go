package server_test

import (
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
	} {
		if !strings.Contains(adminPage, want) {
			t.Errorf("admin home page does not contain %q", want)
		}
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
	for _, gone := range []string{`id="home-stats-title"`, `class="kpis"`, "Receita", "Horas da equipe", "Ver colaboradores", "Novo projeto", `id="project-name"`} {
		if strings.Contains(memberPage, gone) {
			t.Errorf("member home page contains %q, which is for admins", gone)
		}
	}
}
