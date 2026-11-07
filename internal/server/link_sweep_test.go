package server_test

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// A varredura de links. O modo solo esconde páginas (as de equipe) e mostra outras (A receber); o que esconde pode
// continuar apontado por um link esquecido, e o que mostra pode faltar num menu. Estes testes abrem as páginas
// principais como o dono de uma organização solo e de uma com equipe, extraem todo link interno que a página entrega
// (a barra superior e as abas do projeto inclusive), seguem cada um com a sessão do dono e conferem três regras:
//
//  1. nenhum link dá 404 (nem qualquer coisa que não seja uma página): quem clica não cai numa "Página não encontrada";
//  2. no modo solo nenhum link aponta para as páginas de equipe (Colaboradores, Pagamentos, a aba Colaboradores e
//     Times do projeto);
//  3. fora do modo solo nenhum link aponta para A receber.
//
// A segunda metade é a matriz de quem entra direto pela URL: sem sessão, como dono solo e como dono com equipe.

// sweepOrg é uma organização montada para a varredura: o dono, um cliente, um projeto e uma tarefa.
type sweepOrg struct {
	label   string
	solo    bool
	owner   account
	project string
	task    string
}

// newSweepOrg monta uma organização solo ou com equipe, cada uma com cliente por hora, projeto com o valor cobrado e
// uma tarefa. A com equipe ganha também um colaborador no projeto, para as páginas de pessoas terem o que listar.
func newSweepOrg(t *testing.T, e *echo.Echo, solo bool) sweepOrg {
	t.Helper()
	o := sweepOrg{label: "team", solo: solo}
	if solo {
		o.label = "solo"
		o.owner = soloSignup(t, e, "Estúdio Solo", "dono@solo.test")
	} else {
		o.owner = signup(t, e, "Software House", "dono@equipe.test")
	}
	rec := do(e, "POST", "/api/orgs/"+o.owner.orgID+"/customers", `{"name":"Cliente da varredura"}`, o.owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%s: create customer = %d: %s", o.label, rec.Code, rec.Body.String())
	}
	customer := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/orgs/"+o.owner.orgID+"/projects",
		`{"name":"Projeto da varredura","customer_id":"`+customer+`","bill_rate_cents":15000}`, o.owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%s: create project = %d: %s", o.label, rec.Code, rec.Body.String())
	}
	o.project = decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/projects/"+o.project+"/tasks", `{"name":"Tarefa da varredura"}`, o.owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("%s: create task = %d: %s", o.label, rec.Code, rec.Body.String())
	}
	o.task = decode(t, rec)["id"].(string)
	if !solo {
		member := invite(t, e, o.owner, "colega@equipe.test", "member")
		allocate(t, e, o.owner, o.project, member.id, 4000)
	}
	return o
}

// pages são as páginas que o dono abre para a varredura. As de equipe só entram na organização com equipe: no modo solo
// elas respondem 404, e a matriz de endereços diretos é quem confere isso.
func (o sweepOrg) pages() []string {
	org, prj := "/orgs/"+o.owner.orgID, "/projects/"+o.project
	pages := []string{
		org, org + "/about", org + "/settings", org + "/projects", org + "/customers", "/profile", "/help",
		prj + "/overview", prj + "/my-tasks", prj + "/tasks",
		prj + "/management/overview", prj + "/management/integrations", prj + "/management/settings",
		"/tasks/" + o.task,
	}
	if o.solo {
		return append(pages, org+"/receivables")
	}
	return append(pages, org+"/people", org+"/people/"+o.owner.id, org+"/payments", prj+"/collaborators", prj+"/management/teams")
}

var (
	// href="/..." escrito no HTML, não o :href do Alpine (o ":" na frente tira o espaço que a expressão exige).
	staticHrefRE = regexp.MustCompile(`(?:^|[\s"'])href\s*=\s*"([^"]*)"`)
	// :href="expressão" do Alpine, e location.href = expressão dos @click das linhas clicáveis.
	dynamicHrefRE = regexp.MustCompile(`:href\s*=\s*"([^"]*)"`)
	locationRE    = regexp.MustCompile(`location\.href\s*=\s*([^"]*)"`)
	// Um pedaço de uma expressão feita de textos e valores somados: '/tasks/' + t.id + '/x'.
	quotedRE = regexp.MustCompile(`^'([^']*)'$`)
	idAttrRE = regexp.MustCompile(`\sid\s*=\s*"([^"]*)"`)

	// As páginas de equipe, pelo caminho (sem a query): a organização solo não aponta para nenhuma delas.
	teamPageRE = regexp.MustCompile(`^(/orgs/[^/]+/(people|payments)(/[^/]+)?|/projects/[^/]+/(collaborators|teams|management/teams))$`)
	// O A receber, que é só do modo solo.
	receivablesPageRE = regexp.MustCompile(`^/orgs/[^/]+/receivables$`)
)

// sweepLink é um link interno achado numa página, já sem o fragmento.
type sweepLink struct {
	href string // o caminho com a query, como o navegador o pediria
	from string // a página em que apareceu
}

func (l sweepLink) path() string {
	u, err := url.Parse(l.href)
	if err != nil {
		return l.href
	}
	return u.Path
}

// internalLink diz se o href leva a uma página do próprio sistema (e não a outro site, a um esquema ou a um trecho da
// mesma página).
func internalLink(href string) bool {
	return strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//")
}

// withoutFragment tira o #trecho: o servidor nunca o recebe.
func withoutFragment(href string) string {
	if i := strings.Index(href, "#"); i >= 0 {
		return href[:i]
	}
	return href
}

// resolveDynamic monta o endereço de uma expressão do Alpine que só soma textos e ids ('/tasks/' + t.id +
// '/overview'), pondo em cada valor o id do recurso que o texto antes dele nomeia (/tasks/ é a tarefa, /projects/ o
// projeto, /orgs/ a organização, /people/ a pessoa). Uma expressão que chama função ou lê de outro lugar (profileHref(p),
// link.url) não se resolve sem os dados da tela, e volta ok falso.
func resolveDynamic(expr string, ids map[string]string) (string, bool) {
	var out strings.Builder
	for _, part := range strings.Split(expr, " + ") {
		part = strings.TrimSpace(part)
		if m := quotedRE.FindStringSubmatch(part); m != nil {
			out.WriteString(m[1])
			continue
		}
		if strings.ContainsAny(part, "()?:") {
			return "", false
		}
		before := out.String()
		resolved := false
		for prefix, id := range ids {
			if strings.HasSuffix(before, "/"+prefix+"/") {
				out.WriteString(id)
				resolved = true
				break
			}
		}
		if !resolved {
			return "", false
		}
	}
	return out.String(), true
}

// linksIn extrai os links internos do HTML: os href escritos, e os que o Alpine monta com somas de texto e id. Os
// que ele monta de outro jeito vão em unresolved, para a varredura dizer o que não cobriu.
func linksIn(body, from string, ids map[string]string) (links []sweepLink, anchors, unresolved []string) {
	add := func(href string) {
		href = html.UnescapeString(strings.TrimSpace(href))
		switch {
		case strings.HasPrefix(href, "#") && len(href) > 1:
			anchors = append(anchors, href[1:])
		case internalLink(href):
			links = append(links, sweepLink{href: withoutFragment(href), from: from})
		}
	}
	for _, m := range staticHrefRE.FindAllStringSubmatch(body, -1) {
		add(m[1])
	}
	for _, re := range []*regexp.Regexp{dynamicHrefRE, locationRE} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			expr := html.UnescapeString(m[1])
			if !strings.HasPrefix(strings.TrimSpace(expr), "'/") {
				unresolved = append(unresolved, strings.TrimSpace(expr))
				continue
			}
			if href, ok := resolveDynamic(expr, ids); ok {
				add(href)
			} else {
				unresolved = append(unresolved, strings.TrimSpace(expr))
			}
		}
	}
	return links, anchors, unresolved
}

// follow segue um link com a sessão, atravessando os redirecionamentos, e devolve o status da página em que parou.
func follow(e *echo.Echo, href, session string) (status int, final string) {
	final = href
	for hop := 0; hop < 6; hop++ {
		rec := do(e, "GET", final, "", session)
		if rec.Code < 300 || rec.Code >= 400 {
			return rec.Code, final
		}
		next := rec.Header().Get("Location")
		if next == "" {
			return rec.Code, final
		}
		final = next
	}
	return 0, final
}

// sweepFindings aplica as três regras a um conjunto de links e devolve o que as quebra, uma linha por problema.
func sweepFindings(e *echo.Echo, o sweepOrg, links []sweepLink) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range links {
		if seen[l.href] {
			continue
		}
		seen[l.href] = true
		if o.solo && teamPageRE.MatchString(l.path()) {
			out = append(out, fmt.Sprintf("solo: %s links to the team page %s", l.from, l.href))
		}
		if !o.solo && receivablesPageRE.MatchString(l.path()) {
			out = append(out, fmt.Sprintf("team: %s links to the solo-only page %s", l.from, l.href))
		}
		if status, final := follow(e, l.href, o.owner.session); status != http.StatusOK {
			out = append(out, fmt.Sprintf("%s: %s links to %s, which ends at %s with %d, want 200", o.label, l.from, l.href, final, status))
		}
	}
	sort.Strings(out)
	return out
}

// TestLinkSweep_EveryLinkOfTheOwnerLeadsToAPage abre as páginas principais como dono, de uma organização solo e de uma
// com equipe, e segue cada link interno delas.
func TestLinkSweep_EveryLinkOfTheOwnerLeadsToAPage(t *testing.T) {
	e := newServer(t)
	for _, o := range []sweepOrg{newSweepOrg(t, e, true), newSweepOrg(t, e, false)} {
		ids := map[string]string{"tasks": o.task, "projects": o.project, "orgs": o.owner.orgID, "people": o.owner.id}
		var links []sweepLink
		hrefs := map[string]bool{}
		for _, page := range o.pages() {
			rec := do(e, "GET", page, "", o.owner.session)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: GET %s = %d, want 200", o.label, page, rec.Code)
				continue
			}
			body := rec.Body.String()
			found, anchors, unresolved := linksIn(body, page, ids)
			if len(found) == 0 {
				t.Errorf("%s: %s has no internal link at all, the extraction is not reading the page", o.label, page)
			}
			links = append(links, found...)
			for _, l := range found {
				hrefs[l.href] = true
			}
			// Um link de trecho (#ancora) tem de achar o elemento na mesma página: o sumário da ajuda depende disso.
			present := map[string]bool{}
			for _, m := range idAttrRE.FindAllStringSubmatch(body, -1) {
				present[m[1]] = true
			}
			for _, a := range anchors {
				if !present[a] {
					t.Errorf("%s: %s has the link #%s, but no element with that id", o.label, page, a)
				}
			}
			if len(unresolved) > 0 {
				t.Logf("%s: %s: links built from page data, not followed: %s", o.label, page, strings.Join(unresolved, "; "))
			}
		}

		// A varredura precisa enxergar o menu e as abas: se a extração parar de ler a barra superior ou a de abas,
		// as regras abaixo passariam por não ter o que conferir.
		org, prj := "/orgs/"+o.owner.orgID, "/projects/"+o.project
		for _, want := range []string{
			org, org + "/projects", org + "/customers", org + "/about", "/profile", "/help",
			prj + "/overview", prj + "/my-tasks", prj + "/tasks", prj + "/management", "/tasks/" + o.task,
			prj + "/management/overview", prj + "/management/integrations", prj + "/management/settings",
		} {
			if !hrefs[want] {
				t.Errorf("%s: the sweep never saw a link to %s", o.label, want)
			}
		}
		for path, want := range map[string]bool{
			org + "/receivables": o.solo, org + "/people": !o.solo, org + "/payments": !o.solo,
			prj + "/collaborators": !o.solo, prj + "/management/teams": !o.solo,
		} {
			if hrefs[path] != want {
				t.Errorf("%s: link to %s present = %v, want %v", o.label, path, hrefs[path], want)
			}
		}

		for _, finding := range sweepFindings(e, o, links) {
			t.Error(finding)
		}
	}
}

// Os detectores não adiantam se não pegam nada: este teste dá a eles páginas com um link ruim de cada tipo e confere
// que cada um é apontado, e que um link bom passa. É o que a mutação à mão (pôr um link ruim num template e ver a
// varredura falhar) prova sobre os templates de verdade.
func TestLinkSweep_DetectsBadLinks(t *testing.T) {
	e := newServer(t)
	solo := newSweepOrg(t, e, true)
	team := newSweepOrg(t, e, false)
	ids := func(o sweepOrg) map[string]string {
		return map[string]string{"tasks": o.task, "projects": o.project, "orgs": o.owner.orgID, "people": o.owner.id}
	}
	page := func(hrefs ...string) string {
		var b strings.Builder
		for _, h := range hrefs {
			b.WriteString(`<a href="` + h + `">x</a>`)
		}
		return b.String()
	}

	for _, c := range []struct {
		name string
		o    sweepOrg
		body string
		want []string // trechos que as mensagens têm de conter, uma por problema
	}{
		{"a good page has nothing to report", solo, page("/orgs/"+solo.owner.orgID+"/receivables", "/help#roles", "https://example.com", "mailto:a@b.c", "#top"), nil},
		{"a link that is a 404", solo, page("/orgs/" + solo.owner.orgID + "/nao-existe"), []string{"ends at /orgs/" + solo.owner.orgID + "/nao-existe with 404"}},
		{"a made-up id", solo, page("/projects/00000000-0000-0000-0000-000000000000/overview"), []string{"with 404"}},
		{"the solo menu pointing at Colaboradores", solo, page("/orgs/" + solo.owner.orgID + "/people"), []string{"links to the team page", "with 404"}},
		{"the solo menu pointing at Pagamentos", solo, page("/orgs/" + solo.owner.orgID + "/payments"), []string{"links to the team page", "with 404"}},
		{"a solo project tab pointing at Colaboradores", solo, page("/projects/" + solo.project + "/collaborators"), []string{"links to the team page", "with 404"}},
		{"a solo project tab pointing at the Gestão teams tab", solo, page("/projects/" + solo.project + "/management/teams?view=teams"), []string{"links to the team page", "with 404"}},
		{"the old /teams address in solo", solo, page("/projects/" + solo.project + "/teams"), []string{"links to the team page", "with 404"}},
		{"a person profile in solo", solo, page("/orgs/" + solo.owner.orgID + "/people/" + solo.owner.id), []string{"links to the team page", "with 404"}},
		{"the team menu pointing at A receber", team, page("/orgs/" + team.owner.orgID + "/receivables"), []string{"links to the solo-only page", "with 404"}},
		{"the team menu is fine with its own pages", team, page("/orgs/"+team.owner.orgID+"/people", "/orgs/"+team.owner.orgID+"/payments", "/projects/"+team.project+"/collaborators"), nil},
	} {
		links, _, _ := linksIn(c.body, "/fake", ids(c.o))
		got := sweepFindings(e, c.o, links)
		if len(got) != len(c.want) {
			t.Errorf("%s: %d finding(s) %q, want %d", c.name, len(got), got, len(c.want))
			continue
		}
		joined := strings.Join(got, "\n")
		for _, w := range c.want {
			if !strings.Contains(joined, w) {
				t.Errorf("%s: findings %q lack %q", c.name, got, w)
			}
		}
	}

	// As expressões do Alpine que somam texto e id viram um endereço; as que chamam função ficam de fora, avisadas.
	body := `<a :href="'/tasks/' + t.id">x</a><a :href="'/projects/' + r.id + '/overview'">y</a>` +
		`<tr @click="if (!$event.target.closest('a, button')) location.href = '/tasks/' + t.id"></tr>` +
		`<a :href="profileHref(p)">z</a><a :href="link.url">w</a>`
	links, _, unresolved := linksIn(body, "/fake", ids(solo))
	var hrefs []string
	for _, l := range links {
		hrefs = append(hrefs, l.href)
	}
	wantHrefs := []string{"/tasks/" + solo.task, "/projects/" + solo.project + "/overview", "/tasks/" + solo.task}
	if strings.Join(hrefs, ",") != strings.Join(wantHrefs, ",") {
		t.Errorf("dynamic hrefs resolved to %q, want %q", hrefs, wantHrefs)
	}
	if len(unresolved) != 2 {
		t.Errorf("unresolved = %q, want the two that call a function or read another value", unresolved)
	}
}

// TestLinkSweep_DirectAddresses é a matriz de quem entra direto pela URL, sem passar pelo menu: cada página de equipe
// e o A receber, sem sessão, como dono de uma organização solo e como dono de uma com equipe.
func TestLinkSweep_DirectAddresses(t *testing.T) {
	e := newServer(t)
	solo := newSweepOrg(t, e, true)
	team := newSweepOrg(t, e, false)

	type want struct {
		status   int
		location string // o fim do redirecionamento, quando o status é um
	}
	for _, c := range []struct {
		name string
		path string // com {org}, {prj} e {person} no lugar dos ids
		solo want
		team want
	}{
		{"Colaboradores", "/orgs/{org}/people", want{404, ""}, want{200, ""}},
		{"Colaboradores, com o modal de convite", "/orgs/{org}/people?add=1", want{404, ""}, want{200, ""}},
		{"o perfil de um colaborador", "/orgs/{org}/people/{person}", want{404, ""}, want{200, ""}},
		{"Pagamentos", "/orgs/{org}/payments", want{404, ""}, want{200, ""}},
		{"a aba Colaboradores do projeto", "/projects/{prj}/collaborators", want{404, ""}, want{200, ""}},
		{"a aba Colaboradores da Gestão", "/projects/{prj}/management/teams", want{404, ""}, want{200, ""}},
		{"a aba Colaboradores da Gestão, na visão de times", "/projects/{prj}/management/teams?view=teams", want{404, ""}, want{200, ""}},
		{"o endereço antigo /teams", "/projects/{prj}/teams", want{404, ""}, want{303, "/projects/{prj}/management/teams"}},
		{"A receber", "/orgs/{org}/receivables", want{200, ""}, want{404, ""}},
	} {
		for _, who := range []struct {
			o    sweepOrg
			want want
		}{{solo, c.solo}, {team, c.team}} {
			fill := strings.NewReplacer("{org}", who.o.owner.orgID, "{prj}", who.o.project, "{person}", who.o.owner.id)
			path := fill.Replace(c.path)

			// Sem sessão, a página leva ao login e volta para ela depois: nenhuma revela se existe.
			rec := do(e, "GET", path, "", "")
			if wantLocation := "/login?next=" + url.QueryEscape(path); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != wantLocation {
				t.Errorf("%s, no session, %s: GET %s = %d to %q, want 303 to %q", c.name, who.o.label, path, rec.Code, rec.Header().Get("Location"), wantLocation)
			}

			rec = do(e, "GET", path, "", who.o.owner.session)
			if rec.Code != who.want.status {
				t.Errorf("%s, owner of the %s organization: GET %s = %d, want %d", c.name, who.o.label, path, rec.Code, who.want.status)
				continue
			}
			switch who.want.status {
			case http.StatusNotFound:
				if !strings.Contains(rec.Body.String(), "Página não encontrada") {
					t.Errorf("%s, owner of the %s organization: GET %s is a 404 without the not found page", c.name, who.o.label, path)
				}
			case http.StatusSeeOther:
				if got, want := rec.Header().Get("Location"), fill.Replace(who.want.location); got != want {
					t.Errorf("%s, owner of the %s organization: GET %s redirects to %q, want %q", c.name, who.o.label, path, got, want)
				}
			}
		}
	}

	// Outra organização não abre o A receber de ninguém: a mesma página não encontrada, solo ou não.
	for _, o := range []sweepOrg{solo, team} {
		other := team
		if o.label == "team" {
			other = solo
		}
		if rec := do(e, "GET", "/orgs/"+o.owner.orgID+"/receivables", "", other.owner.session); rec.Code != http.StatusNotFound {
			t.Errorf("the owner of the %s organization opens the receivables of the %s one: %d, want 404", other.label, o.label, rec.Code)
		}
	}
}
