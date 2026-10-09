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
	for _, m := range regexp.MustCompile(`<a href="([^"]+)"\s*(aria-current="page")?>([^<]+)</a>`).FindAllStringSubmatch(nav, -1) {
		item := m[3] + " " + m[1]
		if m[2] != "" {
			item += " (current)"
		}
		items = append(items, item)
	}
	return items
}

// A barra superior tem uma página por assunto, sem sub-abas: Início, Projetos, Colaboradores, Clientes e
// Configurações. Cada item só aparece para quem pode abrir a página dele: Colaboradores e Clientes, para quem
// cuida deles.
func TestPages_MainNavHasOnePagePerSubject(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	carol := invite(t, e, admin, "carol@test.com", "member")
	dan := invite(t, e, admin, "dan@test.com", "member")
	otherAdmin := invite(t, e, admin, "eva@test.com", "admin")
	home := "/orgs/" + admin.orgID
	for person, key := range map[string]string{carol.id: "people.manage", dan.id: "customers.manage"} {
		if rec := do(e, "PATCH", "/api/persons/"+person+"/permissions", `{"permissions":["`+key+`"]}`, admin.session); rec.Code != http.StatusOK {
			t.Fatalf("grant %s = %d: %s", key, rec.Code, rec.Body.String())
		}
	}
	projectID := createProject(t, e, admin, "Projeto Alfa")

	// all é a barra de um admin, com o item current marcado.
	all := func(current string) []string {
		items := []string{"Início " + home, "Projetos " + home + "/projects", "Colaboradores " + home + "/people", "Clientes " + home + "/customers", "Configurações " + home + "/about"}
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
		{"admin on the projects page", home + "/projects", admin.session, all("Projetos")},
		{"admin in a project", "/projects/" + projectID + "/tasks", admin.session, all("Projetos")},
		{"admin on the collaborators page", home + "/people", admin.session, all("Colaboradores")},
		{"admin on the customers page", home + "/customers", admin.session, all("Clientes")},
		{"admin on the settings page", home + "/about", admin.session, all("Configurações")},
		{"admin on the organization edit page", home + "/settings", admin.session, all("Configurações")},
		{"an admin who is not the owner", home, otherAdmin.session, all("Início")},
		{"admin on the help page", "/help", admin.session, all("")},
		{"admin on the profile page", "/profile", admin.session, all("")},
		{"member without a permission", home, member.session, []string{"Início " + home + " (current)", "Projetos " + home + "/projects", "Configurações " + home + "/about"}},
		{"member allowed to manage people", home, carol.session, []string{"Início " + home + " (current)", "Projetos " + home + "/projects", "Colaboradores " + home + "/people", "Configurações " + home + "/about"}},
		{"member allowed to manage customers", home, dan.session, []string{"Início " + home + " (current)", "Projetos " + home + "/projects", "Clientes " + home + "/customers", "Configurações " + home + "/about"}},
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
		topbar := body[strings.Index(body, `<header class="topbar">`):strings.Index(body, `</header>`)]
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
