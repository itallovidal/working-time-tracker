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

// O primeiro cliente das boas-vindas tem o formulário inteiro do modal de cliente, nas mesmas duas partes: os dados
// (país, documento, contato, e-mail e telefone) e a cobrança. O valor por hora é obrigatório (Decisão 21) nos dois
// modos; no solo a cobrança também cria o cliente por contrato mensal, e na equipe não há modo nem contrato.
func TestOnboarding_FirstCustomerHasTheWholeCustomerForm(t *testing.T) {
	solo, team := pair(t, "org_home", func(s bool) Data {
		me := identity("admin", true, s)
		me.NeedsOnboarding = true
		return Data{Me: me, Section: "home", Script: "org"}
	})
	for who, html := range map[string]string{"solo": solo, "team": team} {
		mustHave(t, who+" onboarding", html, `class="wizard-substeps"`, `data-customer-step="1"`, `data-customer-step="2"`,
			`id="ob-customer-name"`, `id="ob-customer-country"`, `id="ob-customer-document"`, `id="ob-customer-contact"`,
			`id="ob-customer-email"`, `id="ob-customer-phone"`, `id="ob-customer-hourly-rate"`, `data-field="hourly_rate_cents"`,
			`id="err-hourly_rate_cents"`)
	}
	billing := []string{
		`id="ob-customer-billing-mode"`, `id="ob-customer-monthly-fee"`, `id="ob-customer-estimated-hours"`,
		`id="ob-customer-contract-start"`, `id="ob-customer-contract-end"`, `id="ob-customer-billing-cycle"`,
		`id="ob-customer-cycle-anchor"`, `id="ob-customer-closing-day"`, `id="ob-customer-payment-days"`, `id="ob-customer-late-fee"`,
	}
	mustHave(t, "solo onboarding", solo, billing...)
	mustNotHave(t, "team onboarding", team, billing...)
	mustHave(t, "solo onboarding", solo, "Cliente é quem contrata você. A cobrança, por hora ou por contrato mensal")
	mustHave(t, "team onboarding", team, "Cliente é quem contrata a organização.")
}

// formFields são os data-field do formulário que começa no marcador, na ordem em que aparecem.
func formFields(t *testing.T, html, marker string) []string {
	t.Helper()
	at := strings.Index(html, marker)
	if at < 0 {
		t.Fatalf("the page has no form with %s", marker)
	}
	end := strings.Index(html[at:], "</form>")
	if end < 0 {
		t.Fatalf("the form with %s has no end", marker)
	}
	var out []string
	for _, m := range regexp.MustCompile(`data-field="(\w+)"`).FindAllStringSubmatch(html[at:at+end], -1) {
		out = append(out, m[1])
	}
	return out
}

// O cliente das boas-vindas e o do modal de Clientes e Projetos são o mesmo formulário (o parcial customer_fields): os
// mesmos campos, na mesma ordem, no solo e na equipe. Antes eram escritos à mão em dois lugares, e a etapa das
// boas-vindas ficou sem o contato, o telefone, o país e o contrato mensal.
func TestCustomerForm_TheWelcomeAndTheModalAskTheSame(t *testing.T) {
	for _, solo := range []bool{true, false} {
		me := identity("admin", true, solo)
		projects := render(t, "org_projects", Data{Me: me, Section: "projects", Script: "org"})
		me = identity("admin", true, solo)
		me.NeedsOnboarding = true
		home := render(t, "org_home", Data{Me: me, Section: "home", Script: "org"})
		modal := formFields(t, projects, `@submit.prevent="submitCustomer()"`)
		welcome := formFields(t, home, `@submit.prevent="customerStep === 1 ? customerNext(customer) : saveCustomer()"`)
		if len(modal) < 7 || strings.Join(modal, " ") != strings.Join(welcome, " ") {
			t.Errorf("solo=%v: the welcome asks %v, the customer modal asks %v", solo, welcome, modal)
		}
	}
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
		mustHave(t, who+" project form", html, `id="project-bill-extra"`, `data-field="bill_extra_cents"`, `x-text="billing.customerAmount(customerOf())"`)
		if n := strings.Count(html, `id="project-bill-extra"`); n != 1 {
			t.Errorf("%s project form has %d extra fields, want 1", who, n)
		}
		// O acréscimo é um ajuste: fica no bloco Avançado, recolhido, no grupo Cobrança, entre o cliente e o Desenvolvimento.
		extra := strings.Index(html, `id="project-bill-extra"`)
		fold := strings.LastIndex(html[:max(extra, 0)], `<details class="form-fold"`)
		billing := strings.Index(html, ">Cobrança</legend>")
		routine := strings.Index(html, `id="project-sprint"`)
		if fold < 0 || billing < 0 || routine < 0 || fold < billing || extra > routine {
			t.Errorf("%s project form: the extra is not in a fold of the billing group (billing at %d, fold at %d, routine at %d)", who, billing, fold, routine)
		}
		// O grupo Cobrança só aparece com um cliente escolhido.
		mustHave(t, who+" project form", html, `<fieldset class="form-group" x-cloak x-show="customerOf()">`)
		mustHave(t, who+" project form", html, `:open="extraOpen"`, ">Avançado</summary>")
	}
}

// O modal de cliente, em Clientes e Projetos, continua com os campos do contrato, vindos do parcial. O rascunho é
// customerDraft, para não se misturar com o draft do projeto novo, no mesmo componente.
func TestCustomerModal_KeepsTheContractFields(t *testing.T) {
	html := render(t, "org_projects", Data{Me: identity("admin", true, true), Section: "projects", Script: "org"})
	mustHave(t, "solo customer modal", html, `id="customer-billing-mode"`, `id="customer-monthly-fee"`, `id="customer-contract-end"`,
		`id="customer-payment-days"`, `id="customer-grace-days"`, `x-model="customerDraft.monthly_fee"`,
		`id="customer-estimated-hours"`, `max="168"`, `x-model="customerDraft.estimated_hours"`, "Horas estimadas por semana")
	mustNotHave(t, "solo customer modal", html, `x-model="draft.monthly_fee"`, `x-model="draft.country"`)
	// Quem não gerencia os clientes não recebe o formulário nem os detalhes.
	admin := render(t, "org_projects", Data{Me: identity("admin", false, false), Section: "projects", Script: "org"})
	mustNotHave(t, "admin page", admin, `id="customer-name"`, `customer-details`)
}

// A dedicação aos contratos mensais com estimativa de horas (Decisões 26 e 29) é do modo solo: a coluna Estimativa da
// tela inicial e o cartão da visão geral do projeto (este, só para quem vê a cobrança, com a semana e o período). A
// equipe não tem cliente mensal.
func TestDedication_OnlyInSoloScreens(t *testing.T) {
	solo, team := pair(t, "org_home", func(s bool) Data {
		return Data{Me: identity("admin", true, s), Section: "home", Script: "org"}
	})
	mustHave(t, "solo home", solo, `x-show="soloEstimates()"`, "Estimativa", `WTT.dedication(row.dedication)`)
	mustNotHave(t, "team home", team, `soloEstimates()`, `WTT.dedication(`)

	project := &Crumb{ID: uuid.NewString(), Name: "Site"}
	// render dá a cada um o acesso de base: o dono vê a cobrança; o admin que não é dono, não.
	overview := func(owner bool) string {
		return render(t, "project_overview", Data{Me: identity("admin", owner, true), Project: project, Tab: "overview", Script: "project"})
	}
	mustHave(t, "solo overview of the owner", overview(true), `x-if="data && data.dedication"`, "Estimativa do cliente",
		`WTT.dedication(data.dedication)`, `x-if="data.dedication.week"`, `WTT.dedicationWeek(data.dedication)`, "Nesta semana",
		"No período atual")
	mustNotHave(t, "overview without billing.view", overview(false), `data.dedication`)
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
