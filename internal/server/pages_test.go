package server_test

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"working-time-tracker/internal/template"
	"working-time-tracker/web"
)

// pagePaths são as páginas logadas de uma organização com um projeto que todo
// mundo vê. As abas de gestão da organização (orgPagePaths) são só de admins.
func pagePaths(orgID, projectID string) []string {
	return []string{
		"/orgs/" + orgID,
		"/orgs/" + orgID + "/about",
		"/profile",
		"/projects/" + projectID + "/tasks",
		"/projects/" + projectID + "/time-tracking",
		"/projects/" + projectID + "/teams",
		"/projects/" + projectID + "/integrations",
		"/projects/" + projectID + "/settings",
	}
}

// orgPagePaths são as abas de gestão da organização: Geral, Pessoas e Projetos.
func orgPagePaths(orgID string) []string {
	return []string{
		"/orgs/" + orgID + "/settings",
		"/orgs/" + orgID + "/people",
		"/orgs/" + orgID + "/projects",
	}
}

func TestPages_RenderForAdminAndMember(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto Alfa")

	for _, who := range []account{admin, member} {
		for _, path := range pagePaths(admin.orgID, projectID) {
			rec := do(e, "GET", path, "", who.session)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
				t.Errorf("GET %s = %d %s, want 200 html", path, rec.Code, rec.Header().Get("Content-Type"))
				continue
			}
			if strings.HasPrefix(path, "/projects/") && !strings.Contains(rec.Body.String(), "Projeto Alfa") {
				t.Errorf("GET %s does not show the project name", path)
			}
		}
	}

	// Ações de admin não aparecem para membros.
	rec := do(e, "GET", "/orgs/"+admin.orgID, "", member.session)
	if strings.Contains(rec.Body.String(), "Novo projeto") {
		t.Error("member sees the 'Novo projeto' button")
	}
	rec = do(e, "GET", "/orgs/"+admin.orgID, "", admin.session)
	if !strings.Contains(rec.Body.String(), "Novo projeto") {
		t.Error("admin does not see the 'Novo projeto' button")
	}
}

// As abas de gestão da organização são só de admins: membros não veem os links
// e recebem "Página não encontrada" se abrirem o endereço.
func TestPages_OrgTabsAreAdminOnly(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")

	for _, path := range orgPagePaths(admin.orgID) {
		link := `href="` + path + `"`
		rec := do(e, "GET", path, "", admin.session)
		if rec.Code != http.StatusOK {
			t.Errorf("admin GET %s = %d, want 200", path, rec.Code)
		}
		// Cada aba mostra as outras, então uma leva à outra.
		for _, tab := range orgPagePaths(admin.orgID) {
			if !strings.Contains(rec.Body.String(), `href="`+tab+`"`) {
				t.Errorf("admin GET %s does not link to the tab %s", path, tab)
			}
		}
		if rec := do(e, "GET", path, "", member.session); rec.Code != http.StatusNotFound {
			t.Errorf("member GET %s = %d, want 404", path, rec.Code)
		}
		if rec := do(e, "GET", "/orgs/"+admin.orgID, "", member.session); strings.Contains(rec.Body.String(), link) {
			t.Errorf("member sees a link to %s", path)
		}
		if rec := do(e, "GET", path, "", ""); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
			t.Errorf("GET %s without session = %d to %q, want 303 to /login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	// O item do menu leva à aba Sobre, que todo mundo lê. Só o admin vê, nela,
	// os links para as abas de gestão.
	about := "/orgs/" + admin.orgID + "/about"
	for _, who := range []account{admin, member} {
		if rec := do(e, "GET", "/orgs/"+admin.orgID, "", who.session); !strings.Contains(rec.Body.String(), `href="`+about+`"`) {
			t.Error("the menu does not link to the organization page")
		}
	}
	adminAbout := do(e, "GET", about, "", admin.session).Body.String()
	memberAbout := do(e, "GET", about, "", member.session).Body.String()
	for _, path := range orgPagePaths(admin.orgID) {
		link := `href="` + path + `"`
		if !strings.Contains(adminAbout, link) {
			t.Errorf("admin about page does not link to %s", path)
		}
		if strings.Contains(memberAbout, link) {
			t.Errorf("member about page links to %s", path)
		}
	}
}

// O resumo da organização aparece no cabeçalho das páginas dela e na página
// inicial, e a organização chega pronta ao JavaScript.
func TestPages_OrgSummaryInHeader(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	orgPath := "/orgs/" + admin.orgID

	if body := do(e, "GET", orgPath, "", member.session).Body.String(); !strings.Contains(body, "Cada projeto tem seus times") {
		t.Error("home page without a summary does not show the default lede")
	}

	rec := do(e, "PATCH", "/api/orgs/"+admin.orgID, `{"summary":"Entregas <rápidas> no mesmo dia","industry":"Logística"}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH org = %d: %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{orgPath, orgPath + "/about"} {
		body := do(e, "GET", path, "", member.session).Body.String()
		if !strings.Contains(body, "Entregas &lt;rápidas&gt; no mesmo dia") {
			t.Errorf("GET %s does not show the escaped summary", path)
		}
		if strings.Contains(body, "<rápidas>") {
			t.Errorf("GET %s shows the summary without escaping", path)
		}
		if !strings.Contains(body, `"industry":"Logística"`) {
			t.Errorf("GET %s does not pass the organization to the page script", path)
		}
	}
}

// A página da organização e o perfil são separados, e a barra superior leva
// ao perfil.
func TestPages_OrgSettingsAndProfileAreSeparate(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")

	settings := do(e, "GET", "/orgs/"+admin.orgID+"/settings", "", admin.session).Body.String()
	if !strings.Contains(settings, `id="org-name"`) || !strings.Contains(settings, "Excluir organização") {
		t.Error("org settings page does not show the organization name and deletion")
	}
	if strings.Contains(settings, `id="me-email"`) || strings.Contains(settings, `id="pw-current"`) {
		t.Error("org settings page still shows the profile or password form")
	}
	if !strings.Contains(settings, `href="/profile"`) {
		t.Error("top bar does not link to the profile page")
	}

	profile := do(e, "GET", "/profile", "", admin.session).Body.String()
	if !strings.Contains(profile, `id="me-email"`) || !strings.Contains(profile, `id="pw-current"`) {
		t.Error("profile page does not show the profile and password forms")
	}
	if strings.Contains(profile, `id="org-name"`) || strings.Contains(profile, "Excluir organização") {
		t.Error("profile page shows organization settings")
	}
}

func TestPages_RedirectWithoutSession(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createProject(t, e, admin, "Projeto")

	for _, path := range pagePaths(admin.orgID, projectID) {
		rec := do(e, "GET", path, "", "")
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
			t.Errorf("GET %s without session = %d to %q, want 303 to /login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := do(e, "GET", "/projects/"+projectID, "", admin.session); rec.Header().Get("Location") != "/projects/"+projectID+"/tasks" {
		t.Errorf("GET /projects/:id redirects to %q, want the tasks tab", rec.Header().Get("Location"))
	}
}

func TestPages_StaticAssets(t *testing.T) {
	e := newServer(t)
	for path, contentType := range map[string]string{
		"/static/app.css":          "text/css",
		"/static/app.js":           "javascript",
		"/static/alpine.min.js":    "javascript",
		"/static/pages/auth.js":    "javascript",
		"/static/pages/org.js":     "javascript",
		"/static/pages/project.js": "javascript",
	} {
		rec := do(e, "GET", path, "", "")
		body, _ := io.ReadAll(rec.Body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), contentType) || len(body) == 0 {
			t.Errorf("GET %s = %d %q (%d bytes)", path, rec.Code, rec.Header().Get("Content-Type"), len(body))
		}
	}
}

// Todos os arquivos em templates/pages viram uma página carregada na inicialização.
func TestPages_AllTemplatesLoad(t *testing.T) {
	r, err := template.New(web.FS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	got := r.Pages()
	sort.Strings(got)
	for _, name := range []string{
		"login", "signup", "invite", "notfound",
		"org_projects", "org_people", "org_settings", "org_about", "profile",
		"project_tasks", "project_time", "project_teams", "project_integrations", "project_settings", "task_detail",
	} {
		i := sort.SearchStrings(got, name)
		if i == len(got) || got[i] != name {
			t.Errorf("page template %q not loaded (loaded: %v)", name, got)
		}
	}
}

func TestPages_TaskDetail(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	other := signup(t, e, "Outra", "bia@outra.com")
	projectID := createProject(t, e, admin, "Projeto Alfa")
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)
	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tela de login","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)

	rec = do(e, "GET", "/tasks/"+taskID, "", admin.session)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Projeto Alfa") || !strings.Contains(rec.Body.String(), taskID) {
		t.Errorf("GET /tasks/:id = %d, want 200 with the project name and task id in the page", rec.Code)
	}
	if rec := do(e, "GET", "/tasks/"+taskID, "", other.session); rec.Code != http.StatusNotFound {
		t.Errorf("task page from another org = %d, want 404", rec.Code)
	}
}
