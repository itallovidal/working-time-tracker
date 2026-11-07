package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"working-time-tracker/web"
)

// A tela inicial do modo solo (GET /api/orgs/:orgId/solo/overview e a página inicial): as horas, a Receita (o que entrou),
// a Receita projetada (o que se espera dos períodos que terminam na janela) e a tabela por cliente. Só o dono, só no modo
// solo. As contas de cada janela estão em internal/domain/overview/solo_test.go; aqui vale a API e a página montadas.

func soloOverviewPath(a account) string { return "/api/orgs/" + a.orgID + "/solo/overview" }

// monthDays são o primeiro e o último dia do mês corrente no fuso padrão da organização (America/Sao_Paulo).
func monthDays(t *testing.T, back int) (string, string) {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	first := time.Date(now.Year(), now.Month()-time.Month(back), 1, 0, 0, 0, 0, loc)
	return first.Format(time.DateOnly), first.AddDate(0, 1, -1).Format(time.DateOnly)
}

// O dono do modo solo lê as horas, a Receita e a projetada do mês passado e do corrente, por cliente e em ordem de nome.
func TestSoloOverview_OwnerReadsTheWindows(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	setSolo(t, owner.orgID, true)
	path := soloOverviewPath(owner)
	// Por hora: 1h a 100,00 no meio do mês passado. Mensal: 5.000,00 por mês, com o contrato começando no mês passado.
	hourlyCustomerWithHours(t, owner.orgID, owner.id)
	monthlyCustomer(t, owner.orgID)

	lastFirst, lastLast := monthDays(t, 1)
	rec := do(e, "GET", path+"?window=last_month", "", owner.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("last month = %d: %s", rec.Code, rec.Body.String())
	}
	last := decode(t, rec)
	totals := last["totals"].(map[string]any)
	if last["window"] != "last_month" || last["from"] != lastFirst || last["to"] != lastLast || last["timezone"] != "America/Sao_Paulo" || last["currency"] != "BRL" {
		t.Errorf("last month = %v, want the %s..%s window in America/Sao_Paulo and BRL", last, lastFirst, lastLast)
	}
	// 1h trabalhada; nada recebido; esperados 100,00 (a hora do Por hora, ainda não confirmada) e 5.000,00 (o mês do Mensal).
	if totals["seconds"] != float64(3600) || totals["received_cents"] != float64(0) || totals["projected_cents"] != float64(510000) ||
		totals["effective_rate_cents"] != float64(510000) || totals["below_min_rate"] != false {
		t.Errorf("last month totals = %v", totals)
	}
	rows := last["customers"].([]any)
	if len(rows) != 2 || last["internal_seconds"] != float64(0) {
		t.Fatalf("last month customers = %v, internal %v, want 2 rows and no internal hours", rows, last["internal_seconds"])
	}
	monthly, hourly := rows[0].(map[string]any), rows[1].(map[string]any)
	if monthly["customer"].(map[string]any)["name"] != "Mensal" || monthly["kind"] != "monthly" || monthly["seconds"] != float64(0) ||
		monthly["projected_cents"] != float64(500000) || monthly["effective_rate_cents"] != nil || monthly["below_min_rate"] != false {
		t.Errorf("the monthly row = %v, want no hours, 5.000,00 and no rate", monthly)
	}
	if hourly["customer"].(map[string]any)["name"] != "Por hora" || hourly["kind"] != "hourly" || hourly["seconds"] != float64(3600) ||
		hourly["projected_cents"] != float64(10000) || hourly["effective_rate_cents"] != float64(10000) {
		t.Errorf("the hourly row = %v, want 1h, 100,00 and 100,00 per hour", hourly)
	}

	// Sem janela vale o mês corrente: sem horas nem recebimento, só a mensalidade do mês (o contrato começou no passado).
	first, lastDay := monthDays(t, 0)
	current := decode(t, do(e, "GET", path, "", owner.session))
	if current["window"] != "month" || current["from"] != first || current["to"] != lastDay {
		t.Errorf("the default window = %v %v..%v, want month %s..%s", current["window"], current["from"], current["to"], first, lastDay)
	}
	if ct := current["totals"].(map[string]any); ct["seconds"] != float64(0) || ct["projected_cents"] != float64(500000) || ct["effective_rate_cents"] != nil {
		t.Errorf("this month totals = %v, want only the 5.000,00 of the monthly contract", ct)
	}

	// Confirmar o mês passado do Por hora e receber hoje: é Receita do mês corrente (a data do recebimento), e continua
	// projetada do mês passado (o fim do período), sem contar duas vezes em lugar nenhum.
	var period map[string]any
	for _, p := range decode(t, do(e, "GET", receivablesPath(owner), "", owner.session))["to_confirm"].([]any) {
		if p.(map[string]any)["kind"] == "hourly" {
			period = p.(map[string]any)
		}
	}
	if period == nil {
		t.Fatal("the hourly customer has no period to confirm")
	}
	body := fmt.Sprintf(`{"customer_id":"%s","period_start":"%s","period_end":"%s","amount_cents":10000}`,
		period["customer"].(map[string]any)["id"], period["period_start"], period["period_end"])
	created := do(e, "POST", receivablesPath(owner), body, owner.session)
	if created.Code != http.StatusCreated {
		t.Fatalf("confirm = %d: %s", created.Code, created.Body.String())
	}
	if got := status(e, "POST", receivablesPath(owner)+"/"+decode(t, created)["id"].(string)+"/receive", `{}`, owner.session); got != http.StatusOK {
		t.Fatalf("receive = %d", got)
	}
	this := decode(t, do(e, "GET", path+"?window=month", "", owner.session))["totals"].(map[string]any)
	if this["received_cents"] != float64(10000) || this["projected_cents"] != float64(500000) {
		t.Errorf("this month after receiving: totals %v, want received 10000 and projected 500000", this)
	}
	past := decode(t, do(e, "GET", path+"?window=last_month", "", owner.session))["totals"].(map[string]any)
	if past["received_cents"] != float64(0) || past["projected_cents"] != float64(510000) {
		t.Errorf("last month after receiving: totals %v, want received 0 and projected 510000", past)
	}
	// O ano vai de 1º de janeiro ao fim do mês corrente e traz as duas contas: o que entrou hoje e tudo o que se espera.
	year := decode(t, do(e, "GET", path+"?window=year", "", owner.session))
	yearFirst := first[:4] + "-01-01"
	if yt := year["totals"].(map[string]any); year["window"] != "year" || year["from"] != yearFirst || year["to"] != lastDay || yt["received_cents"] != float64(10000) {
		t.Errorf("the year = %v, want %s..%s with the 100,00 received", year, yearFirst, lastDay)
	}
}

// A janela inválida é 400 com o código e o campo; sem sessão, 401; a organização de outra pessoa, 404.
func TestSoloOverview_Errors(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	other := signup(t, e, "Outra", "bia@outra.com")
	setSolo(t, owner.orgID, true)
	setSolo(t, other.orgID, true)
	path := soloOverviewPath(owner)

	for _, w := range []string{"week", "MONTH", "all_time", "2026", "last-month"} {
		rec := do(e, "GET", path+"?window="+w, "", owner.session)
		detail, _ := decode(t, rec)["error"].(map[string]any)
		if rec.Code != http.StatusBadRequest || detail["code"] != "overview.invalid_window" || errorField(t, rec) != "window" {
			t.Errorf("window %q = %d %v, want 400 overview.invalid_window with field window", w, rec.Code, detail)
		}
	}
	// O texto do erro está nos dois idiomas (a API manda o código, e a tela escreve a mensagem do catálogo).
	for lang, text := range map[string]string{
		"pt-BR": "janela inválida (window): use month, last_month ou year",
		"en":    "invalid window (window): use month, last_month or year",
	} {
		if script := do(e, "GET", "/i18n/"+lang+".js", "", "").Body.String(); !strings.Contains(script, text) {
			t.Errorf("the %s catalog has no text for overview.invalid_window", lang)
		}
	}
	if got := status(e, "GET", path, "", ""); got != http.StatusUnauthorized {
		t.Errorf("no session = %d, want 401", got)
	}
	if got := status(e, "GET", path, "", other.session); got != http.StatusNotFound {
		t.Errorf("another organization's id = %d, want 404", got)
	}
	// Os dados de uma organização não aparecem na da outra.
	hourlyCustomerWithHours(t, other.orgID, other.id)
	if seconds := decode(t, do(e, "GET", path+"?window=last_month", "", owner.session))["totals"].(map[string]any)["seconds"]; seconds != float64(0) {
		t.Errorf("another organization's hours leaked in: %v", seconds)
	}
}

// Fora do modo solo a rota não existe, nem para o dono (404 request.not_found); quem não é o dono recebe 403 antes disso.
// Desligar o modo a esconde na hora, e ligá-lo de novo a traz de volta.
func TestSoloOverview_OnlyTheOwnerOfASoloOrganization(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	admin := invite(t, e, owner, "helena@test.com", "admin")
	member := invite(t, e, owner, "bruno@test.com", "member")
	path := soloOverviewPath(owner)

	if code, errorCode := errCode(t, e, "GET", path, "", owner.session); code != http.StatusNotFound || errorCode != "request.not_found" {
		t.Errorf("the owner outside the solo mode = %d %s, want 404 request.not_found", code, errorCode)
	}
	setSolo(t, owner.orgID, true)
	for _, p := range []struct {
		name    string
		session string
	}{{"admin", admin.session}, {"member", member.session}} {
		if code, errorCode := errCode(t, e, "GET", path, "", p.session); code != http.StatusForbidden || errorCode != "auth.permission_required" {
			t.Errorf("%s = %d %s, want 403 auth.permission_required", p.name, code, errorCode)
		}
	}
	if got := status(e, "GET", path, "", owner.session); got != http.StatusOK {
		t.Errorf("the owner in the solo mode = %d, want 200", got)
	}
	setSolo(t, owner.orgID, false)
	if got := status(e, "GET", path, "", owner.session); got != http.StatusNotFound {
		t.Errorf("after turning the mode off = %d, want 404", got)
	}
	setSolo(t, owner.orgID, true)
	if got := status(e, "GET", path, "", owner.session); got != http.StatusOK {
		t.Errorf("after turning the mode on again = %d, want 200", got)
	}
}

// A página inicial do solo traz a visão geral do solo no lugar da com equipe: as quatro contas, a tabela por cliente e o
// seletor de janela, em português e em inglês, sem uma palavra de equipe, de custo ou de margem e sem link para as páginas
// de equipe (que respondem 404 no solo). O painel da própria pessoa fica.
func TestSoloOverview_SoloHomePage(t *testing.T) {
	e := newServer(t)
	solo := soloSignup(t, e, "Solo", "ana@solo.com")
	home := "/orgs/" + solo.orgID

	for _, c := range []struct {
		lang  string
		want  []string
		gone  []string
		title string
	}{
		{
			lang: "pt-BR",
			want: []string{
				`id="home-solo-title"`, "Visão geral", `aria-label="Janela"`, `x-for="o in soloWindows"`, `@click="setSoloWindow(o.key)"`,
				"Horas", "Receita", "Receita projetada", "Valor por hora efetivo", "Por cliente", "Sem cliente (internos)",
				"<th>Cliente</th>", "<th>Tipo</th>", `<th class="num">Horas</th>`, `<th class="num">Projetada</th>`, "Nada neste período",
				`id="home-me-title"`, "Seu painel",
			},
			gone: []string{kpiLabel("Horas da equipe"), kpiLabel("Trabalhando agora"), "Equipe", kpiLabel("Custo"), kpiLabel("Margem"), kpiLabel("Suas horas"), `id="home-stats-title"`, `id="home-team-title"`, "7 dias", "30 dias"},
		},
		{
			lang: "en",
			want: []string{
				`id="home-solo-title"`, "Overview", `aria-label="Window"`, "Hours", "Revenue", "Projected revenue", "Effective hourly rate", "By customer",
				"No customer (internal)", "<th>Customer</th>", "<th>Type</th>", "Nothing in this period", `id="home-me-title"`, "Your dashboard",
			},
			gone: []string{kpiLabel("Team hours"), kpiLabel("Working now"), kpiLabel("Cost"), kpiLabel("Margin"), kpiLabel("Your hours"), `id="home-stats-title"`, `id="home-team-title"`, "7 days", "30 days"},
		},
	} {
		rec := getPage(e, home, solo.session, c.lang, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: GET %s = %d", c.lang, home, rec.Code)
		}
		page := rec.Body.String()
		// O texto da página, sem o menu e sem as boas-vindas (que têm palavras próprias): do <main> até o modal.
		start, end := strings.Index(page, `<main class="page" x-data="orgHome">`), strings.Index(page, onboardingMarkup)
		if start < 0 || end < start {
			t.Fatalf("%s: no home content before the welcome in the page", c.lang)
		}
		content := page[start:end]
		for _, want := range c.want {
			if !strings.Contains(content, want) {
				t.Errorf("%s: the solo home does not contain %q", c.lang, want)
			}
		}
		for _, gone := range c.gone {
			if strings.Contains(content, gone) {
				t.Errorf("%s: the solo home contains %q", c.lang, gone)
			}
		}
		for _, link := range internalLinks(page) {
			if teamOnly(link) {
				t.Errorf("%s: the solo home links to %s, which is a 404 in the solo mode", c.lang, link)
			}
		}
		for _, bad := range []string{"/people", "/payments", "/collaborators", "/teams"} {
			if strings.Contains(content, bad) {
				t.Errorf("%s: the solo home mentions %q", c.lang, bad)
			}
		}
	}

	// O componente da página lê a rota certa só no modo solo; a organização com equipe continua lendo a visão geral dela.
	script := do(e, "GET", "/static/pages/org.js", "", "").Body.String()
	for _, want := range []string{
		"'/api/orgs/' + orgId + '/solo/overview?window=' + key", "me.organization_solo ? this.loadSolo() : this.loadStats()",
		"'/api/orgs/' + orgId + '/overview'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("org.js does not contain %q", want)
		}
	}
	// O parcial e o script não escrevem à mão o caminho de uma página de equipe.
	for _, file := range []string{"templates/partials/solo_home.gohtml"} {
		src, err := web.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"/people", "/payments", "/collaborators", "management/teams", "/teams"} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s mentions %q, a page that is a 404 in the solo mode", file, bad)
			}
		}
	}
}

// A organização com equipe fica com a tela inicial de sempre, sem nada do solo, e o colaborador nem recebe o bloco. Ligar
// e desligar o modo troca a visão geral na mesma sessão.
func TestSoloOverview_TeamHomeIsUnchanged(t *testing.T) {
	e := newServer(t)
	team := signup(t, e, "Equipe", "bia@equipe.com")
	member := invite(t, e, team, "caio@equipe.com", "member")
	home := "/orgs/" + team.orgID

	solo := []string{`home-solo-title`, "soloScope", "soloWindows", "Receita projetada", "Valor por hora efetivo", "Sem cliente (internos)", "/solo/overview", "setSoloWindow"}
	teamPage := do(e, "GET", home, "", team.session).Body.String()
	for _, want := range []string{`id="home-stats-title"`, `id="home-team-title"`, "Horas da equipe", "Trabalhando agora", "Custo", "Margem", `periods`, `setPeriod(o.key)`} {
		if !strings.Contains(teamPage, want) {
			t.Errorf("the team home lost %q", want)
		}
	}
	memberPage := do(e, "GET", home, "", member.session).Body.String()
	for name, page := range map[string]string{"admin": teamPage, "member": memberPage} {
		for _, bad := range solo {
			if strings.Contains(page, bad) {
				t.Errorf("the %s home of an organization with a team contains %q", name, bad)
			}
		}
	}

	// A mesma organização, vista como solo, e de volta: a visão geral troca sem sair da sessão.
	setSolo(t, team.orgID, true)
	if page := do(e, "GET", home, "", team.session).Body.String(); !strings.Contains(page, `id="home-solo-title"`) || strings.Contains(page, `id="home-stats-title"`) {
		t.Error("turned solo on, the home must show the solo overview instead of the team one")
	}
	// O colaborador do mesmo lugar não é admin: nem o solo, nem a visão geral.
	if page := do(e, "GET", home, "", member.session).Body.String(); strings.Contains(page, `id="home-solo-title"`) || strings.Contains(page, `id="home-stats-title"`) {
		t.Error("a member must see neither overview")
	}
	setSolo(t, team.orgID, false)
	if page := do(e, "GET", home, "", team.session).Body.String(); strings.Contains(page, `id="home-solo-title"`) || !strings.Contains(page, `id="home-stats-title"`) {
		t.Error("turned solo off again, the home must be the team overview")
	}
}
