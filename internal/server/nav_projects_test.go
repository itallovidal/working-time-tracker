package server_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
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

// navProjectNames devolve os nomes do menu Projetos da página, na ordem em que o menu os mostra.
func navProjectNames(t *testing.T, body string) []string {
	t.Helper()
	list, ok := bootOf(t, body)["nav_projects"].([]any)
	if !ok {
		t.Fatal("window.BOOT has no nav_projects list (it must be a list, never null)")
	}
	names := []string{}
	for _, it := range list {
		names = append(names, it.(map[string]any)["name"].(string))
	}
	return names
}

// O menu Projetos da barra superior lista os projetos da pessoa em ordem de nome, de qualquer página: todos, para
// o admin, e só os dela, para um membro (vazio, para quem não está em nenhum). O botão fica marcado nas páginas de
// um projeto, o link leva à lista completa, e o nome de um projeto não vira HTML.
func TestPages_MainNavProjectsMenu(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member") // não está em projeto nenhum
	home := "/orgs/" + admin.orgID
	zeta := createEmptyProject(t, e, admin, "Zé & <Cia>")
	beta := createEmptyProject(t, e, admin, "Projeto Beta")
	alfa := createEmptyProject(t, e, admin, "Projeto Alfa")
	allocate(t, e, admin, alfa, bia.id, 2000)
	allocate(t, e, admin, zeta, bia.id, 2000)
	_ = beta

	get := func(path, session string) string {
		t.Helper()
		rec := do(e, "GET", path, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		return rec.Body.String()
	}

	// A lista é por nome, e a do membro tem só os dele.
	if got := navProjectNames(t, get(home, admin.session)); !slices.Equal(got, []string{"Projeto Alfa", "Projeto Beta", "Zé & <Cia>"}) {
		t.Errorf("admin's menu = %v, want the three projects by name", got)
	}
	if got := navProjectNames(t, get(home, bia.session)); !slices.Equal(got, []string{"Projeto Alfa", "Zé & <Cia>"}) {
		t.Errorf("Bia's menu = %v, want only her two projects", got)
	}
	if got := navProjectNames(t, get(home, caio.session)); len(got) != 0 {
		t.Errorf("Caio's menu = %v, want empty", got)
	}
	// A lista vem em qualquer página de quem está logado, não só na primeira tela.
	for _, path := range []string{"/help", "/profile", "/projects/" + alfa + "/tasks", home + "/about"} {
		if got := navProjectNames(t, get(path, bia.session)); len(got) != 2 {
			t.Errorf("Bia's menu on %s = %v, want her two projects", path, got)
		}
	}

	// O menu: o botão, o painel, o link para a lista completa e o texto de quando não há projeto.
	body := get(home, admin.session)
	for _, want := range []string{
		`class="nav-menu"`, `class="nav-menu-btn"`, `aria-controls="nav-projects-panel"`, `id="nav-projects-panel"`,
		`href="` + home + `#home-projects-title"`, ">Ver todos os projetos</a>", "Nenhum projeto ainda",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the admin's page does not contain %q", want)
		}
	}
	if !strings.Contains(get(home, caio.session), "Você ainda não foi adicionado a nenhum projeto") {
		t.Error("the menu of a person with no project does not say why it is empty")
	}
	// O nome do projeto passa pelo JSON do BOOT, e não vira marcação.
	if strings.Contains(body, "<Cia>") {
		t.Error("the project name reached the page as raw HTML")
	}

	// O botão fica marcado só nas páginas de um projeto.
	button := regexp.MustCompile(`<button type="button" class="nav-menu-btn"[^>]*>`)
	if tag := button.FindString(get("/projects/"+alfa+"/tasks", bia.session)); !strings.Contains(tag, `aria-current="true"`) {
		t.Errorf("the Projetos button on a project page = %s, want aria-current", tag)
	}
	if tag := button.FindString(get(home, bia.session)); strings.Contains(tag, "aria-current") {
		t.Errorf("the Projetos button on the home page = %s, want it unmarked", tag)
	}
	// O projeto aberto vai no BOOT, para o menu marcá-lo na lista.
	if project, _ := bootOf(t, get("/projects/"+alfa+"/tasks", bia.session))["project"].(map[string]any); project["id"] != alfa {
		t.Errorf("BOOT project = %v, want the open project %s", project, alfa)
	}

	// Sem login não há barra nem menu.
	if strings.Contains(get("/login", ""), "nav-menu") {
		t.Error("the login page has the projects menu")
	}
}
