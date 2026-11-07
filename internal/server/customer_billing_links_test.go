package server_test

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Os links das telas de Clientes e do formulário de projeto. No modo solo as páginas de equipe respondem 404
// (/orgs/:id/people, /orgs/:id/payments, /projects/:id/collaborators, /projects/:id/management/teams e
// /projects/:id/teams), então nada que é desta frente aponta para elas; e fora do modo solo a tela não ganha controle
// do modo mensal.

// teamPage reconhece o endereço de uma página de equipe, estática (href="/orgs/1/people") ou montada pelo Alpine
// (:href="'/orgs/' + id + '/people'"): o trecho final basta.
var teamPage = regexp.MustCompile(`/(people|payments|collaborators|teams)(/|$|['"?#\s])`)

var (
	hrefAttr = regexp.MustCompile(`(?s)\s:?href="([^"]*)"`)
	mainTag  = regexp.MustCompile(`(?s)<main\b.*?</main>`)
)

// hrefsOf lista o valor de todo href (e :href) do trecho de HTML.
func hrefsOf(html string) []string {
	var out []string
	for _, m := range hrefAttr.FindAllStringSubmatch(html, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// mainOf devolve o <main> da página: o conteúdo dela, sem a barra superior e o rodapé, que não são desta frente.
func mainOf(t *testing.T, name, page string) string {
	t.Helper()
	m := mainTag.FindString(page)
	if m == "" {
		t.Fatalf("%s has no <main>", name)
	}
	return m
}

// formOf devolve o formulário de um modal (o <form> cujo x-show nomeia o modal), até o </form>.
func formOf(t *testing.T, name, page, modal string) string {
	t.Helper()
	at := strings.Index(page, `$store.modal.name === '`+modal+`'`)
	if at < 0 {
		t.Fatalf("%s has no %s modal", name, modal)
	}
	end := strings.Index(page[at:], "</form>")
	if end < 0 {
		t.Fatalf("%s: the %s modal form has no end", name, modal)
	}
	return page[at : at+end]
}

// monthlyControls são as marcas dos controles do modo mensal e dos termos do cliente que são do modo solo. Nenhuma pode
// estar na tela de uma organização com equipe. O valor por hora do cliente não está aqui: todo cliente por hora tem um,
// na equipe também (Decisão 21).
var monthlyControls = []string{
	`id="customer-billing-mode"`, `id="customer-monthly-fee"`, `id="customer-contract-start"`,
	`id="customer-contract-end"`, `id="customer-closing-day"`, `id="customer-payment-days"`, `id="customer-late-fee"`,
	`id="customer-late-interest"`, `id="customer-grace-days"`,
	`data-field="billing_mode"`, `data-field="monthly_fee_cents"`, `data-field="contract_start"`,
	`data-field="contract_end"`, `data-field="closing_day"`, `data-field="payment_days"`, `data-field="late_fee_bp"`,
	`data-field="late_interest_bp_month"`, `data-field="grace_days"`,
	`lateOpen`, `draft.billing_mode === 'monthly'`, `billingTerms(`,
}

func TestCustomerBilling_LinksAndControlsByMode(t *testing.T) {
	e := newServer(t)
	solo := signup(t, e, "Solo", "ana@test.com")
	team := signup(t, e, "Equipe", "bia@test.com")
	makeSolo(t, solo.orgID, true)

	// Cada organização tem um cliente por hora e um projeto com ele, para as páginas terem o que listar.
	var projects = map[string]string{}
	for _, o := range []account{solo, team} {
		projects[o.orgID] = createProject(t, e, o, "Projeto "+o.orgID[:4])
		setBilling(t, e, o, "/api/projects/"+projects[o.orgID], 15000)
	}

	type view struct {
		name, html string
	}
	pagesOf := func(o account) []view {
		get := func(path string) string {
			rec := do(e, "GET", path, "", o.session)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d", path, rec.Code)
			}
			return rec.Body.String()
		}
		customers := get("/orgs/" + o.orgID + "/customers")
		newProject := get("/orgs/" + o.orgID + "/projects")
		settings := get("/projects/" + projects[o.orgID] + "/management/settings")
		return []view{
			{"customers page", mainOf(t, "customers page", customers)},
			{"new project page", mainOf(t, "new project page", newProject)},
			{"new project form", formOf(t, "new project page", newProject, "project")},
			{"edit project form", formOf(t, "project settings page", settings, "project-edit")},
		}
	}

	// Solo: nenhum href dos trechos desta frente leva a uma página que responde 404 no modo.
	var seen int
	for _, v := range pagesOf(solo) {
		for _, href := range hrefsOf(v.html) {
			seen++
			if teamPage.MatchString(href) {
				t.Errorf("solo %s links to %q, a page that is 404 in solo mode", v.name, href)
			}
		}
	}
	if seen == 0 {
		t.Error("no href found in the solo pages: the test is not reading the links")
	}
	// A tela de Clientes e os formulários do solo, de fato, têm os termos.
	soloViews := pagesOf(solo)
	if !strings.Contains(soloViews[0].html, `id="customer-billing-mode"`) {
		t.Error("the solo customers page has no billing mode selector")
	}

	// Fora do solo: nenhum controle do modo mensal, em nenhuma das telas, e os links de sempre seguem lá.
	teamViews := pagesOf(team)
	for _, v := range teamViews {
		for _, mark := range monthlyControls {
			if strings.Contains(v.html, mark) {
				t.Errorf("%s of an organization with a team has %s", v.name, mark)
			}
		}
	}
	if hrefs := hrefsOf(teamViews[3].html); len(hrefs) == 0 {
		t.Error("the edit project form of the team organization lost its customers link")
	}
}

// Os textos desta frente não mandam a pessoa para uma página de equipe, nem trazem link: a explicação do valor por hora
// que some, o modo travado, as dicas e os erros dos termos.
func TestCustomerBilling_TextsHaveNoTeamLinks(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)href|<a\b|https?://|/(people|payments|collaborators|teams)\b|colaboradores|collaborators|pagamentos|payments`)
	for _, lang := range []string{"pt-BR", "en"} {
		for key, text := range flatCatalog(t, lang) {
			ours := strings.HasPrefix(key, "customer.billing.") || strings.HasPrefix(key, "errors.customer.") || key == "errors.project.bill_rate_not_allowed"
			if ours && forbidden.MatchString(text) {
				t.Errorf("%s %s = %q: it links or points to a team page", lang, key, text)
			}
		}
	}
}
