package page

import (
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/organization"
)

// O que as telas tiram ou trocam no modo solo, renderizado como o servidor faz e conferido também do lado da equipe,
// que não muda.

// pair renderiza a página para o dono de uma organização solo e para o dono de uma organização com equipe.
func pair(t *testing.T, name string, d func(solo bool) Data) (solo, team string) {
	t.Helper()
	return render(t, name, d(true)), render(t, name, d(false))
}

func mustHave(t *testing.T, who, html string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(html, w) {
			t.Errorf("%s: missing %s", who, w)
		}
	}
}

func mustNotHave(t *testing.T, who, html string, gone ...string) {
	t.Helper()
	for _, g := range gone {
		if strings.Contains(html, g) {
			t.Errorf("%s: still has %s", who, g)
		}
	}
}

// Regime e Moeda somem com a caixa do solo marcada (x-show="!form.solo"), e voltam ao desmarcá-la; o fuso fica.
func TestOrgSettings_WorkModeAndCurrencyFollowTheSoloBox(t *testing.T) {
	html := render(t, "org_settings", Data{Me: identity("admin", true, true), Org: &organization.Organization{Solo: true}, Section: "organization", Script: "org"})
	for _, id := range []string{"org-work-mode", "org-currency"} {
		if !regexp.MustCompile(`<div class="field" x-show="!form.solo">\s*<label for="` + id + `"`).MatchString(html) {
			t.Errorf("%s is not behind x-show=\"!form.solo\"", id)
		}
	}
	if regexp.MustCompile(`<div class="field" x-show="!form.solo">\s*<label for="org-timezone"`).MatchString(html) {
		t.Error("the time zone must stay in solo")
	}
	mustHave(t, "solo settings", html, `@change="countryChanged()"`)
}

// As boas-vindas do solo não pedem regime nem moeda.
func TestOnboarding_SoloAsksOnlyTheTimeZone(t *testing.T) {
	solo, team := pair(t, "org_home", func(s bool) Data {
		me := identity("admin", true, s)
		me.NeedsOnboarding = true
		return Data{Me: me, Section: "home", Script: "org"}
	})
	mustNotHave(t, "solo onboarding", solo, `id="ob-work-mode"`, `id="ob-currency"`)
	mustHave(t, "solo onboarding", solo, `id="ob-timezone"`)
	mustHave(t, "team onboarding", team, `id="ob-work-mode"`, `id="ob-currency"`, `id="ob-timezone"`)
}

// O novo projeto não configura a cobrança, que mora no cliente (Decisão 21): nos dois modos mostra a do cliente, só para
// leitura, e pede o acréscimo. A escolha de modo e os campos do contrato ficam só no modal de cliente.
func TestProjectForm_BillingComesFromTheCustomer(t *testing.T) {
	solo, team := pair(t, "org_projects", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Section: "projects", Script: "org"}
	})
	for who, html := range map[string]string{"solo": solo, "team": team} {
		mustNotHave(t, who+" project form", html, `name="project-billing"`, `id="project-monthly-fee"`, `id="project-closing-day"`,
			`id="project-bill-rate"`, `data-field="bill_rate_cents"`)
		mustHave(t, who+" project form", html, `id="project-bill-extra"`, `data-field="bill_extra_cents"`, `x-text="billing.customer(customerOf())"`)
		if n := strings.Count(html, `id="project-bill-extra"`); n != 1 {
			t.Errorf("%s project form has %d extra fields, want 1", who, n)
		}
	}
}

// O modal de cliente continua com os campos do contrato, agora vindos do parcial.
func TestCustomerModal_KeepsTheContractFields(t *testing.T) {
	html := render(t, "org_customers", Data{Me: identity("admin", true, true), Section: "customers", Script: "org"})
	mustHave(t, "solo customer modal", html, `id="customer-billing-mode"`, `id="customer-monthly-fee"`, `id="customer-contract-end"`,
		`id="customer-payment-days"`, `id="customer-grace-days"`, `x-model="draft.monthly_fee"`)
}

// A tarefa nova do solo é sua: o modal não pergunta o responsável. Na página da tarefa ficam "Comigo" e "Sem
// responsável", sem "Outra pessoa".
func TestTasks_NoOtherAssigneeInSolo(t *testing.T) {
	project := &Crumb{ID: uuid.NewString(), Name: "Site"}
	solo, team := pair(t, "project_tasks", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Project: project, Tab: "tasks", Script: "project"}
	})
	mustNotHave(t, "solo new task", solo, `name="task-assign"`)
	mustHave(t, "team new task", team, `name="task-assign"`, `value="other"`)

	task := &Crumb{ID: uuid.NewString(), Name: "Home"}
	solo, team = pair(t, "task_detail", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Project: project, Task: task, Script: "project"}
	})
	mustHave(t, "solo task detail", solo, `name="task-assign" value="me"`, `name="task-assign" value="none"`)
	mustNotHave(t, "solo task detail", solo, `name="task-assign" value="other"`, `id="task-assignee"`)
	mustHave(t, "team task detail", team, `name="task-assign" value="other"`, `id="task-assignee"`)
}

// A visão geral do projeto no solo não tem pessoas, times, quem está trabalhando, filtro de pessoa nem custo e margem.
func TestProjectOverview_SoloHasNoTeamNumbers(t *testing.T) {
	project := &Crumb{ID: uuid.NewString(), Name: "Site"}
	solo, team := pair(t, "project_overview", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Project: project, Management: true, Tab: "overview", Script: "project"}
	})
	teamOnly := []string{`data.people.total`, `data.teams.total`, `data.people.working_now`, `filter.person`,
		`filteredAmount('pay')`, `filteredMargin()`, `amount(s, 'pay')`}
	mustNotHave(t, "solo overview", solo, teamOnly...)
	mustHave(t, "team overview", team, teamOnly...)
	mustHave(t, "solo overview", solo, `filteredAmount('bill')`)
}

// A ajuda é pública: sem ninguém logado ela mostra tudo, e no solo esconde as seções de equipe.
func TestHelp_HidesTeamSectionsInSolo(t *testing.T) {
	teamSections := []string{`id="invite-team"`, `id="people-and-rates"`, `id="org-people"`, `id="payments"`,
		`id="project-people"`, `id="mgmt-people"`, `id="roles"`, `id="faq-invite-email"`, `href="#invite-team"`}
	public := render(t, "help", Data{Section: "help", Script: "help"})
	mustHave(t, "public help", public, teamSections...)
	solo, team := pair(t, "help", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Section: "help", Script: "help"}
	})
	mustNotHave(t, "solo help", solo, teamSections...)
	mustHave(t, "solo help", solo, `id="receivables"`, `id="solo-mode"`, `id="currency"`)
	mustHave(t, "team help", team, teamSections...)
}

// No solo, a barra superior não leva o cargo, e os cartões de projeto não contam pessoas.
func TestTopBarAndProjectCards_NoRoleNorPeopleInSolo(t *testing.T) {
	solo, team := pair(t, "org_projects", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Section: "projects", Script: "org"}
	})
	mustNotHave(t, "solo", solo, `<span class="badge badge-accent">Dono</span>`, `p.member_count`)
	mustHave(t, "team", team, `<span class="badge badge-accent">Dono</span>`, `p.member_count`)
}
