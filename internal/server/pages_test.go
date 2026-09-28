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

// pagePaths são as páginas logadas de uma organização com um projeto.
func pagePaths(orgID, projectID string) []string {
	return []string{
		"/orgs/" + orgID,
		"/orgs/" + orgID + "/people",
		"/orgs/" + orgID + "/settings",
		"/projects/" + projectID + "/tasks",
		"/projects/" + projectID + "/time-tracking",
		"/projects/" + projectID + "/teams",
		"/projects/" + projectID + "/settings",
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
		"org_projects", "org_people", "org_settings",
		"project_tasks", "project_time", "project_teams", "project_settings", "task_detail",
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
