package server_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// mainNav devolve os itens da barra superior: o texto e se está marcado como a página atual.
func mainNav(t *testing.T, body string) []string {
	t.Helper()
	start := strings.Index(body, `<nav class="mainnav"`)
	if start < 0 {
		t.Fatal("the page has no main navigation")
	}
	nav := body[start : start+strings.Index(body[start:], "</nav>")]
	var items []string
	// Cada item tem o seu ícone, que só aparece no painel do menu de hambúrguer.
	for _, m := range regexp.MustCompile(`<a href="([^"]+)"\s*(aria-current="page")?><i class="fa-solid fa-[a-z-]+" aria-hidden="true"></i> ([^<]+)</a>`).FindAllStringSubmatch(nav, -1) {
		item := m[3] + " " + m[1]
		if m[2] != "" {
			item += " (current)"
		}
		items = append(items, item)
	}
	return items
}

// A barra superior tem uma página por assunto, sem sub-abas: Início, Clientes e Projetos, Colaboradores, Pagamentos
// e Configurações. Cada item só aparece para quem pode abrir a página dele: Clientes e Projetos para todos (os clientes,
// dentro dela, só para o dono), Colaboradores para quem cuida deles, Pagamentos (a equipe, que é dinheiro de todos) só
// para o dono. O admin que não é o dono vê Colaboradores, mas não Pagamentos.
func TestPages_MainNavHasOnePagePerSubject(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	otherAdmin := invite(t, e, admin, "eva@test.com", "admin")
	home := "/orgs/" + admin.orgID
	projectID := createProject(t, e, admin, "Projeto Alfa")

	// all é a barra de um admin, com o item current marcado.
	all := func(current string) []string {
		items := []string{"Início " + home, "Clientes e Projetos " + home + "/projects", "Colaboradores " + home + "/people", "Pagamentos " + home + "/payments", "Configurações " + home + "/about"}
		for i, it := range items {
			if strings.HasPrefix(it, current+" ") {
				items[i] += " (current)"
			}
		}
		return items
	}
	for _, c := range []struct {
		who, path, session string
		want               []string
	}{
		{"admin on the home page", home, admin.session, all("Início")},
		{"admin on the customers and projects page", home + "/projects", admin.session, all("Clientes e Projetos")},
		{"admin in a project", "/projects/" + projectID + "/tasks", admin.session, all("Clientes e Projetos")},
		{"admin on the collaborators page", home + "/people", admin.session, all("Colaboradores")},
		{"admin on a collaborator's profile", home + "/people/" + member.id, admin.session, all("Colaboradores")},
		{"admin on the payments page", home + "/payments", admin.session, all("Pagamentos")},
		{"admin on the settings page", home + "/about", admin.session, all("Configurações")},
		{"admin on the organization edit page", home + "/settings", admin.session, all("Configurações")},
		{"an admin who is not the owner", home, otherAdmin.session, []string{"Início " + home + " (current)", "Clientes e Projetos " + home + "/projects", "Colaboradores " + home + "/people", "Configurações " + home + "/about"}},
		{"admin on the help page", "/help", admin.session, all("")},
		{"admin on the profile page", "/profile", admin.session, all("")},
		{"member without a permission", home, member.session, []string{"Início " + home + " (current)", "Clientes e Projetos " + home + "/projects", "Configurações " + home + "/about"}},
	} {
		rec := do(e, "GET", c.path, "", c.session)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: GET %s = %d", c.who, c.path, rec.Code)
			continue
		}
		body := rec.Body.String()
		if got := strings.Join(mainNav(t, body), " | "); got != strings.Join(c.want, " | ") {
			t.Errorf("%s: nav = %q, want %q", c.who, got, strings.Join(c.want, " | "))
		}
		// Nenhuma página tem mais a faixa de abas da Organização, e a barra não tem o ícone ao lado do nome.
		if strings.Contains(body, "Seções da organização") {
			t.Errorf("%s: GET %s still has the organization tab strip", c.who, c.path)
		}
		topbar := body[strings.Index(body, `<header class="topbar"`):strings.Index(body, `</header>`)]
		if strings.Contains(topbar, "brand-mark") || !strings.Contains(topbar, `<a class="brand" href="`+home+`"><span data-org-name>Org</span></a>`) {
			t.Errorf("%s: GET %s: the brand should be just the organization name, without the icon", c.who, c.path)
		}
	}
}

// O menu da conta, no nome de quem está logado, junta o que é da pessoa: o perfil, a ajuda, o idioma e a saída. Fica
// marcado no Perfil e na Ajuda, que não têm item próprio na barra.
func TestPages_AccountMenu(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	home := "/orgs/" + admin.orgID

	menu := func(path string) string {
		t.Helper()
		body := do(e, "GET", path, "", admin.session).Body.String()
		start := strings.Index(body, `class="nav-menu user-menu"`)
		if start < 0 {
			t.Fatalf("GET %s has no account menu", path)
		}
		return body[start : start+strings.Index(body[start:], "</header>")]
	}
	onHome := menu(home)
	for _, want := range []string{
		`x-data="userMenu"`, `aria-controls="user-menu-panel"`, `id="user-menu-panel"`, `data-me-name>Admin</span>`,
		`href="/profile"`, "Perfil", `href="/help"`, "Ajuda", `class="lang-switch"`, `href="/lang/en?next=`, `@click="logout()"`, "Sair",
	} {
		if !strings.Contains(onHome, want) {
			t.Errorf("the account menu does not contain %q", want)
		}
	}
	button := regexp.MustCompile(`<button type="button" class="nav-menu-btn user-menu-btn"[^>]*>`)
	if tag := button.FindString(onHome); tag == "" || strings.Contains(tag, "aria-current") {
		t.Errorf("the account button on the home page = %q, want it there and unmarked", tag)
	}
	for _, path := range []string{"/profile", "/help"} {
		on := menu(path)
		if tag := button.FindString(on); !strings.Contains(tag, `aria-current="true"`) {
			t.Errorf("the account button on %s = %q, want it marked", path, tag)
		}
		if !strings.Contains(on, `href="`+path+`" aria-current="page"`) {
			t.Errorf("the account menu on %s does not mark its own item", path)
		}
	}

	// O perfil, a ajuda, o idioma e a saída saíram da barra: só estão no menu.
	body := do(e, "GET", home, "", admin.session).Body.String()
	nav := body[strings.Index(body, `<nav class="mainnav"`):]
	nav = nav[:strings.Index(nav, "</nav>")]
	for _, gone := range []string{"/help", "/profile", "lang-switch", "logout"} {
		if strings.Contains(nav, gone) {
			t.Errorf("the main navigation still has %q", gone)
		}
	}
	// Sem login não há barra nem menu.
	if strings.Contains(do(e, "GET", "/login", "", "").Body.String(), "nav-menu") {
		t.Error("the login page has the account menu")
	}
}

// Abaixo de 1024px os itens da barra vão para um painel: o botão de hambúrguer, antes do nome da organização, abre o
// #mainnav (aria-controls, aria-expanded) e tem um nome para o leitor de tela; o véu atrás do painel fecha ao toque.
func TestPages_MainNavMenuButton(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	body := do(e, "GET", "/orgs/"+admin.orgID, "", admin.session).Body.String()
	topbar := body[strings.Index(body, `<header class="topbar"`):strings.Index(body, `</header>`)]

	button := regexp.MustCompile(`<button type="button" class="mainnav-toggle"[^>]*>`).FindString(topbar)
	for _, want := range []string{`aria-controls="mainnav"`, `:aria-expanded="open.toString()"`, `@click="toggle()"`} {
		if !strings.Contains(button, want) {
			t.Errorf("the menu button = %q, want it to contain %q", button, want)
		}
	}
	for _, want := range []string{`x-data="mainNav"`, `<span class="sr-only">Menu de páginas</span>`, `<nav class="mainnav" id="mainnav"`, `class="mainnav-scrim"`} {
		if !strings.Contains(topbar, want) {
			t.Errorf("the top bar does not contain %q", want)
		}
	}
	if strings.Index(topbar, `class="mainnav-toggle"`) > strings.Index(topbar, `class="brand"`) {
		t.Error("the menu button should come before the organization name")
	}
}
