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

// A barra superior tem Início, Colaboradores e Organização. Colaboradores leva direto à
// configuração das pessoas, e só aparece para quem pode cuidar delas: admins e quem recebeu essa permissão.
func TestPages_MainNavHasHomePeopleAndOrganization(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	carol := invite(t, e, admin, "carol@test.com", "member")
	home := "/orgs/" + admin.orgID
	if rec := do(e, "PATCH", "/api/persons/"+carol.id+"/permissions", `{"permissions":["people.manage"]}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("grant people.manage = %d: %s", rec.Code, rec.Body.String())
	}

	for _, c := range []struct {
		who, path, session string
		want               []string
	}{
		{"admin on the home page", home, admin.session, []string{"Início " + home + " (current)", "Colaboradores " + home + "/people", "Organização " + home + "/about"}},
		{"admin on the collaborators tab", home + "/people", admin.session, []string{"Início " + home, "Colaboradores " + home + "/people (current)", "Organização " + home + "/about"}},
		{"admin on the about tab", home + "/about", admin.session, []string{"Início " + home, "Colaboradores " + home + "/people", "Organização " + home + "/about (current)"}},
		{"admin on the customers tab", home + "/customers", admin.session, []string{"Início " + home, "Colaboradores " + home + "/people", "Organização " + home + "/about (current)"}},
		{"admin in a project", "/projects/" + createProject(t, e, admin, "Projeto Alfa") + "/tasks", admin.session, []string{"Início " + home, "Colaboradores " + home + "/people", "Organização " + home + "/about"}},
		{"member without the permission", home, member.session, []string{"Início " + home + " (current)", "Organização " + home + "/about"}},
		{"member allowed to manage people", home, carol.session, []string{"Início " + home + " (current)", "Colaboradores " + home + "/people", "Organização " + home + "/about"}},
	} {
		rec := do(e, "GET", c.path, "", c.session)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: GET %s = %d", c.who, c.path, rec.Code)
			continue
		}
		if got := strings.Join(mainNav(t, rec.Body.String()), " | "); got != strings.Join(c.want, " | ") {
			t.Errorf("%s: nav = %q, want %q", c.who, got, strings.Join(c.want, " | "))
		}
	}

	// A aba Colaboradores continua dentro da Organização, e a aba Projetos é a tabela de gestão.
	if body := do(e, "GET", home+"/about", "", admin.session).Body.String(); !strings.Contains(body, `href="`+home+`/people"`) {
		t.Error("the Organização tabs lost the Colaboradores tab")
	}
}
