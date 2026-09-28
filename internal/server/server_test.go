package server_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
	if err := database.AutoMigrate(testClient); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func newServer(t *testing.T) *echo.Echo {
	t.Helper()
	testutil.Truncate(t, testDB)
	e, err := server.New(testClient, server.Options{EncryptKey: "test-key", AuthRateLimit: 1000})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return e
}

// do faz uma requisição com o cookie de sessão dado (ou nenhum, se vazio).
func do(e *echo.Echo, method, path, body, session string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func sessionFrom(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c.Value
		}
	}
	t.Fatalf("response has no session cookie (status %d): %s", rec.Code, rec.Body.String())
	return ""
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return m
}

type account struct {
	session string
	orgID   string
	id      string
}

func signup(t *testing.T, e *echo.Echo, org, email string) account {
	t.Helper()
	rec := do(e, "POST", "/api/auth/signup",
		`{"organization_name":"`+org+`","name":"Admin","email":"`+email+`","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	return account{session: sessionFrom(t, rec), orgID: body["organization_id"].(string), id: body["id"].(string)}
}

// invite cria um convite como admin e aceita com o email dado, devolvendo a conta nova.
func invite(t *testing.T, e *echo.Echo, admin account, email, role string) account {
	t.Helper()
	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/invites", `{"role":"`+role+`"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite = %d: %s", rec.Code, rec.Body.String())
	}
	token := decode(t, rec)["token"].(string)
	rec = do(e, "POST", "/api/auth/invites/"+token+"/accept",
		`{"name":"Convidado","email":"`+email+`","password":"senha-forte-2"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("accept invite = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	return account{session: sessionFrom(t, rec), orgID: body["organization_id"].(string), id: body["id"].(string)}
}

func createProject(t *testing.T, e *echo.Echo, admin account, name string) string {
	t.Helper()
	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/projects", `{"name":"`+name+`"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project = %d: %s", rec.Code, rec.Body.String())
	}
	return decode(t, rec)["id"].(string)
}

// As rotas funcionam com e sem barra no final.
func TestRoutes_TrailingSlash(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org", "ana@test.com")

	cases := []string{
		"/api/orgs/" + a.orgID,
		"/api/orgs/" + a.orgID + "/",
		"/api/orgs/" + a.orgID + "/persons",
		"/api/orgs/" + a.orgID + "/projects/",
		"/api/auth/me",
		"/healthcheck",
		"/healthcheck/",
		"/healthcheck/hello",
	}
	for _, path := range cases {
		if rec := do(e, "GET", path, "", a.session); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAuth_RequiresSession(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org", "ana@test.com")

	if rec := do(e, "GET", "/api/orgs/"+a.orgID, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("API without session = %d, want 401", rec.Code)
	}
	if rec := do(e, "GET", "/api/orgs/"+a.orgID, "", "token-que-nao-existe"); rec.Code != http.StatusUnauthorized {
		t.Errorf("API with unknown token = %d, want 401", rec.Code)
	}

	rec := do(e, "GET", "/orgs/"+a.orgID, "", "")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
		t.Errorf("page without session = %d to %q, want 303 to /login", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAuth_LoginAndLogout(t *testing.T) {
	e := newServer(t)
	signup(t, e, "Org", "ana@test.com")

	if rec := do(e, "POST", "/api/auth/login", `{"email":"ana@test.com","password":"errada-123"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", rec.Code)
	}
	rec := do(e, "POST", "/api/auth/login", `{"email":" ANA@test.com ","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", rec.Code, rec.Body.String())
	}
	session := sessionFrom(t, rec)

	if rec := do(e, "POST", "/api/auth/logout", "", session); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := do(e, "GET", "/api/auth/me", "", session); rec.Code != http.StatusUnauthorized {
		t.Errorf("session after logout = %d, want 401", rec.Code)
	}
}

// Recursos de outra organização respondem 404, como se não existissem.
func TestAccess_OtherOrganizationIsNotFound(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org A", "ana@a.com")
	b := signup(t, e, "Org B", "bia@b.com")
	projectB := createProject(t, e, b, "Projeto B")

	paths := []string{
		"/api/orgs/" + b.orgID,
		"/api/orgs/" + b.orgID + "/persons",
		"/api/projects/" + projectB,
		"/api/projects/" + projectB + "/tasks",
		"/api/persons/" + b.id,
		"/api/projects/not-a-uuid",
	}
	for _, path := range paths {
		if rec := do(e, "GET", path, "", a.session); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s from another org = %d, want 404", path, rec.Code)
		}
	}
	if rec := do(e, "GET", "/projects/"+projectB+"/settings", "", a.session); rec.Code != http.StatusNotFound {
		t.Errorf("page of another org's project = %d, want 404", rec.Code)
	}
}

func TestAccess_MemberCannotUseAdminRoutes(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")

	if member.orgID != admin.orgID {
		t.Fatalf("invited member joined org %s, want %s", member.orgID, admin.orgID)
	}

	forbidden := []struct{ method, path, body string }{
		{"POST", "/api/orgs/" + admin.orgID + "/projects", `{"name":"X"}`},
		{"PATCH", "/api/orgs/" + admin.orgID, `{"name":"X"}`},
		{"DELETE", "/api/projects/" + projectID, ""},
		{"POST", "/api/projects/" + projectID + "/teams", `{"name":"T"}`},
		{"POST", "/api/orgs/" + admin.orgID + "/invites", `{}`},
		{"PATCH", "/api/persons/" + member.id + "/role", `{"role":"admin"}`},
		{"PATCH", "/api/persons/" + admin.id, `{"name":"Hacker","email":"x@test.com"}`},
	}
	for _, tc := range forbidden {
		if rec := do(e, tc.method, tc.path, tc.body, member.session); rec.Code != http.StatusForbidden {
			t.Errorf("member %s %s = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}

	// O membro pode ler e pode editar o próprio perfil.
	if rec := do(e, "GET", "/api/projects/"+projectID, "", member.session); rec.Code != http.StatusOK {
		t.Errorf("member GET project = %d, want 200", rec.Code)
	}
	if rec := do(e, "PATCH", "/api/persons/"+member.id, `{"name":"Bia Souza","email":"bia@test.com"}`, member.session); rec.Code != http.StatusOK {
		t.Errorf("member PATCH own profile = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// Um membro bate o próprio ponto sem mandar person_id, e não pode bater o de outra pessoa.
func TestWorkSessions_UseLoggedInPerson(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")

	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+member.id+`"}`, admin.session)
	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tarefa","assignee_id":"`+member.id+`"}`, member.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task = %d: %s", rec.Code, rec.Body.String())
	}
	taskID := decode(t, rec)["id"].(string)

	rec = do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, member.session)
	if rec.Code != http.StatusCreated || decode(t, rec)["person_id"] != member.id {
		t.Fatalf("member clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-in",
		`{"task_id":"`+taskID+`","person_id":"`+admin.id+`"}`, member.session)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member clock-in for another person = %d, want 403", rec.Code)
	}
	if rec := do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-out", `{}`, member.session); rec.Code != http.StatusOK {
		t.Errorf("member clock-out = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPages_AuthPagesRender(t *testing.T) {
	e := newServer(t)
	for _, path := range []string{"/login", "/signup", "/invite/qualquer-token"} {
		rec := do(e, "GET", path, "", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s = %d %s, want 200 html", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	a := signup(t, e, "Org", "ana@test.com")
	if rec := do(e, "GET", "/login", "", a.session); rec.Code != http.StatusSeeOther {
		t.Errorf("GET /login logged in = %d, want 303 to /", rec.Code)
	}
	if rec := do(e, "GET", "/", "", a.session); rec.Header().Get("Location") != "/orgs/"+a.orgID {
		t.Errorf("GET / redirects to %q, want /orgs/%s", rec.Header().Get("Location"), a.orgID)
	}
	if rec := do(e, "GET", "/pagina-que-nao-existe", "", a.session); rec.Code != http.StatusNotFound ||
		!strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("unknown page = %d %s, want 404 html", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// Um formulário HTML de outro site não consegue chamar a API com o cookie.
func TestAPI_RejectsNonJSONBodies(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org", "ana@test.com")

	req := httptest.NewRequest("PATCH", "/api/orgs/"+a.orgID, strings.NewReader("name=Hackeada"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: a.session})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("form-encoded PATCH = %d, want 415", rec.Code)
	}
}

// A tabela de rotas é o contrato da API. Uma rota que some ou muda de caminho
// quebra este teste antes de chegar ao cliente.
func TestRoutes_Table(t *testing.T) {
	e := newServer(t)

	want := []string{
		"GET /healthcheck",
		"GET /healthcheck/hello",

		"POST /api/auth/signup",
		"POST /api/auth/login",
		"GET /api/auth/invites/:token",
		"POST /api/auth/invites/:token/accept",
		"POST /api/auth/logout",
		"GET /api/auth/me",
		"POST /api/auth/password",

		"GET /api/orgs/:orgId",
		"PATCH /api/orgs/:orgId",
		"DELETE /api/orgs/:orgId",
		"GET /api/orgs/:orgId/persons",
		"POST /api/orgs/:orgId/projects",
		"GET /api/orgs/:orgId/projects",
		"POST /api/orgs/:orgId/invites",
		"GET /api/orgs/:orgId/invites",
		"DELETE /api/invites/:inviteId",

		"GET /api/persons/:personId",
		"PATCH /api/persons/:personId",
		"PATCH /api/persons/:personId/role",

		"GET /api/projects/:projectId",
		"PATCH /api/projects/:projectId",
		"DELETE /api/projects/:projectId",
		"POST /api/projects/:projectId/teams",
		"GET /api/projects/:projectId/teams",
		"POST /api/projects/:projectId/tasks",
		"GET /api/projects/:projectId/tasks",
		"POST /api/projects/:projectId/work-sessions/clock-in",
		"POST /api/projects/:projectId/work-sessions/clock-out",
		"GET /api/projects/:projectId/work-sessions",
		"GET /api/projects/:projectId/work-sessions/total",
		"POST /api/projects/:projectId/integrations",
		"GET /api/projects/:projectId/integrations",

		"GET /api/teams/:teamId",
		"PATCH /api/teams/:teamId",
		"DELETE /api/teams/:teamId",
		"POST /api/teams/:teamId/members",
		"DELETE /api/teams/:teamId/members",
		"GET /api/teams/:teamId/members",

		"GET /api/tasks/:taskId",
		"PATCH /api/tasks/:taskId",
		"DELETE /api/tasks/:taskId",
		"POST /api/tasks/:taskId/link-external-item",
		"DELETE /api/tasks/:taskId/link-external-item",
		"GET /api/tasks/:taskId/external-details",

		"GET /api/integrations/:integrationId",
		"PATCH /api/integrations/:integrationId",
		"DELETE /api/integrations/:integrationId",
	}

	methods := map[string]bool{"GET": true, "POST": true, "PATCH": true, "PUT": true, "DELETE": true}
	var got []string
	for _, r := range e.Router().Routes() {
		if !methods[r.Method] {
			continue // rotas coringa de 404 que o Echo cria para grupos com middleware
		}
		if strings.HasPrefix(r.Path, "/api") || strings.HasPrefix(r.Path, "/healthcheck") {
			got = append(got, r.Method+" "+r.Path)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("route table mismatch\n got:\n  %s\nwant:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
