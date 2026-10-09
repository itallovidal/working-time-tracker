package server_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// bootOf lê o window.BOOT que a página entrega ao JavaScript.
func bootOf(t *testing.T, body string) map[string]any {
	t.Helper()
	const prefix = "window.BOOT = "
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatal("the page has no window.BOOT")
	}
	j := strings.Index(body[i:], ";</script>")
	var boot map[string]any
	if err := json.Unmarshal([]byte(body[i+len(prefix):i+j]), &boot); err != nil {
		t.Fatalf("decode window.BOOT: %v", err)
	}
	return boot
}

// Projetos, na barra superior, é um link para a página dos projetos (não mais um menu suspenso): fica marcado nela e
// dentro de um projeto, e nenhuma página carrega mais a lista de projetos para o menu.
func TestPages_MainNavProjectsIsALink(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	home := "/orgs/" + admin.orgID
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	allocate(t, e, admin, alfa, bia.id, 2000)

	get := func(path, session string) string {
		t.Helper()
		rec := do(e, "GET", path, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}

	link := regexp.MustCompile(`<a href="` + regexp.QuoteMeta(home+"/projects") + `"\s*(aria-current="page")?>Projetos</a>`)
	for path, marked := range map[string]bool{
		home:                           false,
		home + "/projects":             true,
		"/projects/" + alfa + "/tasks": true,
		"/profile":                     false,
		"/help":                        false,
	} {
		body := get(path, bia.session)
		m := link.FindStringSubmatch(body)
		if m == nil {
			t.Errorf("GET %s has no Projetos link in the top bar", path)
			continue
		}
		if got := m[1] != ""; got != marked {
			t.Errorf("GET %s: Projetos marked = %v, want %v", path, got, marked)
		}
		// O menu suspenso e a lista que ele lia saíram.
		for _, gone := range []string{"nav-projects-panel", "navProjects", "Ver todos os projetos"} {
			if strings.Contains(body, gone) {
				t.Errorf("GET %s still has %q from the old projects menu", path, gone)
			}
		}
		if _, has := bootOf(t, body)["nav_projects"]; has {
			t.Errorf("GET %s still sends nav_projects in window.BOOT", path)
		}
	}

	// O projeto aberto continua no BOOT, para os componentes da página dele.
	if project, _ := bootOf(t, get("/projects/"+alfa+"/tasks", bia.session))["project"].(map[string]any); project["id"] != alfa {
		t.Errorf("BOOT project = %v, want the open project %s", project, alfa)
	}
	// "Projetos", no caminho do cabeçalho de um projeto, leva à página dos projetos.
	if body := get("/projects/"+alfa+"/tasks", bia.session); !strings.Contains(body, `<div class="eyebrow"><a href="`+home+`/projects">Projetos</a></div>`) {
		t.Error("the project header does not link back to the projects page")
	}
	// O JavaScript da barra não tem mais o componente do menu.
	if script := get("/static/app.js", ""); strings.Contains(script, "navProjects") || strings.Contains(script, "nav_projects") {
		t.Error("app.js still has the projects menu component")
	}
}
