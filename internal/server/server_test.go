package server_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"working-time-tracker/ent"
	"working-time-tracker/internal/domain/auth"
	"working-time-tracker/internal/server"
	"working-time-tracker/testutil"
)

var testClient *ent.Client
var testDB *sql.DB

func TestMain(m *testing.M) {
	testClient, testDB = testutil.Setup()
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

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var l []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatalf("decode list %q: %v", rec.Body.String(), err)
	}
	return l
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

// createEmptyProject cria o projeto e tira o dono dele. Quem cria um projeto sendo o dono
// da organização já entra nele; os testes que contam quem está no projeto partem de um
// projeto sem ninguém.
func createEmptyProject(t *testing.T, e *echo.Echo, admin account, name string) string {
	t.Helper()
	id := createProject(t, e, admin, name)
	if rec := do(e, "DELETE", "/api/projects/"+id+"/collaborators/"+admin.id, "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("remove the owner from the new project = %d: %s", rec.Code, rec.Body.String())
	}
	return id
}

// allocate define, como admin, quanto a pessoa recebe por hora no projeto. Sem
// isso ela não bate ponto.
func allocate(t *testing.T, e *echo.Echo, admin account, projectID, personID string, cents int) {
	t.Helper()
	rec := do(e, "PUT", "/api/projects/"+projectID+"/allocations/"+personID, fmt.Sprintf(`{"pay_rate_cents":%d}`, cents), admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("allocate = %d: %s", rec.Code, rec.Body.String())
	}
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
		"/api/projects/" + projectB + "/labels",
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
		{"PATCH", "/api/persons/" + member.id + "/weekly-hours", `{"weekly_hours":10}`},
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

// O admin preenche o perfil da organização; qualquer membro lê, e a moeda
// acompanha a pessoa logada.
func TestOrganization_Profile(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	path := "/api/orgs/" + admin.orgID

	rec := do(e, "PATCH", path, `{"summary":"Entregas no mesmo dia","website":"acme.com.br","cnpj":"11.222.333/0001-81","currency":"USD","work_mode":"hybrid"}`, admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin PATCH = %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(e, "GET", path, "", member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("member GET = %d: %s", rec.Code, rec.Body.String())
	}
	org := decode(t, rec)
	for field, want := range map[string]any{
		"name": "Org", "summary": "Entregas no mesmo dia", "website": "https://acme.com.br",
		"cnpj": "11222333000181", "currency": "USD", "timezone": "America/Sao_Paulo", "work_mode": "hybrid",
	} {
		if org[field] != want {
			t.Errorf("%s = %v, want %v", field, org[field], want)
		}
	}

	if got := decode(t, do(e, "GET", "/api/auth/me", "", member.session))["organization_currency"]; got != "USD" {
		t.Errorf("organization_currency in /auth/me = %v, want USD", got)
	}

	// Jornada e sprint não são da organização: a sprint é de cada projeto, e a
	// jornada, de cada pessoa. O projeto ignora uma jornada que venha no corpo.
	for _, gone := range []string{"weekly_hours", "default_sprint_days"} {
		if _, ok := org[gone]; ok {
			t.Errorf("organization still has %s", gone)
		}
	}
	rec = do(e, "POST", path+"/projects", `{"name":"Projeto","weekly_hours":30}`, admin.session)
	created := decode(t, rec)
	if created["sprint_duration_days"] != float64(14) {
		t.Errorf("new project = sprint %v, want 14", created["sprint_duration_days"])
	}
	if _, ok := created["weekly_hours"]; ok {
		t.Errorf("project still has weekly_hours: %s", rec.Body.String())
	}

	if rec := do(e, "PATCH", path, `{"website":"javascript:alert(1)"}`, admin.session); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH with a javascript: link = %d, want 400", rec.Code)
	}
	if rec := do(e, "PATCH", path, `{"summary":"Invasão"}`, member.session); rec.Code != http.StatusForbidden {
		t.Errorf("member PATCH = %d, want 403", rec.Code)
	}
}

// A lista de projetos e o detalhe trazem quantos colaboradores o projeto tem
// (quem tem valor por hora nele, cada pessoa uma vez, esteja em quantos times
// estiver) e quantas tarefas.
func TestProjects_MemberAndTaskCounts(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	unteamed := invite(t, e, admin, "caio@test.com", "member")
	busy := createEmptyProject(t, e, admin, "Projeto Alfa")
	idle := createEmptyProject(t, e, admin, "Projeto Beta")

	post := func(path, body string) map[string]any {
		t.Helper()
		rec := do(e, "POST", path, body, admin.session)
		if rec.Code >= 300 {
			t.Fatalf("POST %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}
	// Os três entram no projeto com um valor por hora; o Caio não entra em time nenhum.
	allocate(t, e, admin, busy, admin.id, 0)
	allocate(t, e, admin, busy, member.id, 5000)
	allocate(t, e, admin, busy, unteamed.id, 4000)
	// Dois times no mesmo projeto; a Bia está nos dois.
	backend := post("/api/projects/"+busy+"/teams", `{"name":"Backend"}`)["id"].(string)
	frontend := post("/api/projects/"+busy+"/teams", `{"name":"Frontend"}`)["id"].(string)
	post("/api/teams/"+backend+"/members", `{"person_id":"`+admin.id+`"}`)
	post("/api/teams/"+backend+"/members", `{"person_id":"`+member.id+`"}`)
	post("/api/teams/"+frontend+"/members", `{"person_id":"`+member.id+`"}`)
	for _, name := range []string{"Login", "Cadastro", "Relatório"} {
		post("/api/projects/"+busy+"/tasks", `{"name":"`+name+`","assignee_id":"`+admin.id+`"}`)
	}

	type counts struct{ members, tasks any }
	got := map[string]counts{}
	for _, p := range decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/projects", "", member.session)) {
		got[p["id"].(string)] = counts{p["member_count"], p["task_count"]}
	}
	if got[busy] != (counts{float64(3), float64(3)}) {
		t.Errorf("project with two teams, a person with only a rate and three tasks: got %+v, want 3 members and 3 tasks", got[busy])
	}
	if got[idle] != (counts{float64(0), float64(0)}) {
		t.Errorf("project without teams or tasks: got %+v, want zeros", got[idle])
	}

	detail := decode(t, do(e, "GET", "/api/projects/"+busy, "", member.session))
	if detail["member_count"] != float64(3) || detail["task_count"] != float64(3) {
		t.Errorf("project detail: member_count=%v task_count=%v, want 3 and 3", detail["member_count"], detail["task_count"])
	}
}

// O admin vê e altera todos os valores. O membro vê só o que recebe: nunca o
// valor cobrado do cliente nem o valor de um colega, em nenhuma rota.
func TestRates_VisibilityByRole(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto X")
	prj := "/api/projects/" + projectID

	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/customers", `{"name":"Empresa A","document":"11.222.333/0001-81"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create customer = %d: %s", rec.Code, rec.Body.String())
	}
	customerID := decode(t, rec)["id"].(string)

	rec = do(e, "PUT", prj+"/billing", `{"customer_id":"`+customerID+`","bill_rate_cents":10000}`, admin.session)
	if rec.Code != http.StatusOK || decode(t, rec)["bill_rate_cents"] != float64(10000) {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	for person, cents := range map[string]string{bia.id: "2000", caio.id: "2599", admin.id: "0"} {
		if rec := do(e, "PUT", prj+"/allocations/"+person, `{"pay_rate_cents":`+cents+`}`, admin.session); rec.Code != http.StatusOK {
			t.Fatalf("set allocation = %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Rotas de admin.
	forbidden := []struct{ method, path, body string }{
		{"GET", prj + "/billing", ""},
		{"PUT", prj + "/billing", `{"bill_rate_cents":1}`},
		{"GET", "/api/orgs/" + admin.orgID + "/customers", ""},
		{"POST", "/api/orgs/" + admin.orgID + "/customers", `{"name":"X"}`},
		{"GET", "/api/customers/" + customerID, ""},
		{"PATCH", "/api/customers/" + customerID, `{"name":"X"}`},
		{"DELETE", "/api/customers/" + customerID, ""},
		{"PUT", prj + "/allocations/" + bia.id, `{"pay_rate_cents":999999}`},
		{"GET", "/api/persons/" + caio.id + "/allocations", ""},
		{"GET", "/api/persons/" + admin.id + "/allocations", ""},
	}
	for _, tc := range forbidden {
		if rec := do(e, tc.method, tc.path, tc.body, bia.session); rec.Code != http.StatusForbidden {
			t.Errorf("member %s %s = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}

	// O projeto mostra o nome do cliente, e só isso.
	for _, path := range []string{prj, "/api/orgs/" + admin.orgID + "/projects"} {
		for _, who := range []account{admin, bia} {
			body := do(e, "GET", path, "", who.session).Body.String()
			if !strings.Contains(body, `"name":"Empresa A"`) {
				t.Errorf("GET %s does not show the customer name: %s", path, body)
			}
			// Só as chaves e o CNPJ são conferidos: um número solto como 10000
			// pode aparecer por acaso num UUID ou num horário.
			if strings.Contains(body, "bill_rate") || strings.Contains(body, "pay_rate") || strings.Contains(body, "11222333000181") {
				t.Errorf("GET %s leaks billing data: %s", path, body)
			}
		}
	}

	// A lista de valores do projeto: o membro recebe só a própria linha.
	rec = do(e, "GET", prj+"/allocations", "", bia.session)
	mine := decodeList(t, rec)
	if len(mine) != 1 || mine[0]["person_id"] != bia.id || mine[0]["pay_rate_cents"] != float64(2000) {
		t.Errorf("member allocations = %s, want only her own", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), caio.id) {
		t.Errorf("member sees a colleague's allocation: %s", rec.Body.String())
	}
	if all := decodeList(t, do(e, "GET", prj+"/allocations", "", admin.session)); len(all) != 3 {
		t.Errorf("admin sees %d allocations, want 3", len(all))
	}

	// Os valores de uma pessoa em todos os projetos: ela mesma e o admin.
	for _, who := range []account{bia, admin} {
		rec := do(e, "GET", "/api/persons/"+bia.id+"/allocations", "", who.session)
		list := decodeList(t, rec)
		if rec.Code != http.StatusOK || len(list) != 1 || list[0]["pay_rate_cents"] != float64(2000) {
			t.Errorf("GET own allocations = %d %s", rec.Code, rec.Body.String())
		}
		if prjRef, _ := list[0]["project"].(map[string]any); prjRef["name"] != "Projeto X" {
			t.Errorf("allocation does not name the project: %s", rec.Body.String())
		}
	}

	// Quem ainda não tem valor recebe uma lista vazia, não um erro.
	dora := invite(t, e, admin, "dora@test.com", "member")
	if rec := do(e, "GET", prj+"/allocations", "", dora.session); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("member without allocation = %d %s, want 200 []", rec.Code, rec.Body.String())
	}

	if rec := do(e, "PUT", prj+"/allocations/"+bia.id, `{}`, admin.session); rec.Code != http.StatusBadRequest {
		t.Errorf("PUT allocation without a rate = %d, want 400", rec.Code)
	}
	if rec := do(e, "DELETE", "/api/customers/"+customerID, "", admin.session); rec.Code != http.StatusBadRequest {
		t.Errorf("DELETE customer with projects = %d, want 400", rec.Code)
	}
}

// Clientes e valores de uma organização não existem para a outra.
func TestRates_StayInOrganization(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org A", "ana@a.com")
	b := signup(t, e, "Org B", "bia@b.com")
	projectA := createProject(t, e, a, "Projeto A")
	projectB := createProject(t, e, b, "Projeto B")
	customerA := decode(t, do(e, "POST", "/api/orgs/"+a.orgID+"/customers", `{"name":"Empresa A"}`, a.session))["id"].(string)
	do(e, "PUT", "/api/projects/"+projectA+"/allocations/"+a.id, `{"pay_rate_cents":2000}`, a.session)

	notFound := []struct{ method, path, body string }{
		{"GET", "/api/orgs/" + a.orgID + "/customers", ""},
		{"POST", "/api/orgs/" + a.orgID + "/customers", `{"name":"X"}`},
		{"GET", "/api/customers/" + customerA, ""},
		{"PATCH", "/api/customers/" + customerA, `{"name":"X"}`},
		{"DELETE", "/api/customers/" + customerA, ""},
		{"GET", "/api/projects/" + projectA + "/billing", ""},
		{"PUT", "/api/projects/" + projectA + "/billing", `{"bill_rate_cents":1}`},
		{"GET", "/api/projects/" + projectA + "/allocations", ""},
		{"PUT", "/api/projects/" + projectA + "/allocations/" + a.id, `{"pay_rate_cents":1}`},
		{"GET", "/api/persons/" + a.id + "/allocations", ""},
		// Projeto de B com uma pessoa de A.
		{"PUT", "/api/projects/" + projectB + "/allocations/" + a.id, `{"pay_rate_cents":1}`},
	}
	for _, tc := range notFound {
		if rec := do(e, tc.method, tc.path, tc.body, b.session); rec.Code != http.StatusNotFound {
			t.Errorf("other org %s %s = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}

	// O ID do cliente vem no corpo, então quem confere a organização é o service.
	rec := do(e, "PUT", "/api/projects/"+projectB+"/billing", `{"customer_id":"`+customerA+`","bill_rate_cents":5000}`, b.session)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("billing with another organization's customer = %d, want 400", rec.Code)
	}
	if rec := do(e, "GET", "/api/projects/"+projectB, "", b.session); strings.Contains(rec.Body.String(), "Empresa A") {
		t.Errorf("project B shows organization A's customer: %s", rec.Body.String())
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
	allocate(t, e, admin, projectID, member.id, 2000)
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

// Sem valor por hora no projeto ninguém bate ponto, nem um admin por outra
// pessoa. É o caso de quem foi tirado do projeto e ficou com uma tarefa dele.
func TestWorkSessions_RequireRate(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")
	prj := "/api/projects/" + projectID

	teamID := decode(t, do(e, "POST", prj+"/teams", `{"name":"Time"}`, admin.session))["id"].(string)
	allocate(t, e, admin, projectID, member.id, 2000)
	do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+member.id+`"}`, admin.session)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Tarefa","assignee_id":"`+member.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "DELETE", prj+"/collaborators/"+member.id, "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("remove from the project = %d: %s", rec.Code, rec.Body.String())
	}

	for who, tc := range map[string]struct{ body, session string }{
		"member":             {`{"task_id":"` + taskID + `"}`, member.session},
		"admin for a member": {`{"task_id":"` + taskID + `","person_id":"` + member.id + `"}`, admin.session},
	} {
		rec := do(e, "POST", prj+"/work-sessions/clock-in", tc.body, tc.session)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"work_session.no_rate"`) {
			t.Errorf("%s clock-in without a rate = %d %s, want 400 explaining the missing rate", who, rec.Code, rec.Body.String())
		}
	}
	if rec := do(e, "GET", "/api/work-sessions/active", "", member.session); strings.TrimSpace(rec.Body.String()) != "null" {
		t.Errorf("a session was opened without a rate: %s", rec.Body.String())
	}

	allocate(t, e, admin, projectID, member.id, 2000)
	if rec := do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, member.session); rec.Code != http.StatusCreated {
		t.Errorf("clock-in after the rate was set = %d: %s", rec.Code, rec.Body.String())
	}
}

// Nas sessões, o membro vê o valor só das próprias; o admin vê o valor pago de
// todos e o valor cobrado. Mudar um valor não altera as sessões já feitas.
func TestWorkSessions_AmountsByRole(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")
	prj := "/api/projects/" + projectID

	teamID := decode(t, do(e, "POST", prj+"/teams", `{"name":"Time"}`, admin.session))["id"].(string)
	allocate(t, e, admin, projectID, bia.id, 2000)
	allocate(t, e, admin, projectID, caio.id, 2500)
	for _, p := range []account{bia, caio} {
		do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+p.id+`"}`, admin.session)
	}
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Tarefa","assignee_id":"`+bia.id+`"}`, admin.session))["id"].(string)
	if rec := do(e, "PUT", prj+"/billing", `{"bill_rate_cents":10000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}

	clockIn := `{"task_id":"` + taskID + `"}`
	rec := do(e, "POST", prj+"/work-sessions/clock-in", clockIn, bia.session)
	opened := decode(t, rec)
	if rec.Code != http.StatusCreated || opened["pay_rate_cents"] != float64(2000) || opened["bill_rate_cents"] != nil {
		t.Fatalf("member clock-in = %d %s, want her pay rate and no bill rate", rec.Code, rec.Body.String())
	}
	do(e, "POST", prj+"/work-sessions/clock-out", `{}`, bia.session)
	do(e, "POST", prj+"/work-sessions/clock-in", clockIn, caio.session)
	do(e, "POST", prj+"/work-sessions/clock-out", `{}`, caio.session)

	// O admin troca o valor da Bia depois que ela já trabalhou.
	allocate(t, e, admin, projectID, bia.id, 9000)

	byPerson := func(session string, want int) map[string]map[string]any {
		t.Helper()
		out := map[string]map[string]any{}
		for _, s := range decodeList(t, do(e, "GET", prj+"/work-sessions", "", session)) {
			out[s["person_id"].(string)] = s
		}
		if len(out) != want {
			t.Fatalf("expected sessions of %d people, got %d", want, len(out))
		}
		return out
	}

	// O membro só recebe as próprias sessões, com o que ele mesmo ganhou e sem o valor cobrado.
	asBia := byPerson(bia.session, 1)
	if asBia[bia.id]["pay_rate_cents"] != float64(2000) {
		t.Errorf("her old session shows rate %v, want the 2000 from when she clocked in", asBia[bia.id]["pay_rate_cents"])
	}
	if asBia[bia.id]["pay_amount_cents"] == nil {
		t.Error("member does not see the amount of her own session")
	}
	for _, key := range []string{"bill_rate_cents", "bill_amount_cents"} {
		if v := asBia[bia.id][key]; v != nil {
			t.Errorf("member sees %s = %v on her own session", key, v)
		}
	}
	if rec := do(e, "GET", prj+"/work-sessions?person_id="+bia.id, "", bia.session); len(decodeList(t, rec)) != 1 {
		t.Errorf("member asking for herself = %s, want her one session", rec.Body.String())
	}

	asAdmin := byPerson(admin.session, 2)
	if asAdmin[bia.id]["pay_rate_cents"] != float64(2000) || asAdmin[caio.id]["pay_rate_cents"] != float64(2500) {
		t.Errorf("admin sees pay rates %v and %v, want 2000 and 2500", asAdmin[bia.id]["pay_rate_cents"], asAdmin[caio.id]["pay_rate_cents"])
	}
	for _, p := range []account{bia, caio} {
		if asAdmin[p.id]["bill_rate_cents"] != float64(10000) || asAdmin[p.id]["bill_amount_cents"] == nil {
			t.Errorf("admin does not see the bill rate and amount: %v", asAdmin[p.id])
		}
	}

	// O total segue as mesmas regras.
	total := func(query, session string) map[string]any {
		return decode(t, do(e, "GET", prj+"/work-sessions/total?"+query, "", session))
	}
	if got := total("person_id="+bia.id, bia.session); got["pay_amount_cents"] == nil || got["bill_amount_cents"] != nil {
		t.Errorf("member total of herself = %v, want her earnings and no bill amount", got)
	}
	if got := total("task_id="+taskID, admin.session); got["pay_amount_cents"] == nil || got["bill_amount_cents"] == nil {
		t.Errorf("admin total = %v, want both amounts", got)
	}
}

// O membro só enxerga o próprio ponto, e o servidor garante: sem person_id vale o
// dele, o de outra pessoa é recusado, e um total só por tarefa não soma os outros.
func TestWorkSessions_MemberOnlySeesOwn(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	caio := invite(t, e, admin, "caio@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")
	prj := "/api/projects/" + projectID
	allocate(t, e, admin, projectID, bia.id, 2000)
	allocate(t, e, admin, projectID, caio.id, 2500)
	taskID := decode(t, do(e, "POST", prj+"/tasks", `{"name":"Livre"}`, admin.session))["id"].(string)

	for _, p := range []account{bia, caio} {
		do(e, "POST", prj+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, p.session)
		do(e, "POST", prj+"/work-sessions/clock-out", `{}`, p.session)
	}

	list := func(query, session string) *httptest.ResponseRecorder {
		return do(e, "GET", prj+"/work-sessions"+query, "", session)
	}
	if rec := list("", bia.session); rec.Code != http.StatusOK || len(decodeList(t, rec)) != 1 || decodeList(t, rec)[0]["person_id"] != bia.id {
		t.Errorf("member list without filters = %d %s, want only her session", rec.Code, rec.Body.String())
	}
	if rec := list("?task_id="+taskID, bia.session); len(decodeList(t, rec)) != 1 {
		t.Errorf("member list by task = %s, want only her session", rec.Body.String())
	}
	for _, path := range []string{"/work-sessions?person_id=" + caio.id, "/work-sessions/total?person_id=" + caio.id} {
		if rec := do(e, "GET", prj+path, "", bia.session); rec.Code != http.StatusForbidden {
			t.Errorf("member GET %s = %d, want 403", path, rec.Code)
		}
	}
	// Um total só por tarefa fica restrito ao que é dela.
	byTask := decode(t, do(e, "GET", prj+"/work-sessions/total?task_id="+taskID, "", bia.session))
	byBia := decode(t, do(e, "GET", prj+"/work-sessions/total?person_id="+bia.id+"&task_id="+taskID, "", bia.session))
	if byTask["total_seconds"] != byBia["total_seconds"] || byTask["pay_amount_cents"] != byBia["pay_amount_cents"] {
		t.Errorf("member total by task = %v, want the same as her own total %v", byTask, byBia)
	}

	// O admin continua vendo todos, com ou sem filtro.
	if rec := list("", admin.session); len(decodeList(t, rec)) != 2 {
		t.Errorf("admin list = %s, want both sessions", rec.Body.String())
	}
	if rec := list("?person_id="+caio.id, admin.session); len(decodeList(t, rec)) != 1 {
		t.Errorf("admin list of one person = %s, want one session", rec.Body.String())
	}
}

// IDs que chegam pelo corpo ou pela query não passam pelo RequireOrg, então o
// service precisa conferir a organização: um admin de A não fecha o ponto de
// alguém de B nem soma as horas de uma tarefa de B.
func TestAccess_IDsInBodyAndQueryStayInOrganization(t *testing.T) {
	e := newServer(t)
	a := signup(t, e, "Org A", "ana@a.com")
	b := signup(t, e, "Org B", "bia@b.com")
	projectA := createProject(t, e, a, "Projeto A")
	projectB := createProject(t, e, b, "Projeto B")

	rec := do(e, "POST", "/api/projects/"+projectB+"/teams", `{"name":"Time B"}`, b.session)
	teamB := decode(t, rec)["id"].(string)
	allocate(t, e, b, projectB, b.id, 2000)
	do(e, "POST", "/api/teams/"+teamB+"/members", `{"person_id":"`+b.id+`"}`, b.session)
	rec = do(e, "POST", "/api/projects/"+projectB+"/tasks", `{"name":"Tarefa B","assignee_id":"`+b.id+`"}`, b.session)
	taskB := decode(t, rec)["id"].(string)
	if rec := do(e, "POST", "/api/projects/"+projectB+"/work-sessions/clock-in", `{"task_id":"`+taskB+`"}`, b.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock-in in org B = %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(e, "POST", "/api/projects/"+projectA+"/work-sessions/clock-out", `{"person_id":"`+b.id+`"}`, a.session)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("admin of A clocking out someone from B = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/work-sessions/active", "", b.session); strings.TrimSpace(rec.Body.String()) == "null" {
		t.Error("the session in org B was closed by an admin of org A")
	}

	for _, query := range []string{"task_id=" + taskB, "task_id=" + taskB + "&person_id=" + b.id} {
		rec = do(e, "GET", "/api/projects/"+projectA+"/work-sessions/total?"+query, "", a.session)
		if total := decode(t, rec)["total_seconds"]; total != float64(0) {
			t.Errorf("total of org B's task through project A (%s) = %v, want 0", query, total)
		}
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
		"GET /api/orgs/:orgId/overview",
		"GET /api/orgs/:orgId/persons",
		"POST /api/orgs/:orgId/projects",
		"GET /api/orgs/:orgId/projects",
		"POST /api/orgs/:orgId/invites",
		"GET /api/orgs/:orgId/invites",
		"DELETE /api/invites/:inviteId",

		"POST /api/orgs/:orgId/customers",
		"GET /api/orgs/:orgId/customers",
		"GET /api/customers/:customerId",
		"PATCH /api/customers/:customerId",
		"DELETE /api/customers/:customerId",

		"GET /api/persons/:personId",
		"PATCH /api/persons/:personId",
		"PATCH /api/persons/:personId/role",
		"PATCH /api/persons/:personId/permissions",
		"PATCH /api/persons/:personId/weekly-hours",
		"GET /api/persons/:personId/allocations",

		"GET /api/projects/:projectId",
		"PATCH /api/projects/:projectId",
		"DELETE /api/projects/:projectId",
		"GET /api/projects/:projectId/overview",
		"POST /api/projects/:projectId/teams",
		"GET /api/projects/:projectId/teams",
		"GET /api/projects/:projectId/members",
		"GET /api/projects/:projectId/billing",
		"PUT /api/projects/:projectId/billing",
		"GET /api/projects/:projectId/allocations",
		"PUT /api/projects/:projectId/allocations/:personId",
		"GET /api/projects/:projectId/collaborators",
		"DELETE /api/projects/:projectId/collaborators/:personId",
		"POST /api/projects/:projectId/tasks",
		"GET /api/projects/:projectId/tasks",
		"GET /api/projects/:projectId/labels",
		"POST /api/projects/:projectId/labels",
		"PATCH /api/projects/:projectId/labels/:labelId",
		"DELETE /api/projects/:projectId/labels/:labelId",
		"POST /api/projects/:projectId/work-sessions/clock-in",
		"POST /api/projects/:projectId/work-sessions/clock-out",
		"GET /api/projects/:projectId/work-sessions",
		"GET /api/projects/:projectId/work-sessions/total",
		"POST /api/projects/:projectId/work-sessions/:sessionId/tasks",
		"PATCH /api/projects/:projectId/work-sessions/:sessionId/tasks/:linkId",
		"DELETE /api/projects/:projectId/work-sessions/:sessionId/tasks/:linkId",
		"GET /api/work-sessions/active",
		"POST /api/projects/:projectId/integrations",
		"GET /api/projects/:projectId/integrations",
		"POST /api/projects/:projectId/sync",

		"GET /api/teams/:teamId",
		"PATCH /api/teams/:teamId",
		"DELETE /api/teams/:teamId",
		"POST /api/teams/:teamId/members",
		"DELETE /api/teams/:teamId/members",
		"GET /api/teams/:teamId/members",

		"GET /api/tasks/:taskId",
		"PATCH /api/tasks/:taskId",
		"POST /api/tasks/:taskId/claim",
		"PATCH /api/tasks/:taskId/attributes",
		"DELETE /api/tasks/:taskId",
		"POST /api/tasks/:taskId/link-external-item",
		"DELETE /api/tasks/:taskId/link-external-item",
		"GET /api/tasks/:taskId/external-details",
		"POST /api/tasks/:taskId/sync",
		"POST /api/tasks/:taskId/publish",

		"GET /api/integrations/:integrationId",
		"GET /api/integrations/:integrationId/repositories",
		"POST /api/integrations/:integrationId/sync",
		"PATCH /api/integrations/:integrationId",
		"DELETE /api/integrations/:integrationId",

		"GET /api/permissions",
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

// A tela de ponto usa a sessão ativa da pessoa logada, e o seletor de
// responsável usa os membros dos times do projeto.
func TestAPI_ActiveSessionAndProjectMembers(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto")

	if rec := do(e, "GET", "/api/work-sessions/active", "", admin.session); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("active without session = %d %q, want 200 null", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/projects/"+projectID+"/members", "", admin.session); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("members of a project without teams = %q, want []", rec.Body.String())
	}

	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"A"}`, admin.session)
	teamA := decode(t, rec)["id"].(string)
	rec = do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"B"}`, admin.session)
	teamB := decode(t, rec)["id"].(string)
	allocate(t, e, admin, projectID, admin.id, 0)
	do(e, "POST", "/api/teams/"+teamA+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)
	do(e, "POST", "/api/teams/"+teamB+"/members", `{"person_id":"`+admin.id+`"}`, admin.session)

	rec = do(e, "GET", "/api/projects/"+projectID+"/members", "", admin.session)
	var members []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &members)
	if len(members) != 1 || members[0]["id"] != admin.id {
		t.Errorf("members = %s, want only the admin once (even though she is in two teams)", rec.Body.String())
	}

	rec = do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Tarefa","assignee_id":"`+admin.id+`"}`, admin.session)
	taskID := decode(t, rec)["id"].(string)
	do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, admin.session)

	active := decode(t, do(e, "GET", "/api/work-sessions/active", "", admin.session))
	tasks, _ := active["tasks"].([]any)
	var task map[string]any
	if len(tasks) == 1 {
		task, _ = tasks[0].(map[string]any)["task"].(map[string]any)
	}
	if task == nil || task["id"] != taskID || task["project_id"] != projectID || active["project_id"] != projectID {
		t.Errorf("active session = %v, want task %s of project %s", active, taskID, projectID)
	}
}

// Todos no projeto veem quem são os colaboradores e os times de cada um. O
// valor por hora dos colegas é só de admins. A pessoa entra no projeto com o
// valor por hora, e um time recusa quem ainda não entrou. Só um admin tira
// alguém do projeto: a pessoa perde o valor e sai de todos os times, e as
// tarefas ficam.
func TestCollaborators_VisibilityAndRemoval(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	colleague := invite(t, e, admin, "caio@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createEmptyProject(t, e, admin, "Projeto")

	post := func(path, body string) map[string]any {
		t.Helper()
		rec := do(e, "POST", path, body, admin.session)
		if rec.Code >= 300 {
			t.Fatalf("POST %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}
	backend := post("/api/projects/"+projectID+"/teams", `{"name":"Backend"}`)["id"].(string)
	mobile := post("/api/projects/"+projectID+"/teams", `{"name":"Mobile"}`)["id"].(string)

	// Sem valor por hora no projeto a pessoa não entra em time, e continua fora dele.
	rec := do(e, "POST", "/api/teams/"+backend+"/members", `{"person_id":"`+member.id+`"}`, admin.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "team.no_rate") {
		t.Errorf("adding to a team a person without a rate = %d %s, want 400 team.no_rate", rec.Code, rec.Body.String())
	}
	if rec := do(e, "GET", "/api/projects/"+projectID+"/collaborators", "", admin.session); strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("collaborators after the refused team = %s, want []", rec.Body.String())
	}
	// Apagar só o valor deixaria a pessoa no time sem ele: essa rota não existe mais.
	if rec := do(e, "DELETE", "/api/projects/"+projectID+"/allocations/"+member.id, "", admin.session); rec.Code < 400 {
		t.Errorf("DELETE of a rate alone = %d, want the route gone", rec.Code)
	}

	// A Bia entra com valor e vai para dois times; o Caio entra com valor e nenhum time.
	allocate(t, e, admin, projectID, member.id, 5000)
	allocate(t, e, admin, projectID, colleague.id, 7000)
	post("/api/teams/"+backend+"/members", `{"person_id":"`+member.id+`"}`)
	post("/api/teams/"+mobile+"/members", `{"person_id":"`+member.id+`"}`)
	taskID := post("/api/projects/"+projectID+"/tasks", `{"name":"Tarefa","assignee_id":"`+member.id+`"}`)["id"].(string)

	base := "/api/projects/" + projectID + "/collaborators"
	// collaborators devolve, por pessoa, o valor e quantos times ela tem.
	type row struct {
		rate  any
		teams int
	}
	collaborators := func(session string) map[string]row {
		t.Helper()
		rec := do(e, "GET", base, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET collaborators = %d: %s", rec.Code, rec.Body.String())
		}
		rows := map[string]row{}
		for _, c := range decodeList(t, rec) {
			teams, _ := c["teams"].([]any)
			rows[c["person"].(map[string]any)["id"].(string)] = row{c["pay_rate_cents"], len(teams)}
		}
		return rows
	}

	asAdmin := collaborators(admin.session)
	if len(asAdmin) != 2 || asAdmin[member.id] != (row{float64(5000), 2}) || asAdmin[colleague.id] != (row{float64(7000), 0}) {
		t.Errorf("admin sees %+v, want Bia with 5000 and two teams and Caio with 7000 and none", asAdmin)
	}
	asMember := collaborators(member.session)
	if len(asMember) != 2 || asMember[member.id] != (row{float64(5000), 2}) || asMember[colleague.id] != (row{nil, 0}) {
		t.Errorf("member sees %+v, want her own rate and no rate for the colleague", asMember)
	}

	if rec := do(e, "GET", base, "", outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("collaborators of another organization = %d, want 404", rec.Code)
	}
	if rec := do(e, "GET", base, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("collaborators without session = %d, want 401", rec.Code)
	}
	if rec := do(e, "DELETE", base+"/"+colleague.id, "", member.session); rec.Code != http.StatusForbidden {
		t.Errorf("member removing a colleague = %d, want 403", rec.Code)
	}
	if rec := do(e, "DELETE", base+"/"+member.id, "", outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization removing a collaborator = %d, want 404", rec.Code)
	}
	if rec := do(e, "DELETE", base+"/"+outsider.id, "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("removing a person of another organization = %d, want 404", rec.Code)
	}
	if rec := do(e, "DELETE", base+"/"+admin.id, "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("removing someone who is not in the project = %d, want 404", rec.Code)
	}

	if rec := do(e, "DELETE", base+"/"+member.id, "", admin.session); rec.Code != http.StatusNoContent {
		t.Fatalf("admin removing a collaborator = %d: %s", rec.Code, rec.Body.String())
	}
	if left := collaborators(admin.session); len(left) != 1 || left[colleague.id].rate != float64(7000) {
		t.Errorf("after the removal the project has %+v, want only Caio", left)
	}
	// /members lista quem pode ser responsável por tarefa: quem está no projeto, com time ou
	// não. Depois da remoção sobra o Caio, que tem valor e nenhum time.
	if members := decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/members", "", admin.session)); len(members) != 1 || members[0]["id"] != colleague.id {
		t.Errorf("project members after the removal = %v, want only Caio", members)
	}
	for _, teamID := range []string{backend, mobile} {
		if members := decodeList(t, do(e, "GET", "/api/teams/"+teamID+"/members", "", admin.session)); len(members) != 0 {
			t.Errorf("team %s still has members after the removal: %v", teamID, members)
		}
	}
	if rates := decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/allocations", "", admin.session)); len(rates) != 1 {
		t.Errorf("rates after the removal = %v, want only Caio's", rates)
	}
	// A tarefa continua com ela, mas sem valor ela não bate mais ponto aqui.
	if task := decode(t, do(e, "GET", "/api/tasks/"+taskID, "", admin.session)); task["assignee_id"] != member.id {
		t.Errorf("task assignee after the removal = %v, want it unchanged", task["assignee_id"])
	}
	if rec := do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, member.session); rec.Code != http.StatusBadRequest {
		t.Errorf("clock-in after the removal = %d, want 400", rec.Code)
	}
	if rec := do(e, "DELETE", base+"/"+member.id, "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("removing the same person twice = %d, want 404", rec.Code)
	}
}

// Qualquer pessoa do projeto filtra e pagina a lista de tarefas. Sem page a
// rota segue devolvendo o array inteiro, que é o que a aba Ponto consome.
func TestTasks_ListFiltersAndPages(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createProject(t, e, admin, "Projeto")

	teamID := decode(t, do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"A"}`, admin.session))["id"].(string)
	for _, id := range []string{admin.id, member.id} {
		allocate(t, e, admin, projectID, id, 1000)
		do(e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+id+`"}`, admin.session)
	}
	for i := range 12 {
		assignee := admin.id
		if i < 3 {
			assignee = member.id
		}
		body := fmt.Sprintf(`{"name":"Tarefa %02d","assignee_id":"%s"}`, i, assignee)
		if rec := do(e, "POST", "/api/projects/"+projectID+"/tasks", body, admin.session); rec.Code != http.StatusCreated {
			t.Fatalf("create task %d = %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	base := "/api/projects/" + projectID + "/tasks"
	page := func(query, session string) (total, number float64, items []any) {
		t.Helper()
		rec := do(e, "GET", base+query, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks%s = %d: %s", query, rec.Code, rec.Body.String())
		}
		body := decode(t, rec)
		items, _ = body["items"].([]any)
		return body["total"].(float64), body["page"].(float64), items
	}

	if all := decodeList(t, do(e, "GET", base, "", member.session)); len(all) != 12 {
		t.Errorf("list without page has %d tasks, want all 12 in an array", len(all))
	}
	if total, number, items := page("?page=2", member.session); total != 12 || number != 2 || len(items) != 2 {
		t.Errorf("page 2: total=%v page=%v items=%d, want 12, 2 and 2", total, number, len(items))
	}
	if total, _, items := page("?page=1&assignee_id="+member.id, member.session); total != 3 || len(items) != 3 {
		t.Errorf("member's tasks: total=%v items=%d, want 3 and 3", total, len(items))
	}
	if total, _, items := page("?page=1&q=tarefa+1", admin.session); total != 2 || len(items) != 2 {
		t.Errorf(`search "tarefa 1": total=%v items=%d, want 2 (Tarefa 10 and Tarefa 11)`, total, len(items))
	}
	// Um responsável de outra organização só deixa a lista vazia.
	if total, _, items := page("?page=1&assignee_id="+outsider.id, member.session); total != 0 || len(items) != 0 {
		t.Errorf("assignee from another organization: total=%v items=%d, want an empty page", total, len(items))
	}

	if rec := do(e, "GET", base+"?page=1", "", outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("tasks of another organization = %d, want 404", rec.Code)
	}
	if rec := do(e, "GET", base+"?page=0", "", member.session); rec.Code != http.StatusBadRequest {
		t.Errorf("page=0 = %d, want 400", rec.Code)
	}
}

// A jornada semanal é da pessoa, vale para a organização toda e só um admin
// define. Todos da organização leem.
// O dono da organização (quem a criou) entra nos projetos que cria, com valor pago zero, e
// as horas dele valem o valor cobrado. Outro admin não é dono: não entra sozinho no projeto,
// precisa de valor para bater ponto, e não rebaixa o dono.
func TestOwner_EntersItsProjectsAndWorksAtTheBilledRate(t *testing.T) {
	e := newServer(t)
	owner := signup(t, e, "Org", "ana@test.com")
	other := invite(t, e, owner, "bia@test.com", "admin")

	me := decode(t, do(e, "GET", "/api/auth/me", "", owner.session))
	if me["is_owner"] != true || me["role"] != "admin" {
		t.Errorf("the account that created the organization = role %v, is_owner %v; want admin and owner", me["role"], me["is_owner"])
	}
	if got := decode(t, do(e, "GET", "/api/auth/me", "", other.session)); got["is_owner"] != false {
		t.Errorf("an invited admin has is_owner = %v, want false", got["is_owner"])
	}

	collaborators := func(projectID, session string) []map[string]any {
		t.Helper()
		return decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/collaborators", "", session))
	}
	mine := createProject(t, e, owner, "Do dono")
	got := collaborators(mine, owner.session)
	if len(got) != 1 || got[0]["person"].(map[string]any)["id"] != owner.id ||
		got[0]["person"].(map[string]any)["is_owner"] != true || got[0]["pay_rate_cents"] != float64(0) {
		t.Errorf("collaborators of the owner's project = %v, want the owner with a zero rate", got)
	}
	// Outro admin que cria um projeto não entra nele, e quem o cria não o torna dono.
	theirs := createProject(t, e, other, "Do outro admin")
	if got := collaborators(theirs, owner.session); len(got) != 0 {
		t.Errorf("collaborators of another admin's project = %v, want none", got)
	}

	// Escolher um valor para o dono não muda nada: ele recebe o valor cobrado.
	allocate(t, e, other, mine, owner.id, 9000)
	if got := collaborators(mine, owner.session); got[0]["pay_rate_cents"] != float64(0) {
		t.Errorf("owner rate after asking for 9000 = %v, want 0", got[0]["pay_rate_cents"])
	}

	// O dono bate ponto sem que ninguém o tenha adicionado num projeto sem ele: a sessão
	// guarda valor pago zero e o valor cobrado, e o outro admin, sem valor, não bate.
	if rec := do(e, "PUT", "/api/projects/"+theirs+"/billing", `{"bill_rate_cents":12000}`, other.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	taskID := decode(t, do(e, "POST", "/api/projects/"+theirs+"/tasks", `{"name":"Tarefa"}`, owner.session))["id"].(string)
	clock := "/api/projects/" + theirs + "/work-sessions/clock-in"
	if rec := do(e, "POST", clock, `{"task_id":"`+taskID+`"}`, other.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "work_session.no_rate") {
		t.Errorf("another admin without a rate clocks in = %d %s, want 400 work_session.no_rate", rec.Code, rec.Body.String())
	}
	rec := do(e, "POST", clock, `{"task_id":"`+taskID+`"}`, owner.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("owner clock-in = %d: %s", rec.Code, rec.Body.String())
	}
	session := decode(t, rec)
	if session["owner_hours"] != true || session["pay_rate_cents"] != float64(0) || session["bill_rate_cents"] != float64(12000) {
		t.Errorf("owner session = owner %v, pay %v, bill %v; want owner, 0 and 12000", session["owner_hours"], session["pay_rate_cents"], session["bill_rate_cents"])
	}
	if got := collaborators(theirs, owner.session); len(got) != 1 || got[0]["person"].(map[string]any)["id"] != owner.id {
		t.Errorf("after working there the owner is a collaborator: %v", got)
	}

	// Só o dono muda papéis, e o dono continua admin: nem ele mesmo se rebaixa.
	rec = do(e, "PATCH", "/api/persons/"+owner.id+"/role", `{"role":"member"}`, other.session)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "auth.owner_only") {
		t.Errorf("another admin demoting the owner = %d %s, want 403 auth.owner_only", rec.Code, rec.Body.String())
	}
	rec = do(e, "PATCH", "/api/persons/"+owner.id+"/role", `{"role":"member"}`, owner.session)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "person.owner_is_admin") {
		t.Errorf("the owner demoting itself = %d %s, want 400 person.owner_is_admin", rec.Code, rec.Body.String())
	}
	people := decodeList(t, do(e, "GET", "/api/orgs/"+owner.orgID+"/persons", "", other.session))
	owners := 0
	for _, p := range people {
		if p["is_owner"] == true {
			owners++
		}
	}
	if owners != 1 {
		t.Errorf("the organization has %d owners in the people list, want 1", owners)
	}
}

// A reunião semanal com o cliente é um dia e um horário do projeto: o admin cria e
// altera, todos os membros leem, e tirar o cliente do projeto apaga a reunião.
func TestProjects_CustomerMeeting(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	cust := decode(t, do(e, "POST", "/api/orgs/"+admin.orgID+"/customers", `{"name":"Empresa A"}`, admin.session))["id"].(string)

	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/projects",
		`{"name":"Alfa","customer_meeting_day":"Wednesday","customer_meeting_time":"10:30"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	id := created["id"].(string)
	if created["customer_meeting_day"] != "wednesday" || created["customer_meeting_time"] != "10:30" {
		t.Errorf("created meeting = %v %v, want wednesday 10:30", created["customer_meeting_day"], created["customer_meeting_time"])
	}
	if rec := do(e, "PUT", "/api/projects/"+id+"/billing", `{"customer_id":"`+cust+`","bill_rate_cents":9000}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set customer = %d: %s", rec.Code, rec.Body.String())
	}

	// Quem não é admin lê a reunião, mas não altera o projeto.
	read := decode(t, do(e, "GET", "/api/projects/"+id, "", member.session))
	if read["customer_meeting_day"] != "wednesday" || read["customer_meeting_time"] != "10:30" {
		t.Errorf("member reads the meeting as %v %v", read["customer_meeting_day"], read["customer_meeting_time"])
	}
	if rec := do(e, "PATCH", "/api/projects/"+id, `{"name":"Alfa","customer_meeting_day":"friday"}`, member.session); rec.Code != http.StatusForbidden {
		t.Errorf("member PATCH = %d, want 403", rec.Code)
	}

	// Trocar só o horário mantém o dia; as respostas inválidas trazem o código do erro.
	rec = do(e, "PATCH", "/api/projects/"+id, `{"name":"Alfa","customer_meeting_time":"16:00"}`, admin.session)
	if got := decode(t, rec); rec.Code != http.StatusOK || got["customer_meeting_day"] != "wednesday" || got["customer_meeting_time"] != "16:00" {
		t.Fatalf("PATCH time only = %d: %s", rec.Code, rec.Body.String())
	}
	for body, code := range map[string]string{
		`{"name":"Alfa","customer_meeting_time":"25:00"}`:                           "project.invalid_customer_meeting_time",
		`{"name":"Alfa","customer_meeting_day":"someday"}`:                          "project.invalid_weekday",
		`{"name":"Alfa","customer_meeting_day":"","customer_meeting_time":"10:00"}`: "project.customer_meeting_time_without_day",
	} {
		rec := do(e, "PATCH", "/api/projects/"+id, body, admin.session)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), code) {
			t.Errorf("PATCH %s = %d %s, want 400 %s", body, rec.Code, rec.Body.String(), code)
		}
	}

	// Sem cliente não há reunião: tirar o cliente apaga o dia e o horário.
	if rec := do(e, "PUT", "/api/projects/"+id+"/billing", `{"customer_id":null,"bill_rate_cents":null}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("clear customer = %d: %s", rec.Code, rec.Body.String())
	}
	read = decode(t, do(e, "GET", "/api/projects/"+id, "", admin.session))
	if _, ok := read["customer_meeting_day"]; ok {
		t.Errorf("an internal project still has a meeting day: %v", read["customer_meeting_day"])
	}
	if _, ok := read["customer_meeting_time"]; ok {
		t.Errorf("an internal project still has a meeting time: %v", read["customer_meeting_time"])
	}
}

func TestPersons_WeeklyHours(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	path := "/api/persons/" + bia.id + "/weekly-hours"

	if got, ok := decode(t, do(e, "GET", "/api/persons/"+bia.id, "", bia.session))["weekly_hours"]; !ok || got != nil {
		t.Errorf("new person weekly_hours = %v (present: %v), want null", got, ok)
	}

	rec := do(e, "PATCH", path, `{"weekly_hours":30}`, admin.session)
	if got := decode(t, rec)["weekly_hours"]; rec.Code != http.StatusOK || got != float64(30) {
		t.Fatalf("admin PATCH = %d, weekly_hours %v; want 200 and 30: %s", rec.Code, got, rec.Body.String())
	}
	// A própria pessoa e os colegas veem a jornada na lista da organização.
	for _, p := range decodeList(t, do(e, "GET", "/api/orgs/"+admin.orgID+"/persons", "", bia.session)) {
		want := any(nil)
		if p["id"] == bia.id {
			want = float64(30)
		}
		if p["weekly_hours"] != want {
			t.Errorf("list: weekly_hours of %v = %v, want %v", p["email"], p["weekly_hours"], want)
		}
	}
	// Mudar nome e email não mexe na jornada.
	rec = do(e, "PATCH", "/api/persons/"+bia.id, `{"name":"Bia Souza","email":"bia@test.com"}`, bia.session)
	if got := decode(t, rec)["weekly_hours"]; rec.Code != http.StatusOK || got != float64(30) {
		t.Errorf("PATCH profile = %d, weekly_hours %v; want 200 and 30", rec.Code, got)
	}

	for _, bad := range []string{`{"weekly_hours":169}`, `{"weekly_hours":-1}`} {
		if rec := do(e, "PATCH", path, bad, admin.session); rec.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s = %d, want 400", bad, rec.Code)
		}
	}
	if rec := do(e, "PATCH", path, `{"weekly_hours":20}`, bia.session); rec.Code != http.StatusForbidden {
		t.Errorf("member setting own weekly hours = %d, want 403", rec.Code)
	}
	if rec := do(e, "PATCH", path, `{"weekly_hours":20}`, outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("admin of another organization = %d, want 404", rec.Code)
	}

	// Zero e null apagam.
	for _, none := range []string{`{"weekly_hours":0}`, `{"weekly_hours":null}`} {
		do(e, "PATCH", path, `{"weekly_hours":30}`, admin.session)
		rec := do(e, "PATCH", path, none, admin.session)
		if got := decode(t, rec)["weekly_hours"]; rec.Code != http.StatusOK || got != nil {
			t.Errorf("PATCH %s = %d, weekly_hours %v; want 200 and null", none, rec.Code, got)
		}
	}
}

// Uma tarefa pode nascer sem responsável. Ela aparece no filtro "none", e quem
// bate o ponto nela, mesmo sem estar num time, passa a ser o responsável.
func TestTasks_WithoutAssigneeIsClaimedByTheFirstClockIn(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Alpha")
	allocate(t, e, admin, projectID, member.id, 5000)

	rec := do(e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Livre"}`, member.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without assignee = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	taskID := created["id"].(string)
	if created["assignee_id"] != nil {
		t.Errorf("assignee_id = %v, want null", created["assignee_id"])
	}

	free := func() int {
		rec := do(e, "GET", "/api/projects/"+projectID+"/tasks?assignee_id=none&page=1", "", member.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("list unassigned = %d: %s", rec.Code, rec.Body.String())
		}
		return int(decode(t, rec)["total"].(float64))
	}
	if n := free(); n != 1 {
		t.Fatalf("unassigned before the clock in = %d, want 1", n)
	}

	if rec := do(e, "POST", "/api/projects/"+projectID+"/work-sessions/clock-in", `{"task_id":"`+taskID+`"}`, member.session); rec.Code != http.StatusCreated {
		t.Fatalf("clock in = %d: %s", rec.Code, rec.Body.String())
	}
	if n := free(); n != 0 {
		t.Errorf("unassigned after the clock in = %d, want 0", n)
	}
	rec = do(e, "GET", "/api/tasks/"+taskID, "", member.session)
	if got := decode(t, rec)["assignee_id"]; got != member.id {
		t.Errorf("assignee_id after the clock in = %v, want %s", got, member.id)
	}

	// Um PATCH com assignee_id vazio desvincula de novo.
	rec = do(e, "PATCH", "/api/tasks/"+taskID, `{"name":"Livre","assignee_id":""}`, admin.session)
	if rec.Code != http.StatusOK || decode(t, rec)["assignee_id"] != nil {
		t.Errorf("unassign = %d %s, want 200 with a null assignee", rec.Code, rec.Body.String())
	}
}

// A lista de tarefas se divide em duas pela API: assignee_id=none traz as sem responsável,
// assignee_id=any as que têm, e uma pessoa traz só as dela.
func TestTasks_AssignedAndUnassignedLists(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Alpha")
	allocate(t, e, admin, projectID, member.id, 5000)
	tasks := "/api/projects/" + projectID + "/tasks"

	do(e, "POST", tasks, `{"name":"Livre 1"}`, member.session)
	do(e, "POST", tasks, `{"name":"Livre 2"}`, member.session)
	do(e, "POST", tasks, `{"name":"Da Bia","assignee_id":"`+member.id+`"}`, member.session)

	total := func(query string) int {
		t.Helper()
		rec := do(e, "GET", tasks+query, "", member.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks%s = %d: %s", query, rec.Code, rec.Body.String())
		}
		return int(decode(t, rec)["total"].(float64))
	}
	for query, want := range map[string]int{
		"?page=1":                          3,
		"?page=1&assignee_id=none":         2,
		"?page=1&assignee_id=any":          1,
		"?page=1&assignee_id=" + member.id: 1,
		"?page=1&assignee_id=" + admin.id:  0,
		"?page=1&assignee_id=any&q=livre":  0,
	} {
		if got := total(query); got != want {
			t.Errorf("GET tasks%s: total %d, want %d", query, got, want)
		}
	}
	// Sem page, a lista inteira vem num array: é o que a aba Minhas tarefas pede.
	mine := decodeList(t, do(e, "GET", tasks+"?assignee_id="+member.id, "", member.session))
	if len(mine) != 1 || mine[0]["name"] != "Da Bia" {
		t.Errorf("my tasks = %v, want only Da Bia", mine)
	}
	if rec := do(e, "GET", tasks+"?assignee_id=qualquer", "", member.session); rec.Code != http.StatusBadRequest {
		t.Errorf("an invalid assignee_id = %d, want 400", rec.Code)
	}
}

// assignee_id=others traz as tarefas que têm responsável e não é quem pergunta: a lista Com responsável
// da Lista de tarefas, que deixa as da própria pessoa para a aba Minhas tarefas. O total e as páginas
// já vêm sem as dela, e o resultado muda com quem pergunta.
func TestTasks_OthersList(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Alpha")
	allocate(t, e, admin, projectID, member.id, 5000)
	tasks := "/api/projects/" + projectID + "/tasks"

	do(e, "POST", tasks, `{"name":"Livre"}`, member.session)
	do(e, "POST", tasks, `{"name":"Da Ana 1","assignee_id":"`+admin.id+`"}`, admin.session)
	do(e, "POST", tasks, `{"name":"Da Ana 2","assignee_id":"`+admin.id+`"}`, admin.session)
	do(e, "POST", tasks, `{"name":"Da Bia","assignee_id":"`+member.id+`"}`, member.session)

	names := func(session, query string) []string {
		t.Helper()
		rec := do(e, "GET", tasks+query, "", session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks%s = %d: %s", query, rec.Code, rec.Body.String())
		}
		var out []string
		for _, it := range decode(t, rec)["items"].([]any) {
			out = append(out, it.(map[string]any)["name"].(string))
		}
		return out
	}
	// A Bia vê as duas da Ana, e não a dela nem a livre; a Ana vê só a da Bia.
	if got := names(member.session, "?page=1&assignee_id=others&per_page=1"); len(got) != 1 {
		t.Errorf("page of one for Bia = %v, want one task", got)
	}
	rec := do(e, "GET", tasks+"?page=1&assignee_id=others&per_page=1", "", member.session)
	if total := int(decode(t, rec)["total"].(float64)); total != 2 {
		t.Errorf("total of the others for Bia = %d, want 2 (the pages count without her own)", total)
	}
	for who, c := range map[string]struct {
		session string
		want    []string
	}{
		"Bia": {member.session, []string{"Da Ana 1", "Da Ana 2"}},
		"Ana": {admin.session, []string{"Da Bia"}},
	} {
		got := names(c.session, "?page=1&assignee_id=others&per_page=10")
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("others for %s = %v, want %v", who, got, c.want)
		}
	}
	// Quem escolhe uma pessoa pelo id continua vendo as tarefas dela, inclusive as da própria pessoa.
	if got := names(member.session, "?page=1&assignee_id="+member.id); len(got) != 1 || got[0] != "Da Bia" {
		t.Errorf("Bia asking for her own tasks = %v, want only Da Bia", got)
	}
}

// Pegar a tarefa pela API: POST /api/tasks/:taskId/claim passa uma tarefa sem responsável para quem está
// logado, sem bater o ponto e sem mexer no status; quem a pega de novo não muda nada, e uma tarefa que já é
// de outra pessoa responde 409.
func TestTasks_Claim(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createProject(t, e, admin, "Alpha")
	allocate(t, e, admin, projectID, member.id, 5000)
	tasks := "/api/projects/" + projectID + "/tasks"

	id := decode(t, do(e, "POST", tasks, `{"name":"Livre"}`, member.session))["id"].(string)
	claim := "/api/tasks/" + id + "/claim"

	if rec := do(e, "POST", claim, "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("claiming without a session = %d, want 401", rec.Code)
	}
	if rec := do(e, "POST", claim, "", outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("claiming a task of another organization = %d, want 404", rec.Code)
	}
	if rec := do(e, "POST", "/api/tasks/00000000-0000-4000-8000-000000000000/claim", "", member.session); rec.Code != http.StatusNotFound {
		t.Errorf("claiming a task that does not exist = %d, want 404", rec.Code)
	}

	rec := do(e, "POST", claim, "", member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got["assignee_id"] != member.id || got["status"] != "backlog" {
		t.Errorf("after the claim: assignee %v and status %v, want the member and backlog (claiming is not starting)", got["assignee_id"], got["status"])
	}
	if rec := do(e, "POST", claim, "", member.session); rec.Code != http.StatusOK {
		t.Errorf("claiming a task that is already yours = %d, want 200", rec.Code)
	}
	if rec := do(e, "POST", claim, "", admin.session); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "task.already_assigned") {
		t.Errorf("claiming the task of another person = %d: %s, want 409 task.already_assigned", rec.Code, rec.Body.String())
	}
	if after := decode(t, do(e, "GET", "/api/tasks/"+id, "", admin.session)); after["assignee_id"] != member.id {
		t.Errorf("the refused claim moved the task to %v", after["assignee_id"])
	}

	// Pegar não bate o ponto: a pessoa não fica com sessão, e a tarefa sai das sem responsável e passa às dela.
	if sessions := decodeList(t, do(e, "GET", "/api/projects/"+projectID+"/work-sessions?person_id="+member.id, "", member.session)); len(sessions) != 0 {
		t.Errorf("claiming opened %d sessions, want none", len(sessions))
	}
	total := func(query string) int {
		t.Helper()
		return int(decode(t, do(e, "GET", tasks+query, "", member.session))["total"].(float64))
	}
	if free, mine := total("?page=1&assignee_id=none"), total("?page=1&assignee_id="+member.id); free != 0 || mine != 1 {
		t.Errorf("after the claim: %d unassigned and %d of the member, want 0 and 1", free, mine)
	}
}

// A atualização rápida pela API: PATCH /api/tasks/:taskId/attributes muda só a prioridade, o status e as
// etiquetas que vierem, e deixa o nome, a descrição, o responsável e o prazo como estão (o PATCH da tarefa
// exige o nome e troca a descrição).
func TestTasks_UpdateAttributes(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createProject(t, e, admin, "Alpha")
	otherProject := createProject(t, e, admin, "Outro")
	allocate(t, e, admin, projectID, member.id, 5000)
	labels := "/api/projects/" + projectID + "/labels"
	bug := decode(t, do(e, "POST", labels, `{"name":"bug"}`, admin.session))["id"].(string)
	foreign := decode(t, do(e, "POST", "/api/projects/"+otherProject+"/labels", `{"name":"bug"}`, admin.session))["id"].(string)

	body := fmt.Sprintf(`{"name":"Tarefa","description":"Detalhes importantes","assignee_id":"%s","priority":"low"}`, member.id)
	created := decode(t, do(e, "POST", "/api/projects/"+projectID+"/tasks", body, member.session))
	id := created["id"].(string)
	attrs := "/api/tasks/" + id + "/attributes"
	untouched := func(step string, task map[string]any) {
		t.Helper()
		if task["name"] != "Tarefa" || task["description"] != "Detalhes importantes" || task["assignee_id"] != member.id || task["deadline"] != created["deadline"] {
			t.Errorf("%s changed what the quick update does not own: %v %v %v %v", step, task["name"], task["description"], task["assignee_id"], task["deadline"])
		}
	}

	rec := do(e, "PATCH", attrs, `{"status":"awaiting_closure"}`, member.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("update the status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got["status"] != "awaiting_closure" || got["priority"] != "low" {
		t.Errorf("only the status should change: %v and %v", got["status"], got["priority"])
	}
	untouched("the status", got)

	rec = do(e, "PATCH", attrs, fmt.Sprintf(`{"priority":"urgent","label_ids":["%s"]}`, bug), admin.session)
	if rec.Code != http.StatusOK {
		t.Fatalf("update the priority and the labels = %d: %s", rec.Code, rec.Body.String())
	}
	got = decode(t, rec)
	if got["priority"] != "urgent" || got["status"] != "awaiting_closure" || len(got["labels"].([]any)) != 1 {
		t.Errorf("priority and labels should change and the status stay: %v %v %v", got["priority"], got["status"], got["labels"])
	}
	untouched("the priority and the labels", got)

	if got = decode(t, do(e, "PATCH", attrs, `{"label_ids":[]}`, member.session)); len(got["labels"].([]any)) != 0 {
		t.Errorf("an empty label_ids left %v, want the labels cleared", got["labels"])
	}

	for name, c := range map[string]struct{ body, code string }{
		"invalid priority":       {`{"priority":"critical"}`, "task.invalid_priority"},
		"invalid status":         {`{"status":"done"}`, "task.invalid_status"},
		"label of other project": {fmt.Sprintf(`{"label_ids":["%s"]}`, foreign), "task.label_other_project"},
	} {
		if rec := do(e, "PATCH", attrs, c.body, member.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.code) {
			t.Errorf("%s = %d: %s, want 400 %s", name, rec.Code, rec.Body.String(), c.code)
		}
	}
	if rec := do(e, "PATCH", attrs, `{"status":`, member.session); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed body = %d, want 400", rec.Code)
	}
	if rec := do(e, "PATCH", attrs, `{"status":"closed"}`, outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization updating the attributes = %d, want 404", rec.Code)
	}
	if rec := do(e, "PATCH", attrs, `{"status":"closed"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("updating without a session = %d, want 401", rec.Code)
	}
	after := decode(t, do(e, "GET", "/api/tasks/"+id, "", member.session))
	if after["status"] != "awaiting_closure" || after["priority"] != "urgent" {
		t.Errorf("a refused update changed the task: %v and %v", after["status"], after["priority"])
	}
	untouched("the refused updates", after)
}

// O status pela API: a tarefa nasce em backlog mesmo que o corpo peça outro, só o PATCH
// muda, um valor fora dos quatro é recusado, e a lista filtra por um ou mais status.
func TestTasks_Status(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "Projeto")
	tasks := "/api/projects/" + projectID + "/tasks"

	rec := do(e, "POST", tasks, `{"name":"Nova","status":"closed"}`, member.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	if created["status"] != "backlog" {
		t.Errorf("a new task came back with status %v, want backlog", created["status"])
	}
	one := "/api/tasks/" + created["id"].(string)
	do(e, "POST", tasks, `{"name":"Outra"}`, member.session)

	if rec := do(e, "PATCH", one, `{"name":"Nova","status":"in_progress"}`, member.session); rec.Code != http.StatusOK || decode(t, rec)["status"] != "in_progress" {
		t.Fatalf("patch to in_progress = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "PATCH", one, `{"name":"Nova 2"}`, member.session); rec.Code != http.StatusOK || decode(t, rec)["status"] != "in_progress" {
		t.Errorf("a patch without status changed it: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "PATCH", one, `{"name":"Nova","status":"done"}`, member.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "task.invalid_status") {
		t.Errorf("an invalid status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, do(e, "GET", one, "", member.session)); got["status"] != "in_progress" {
		t.Errorf("a refused patch left status %v, want in_progress", got["status"])
	}

	count := func(query string) int {
		t.Helper()
		rec := do(e, "GET", tasks+query, "", member.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks%s = %d: %s", query, rec.Code, rec.Body.String())
		}
		return int(decode(t, rec)["total"].(float64))
	}
	for query, want := range map[string]int{
		"?page=1":                            2,
		"?page=1&status=backlog":             1,
		"?page=1&status=in_progress":         1,
		"?page=1&status=backlog,in_progress": 2,
		"?page=1&status=closed":              0,
		"?page=1&status=in_progress&q=outra": 0,
	} {
		if got := count(query); got != want {
			t.Errorf("GET tasks%s: total %d, want %d", query, got, want)
		}
	}
	if rec := do(e, "GET", tasks+"?status=done", "", member.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "task.invalid_status_filter") {
		t.Errorf("GET tasks?status=done = %d: %s", rec.Code, rec.Body.String())
	}
}

// Prioridade e etiquetas pela API: só admin cria etiqueta, a tarefa guarda e devolve as
// duas, os filtros combinam, e uma etiqueta de outro projeto ou outra organização é recusada.
func TestTasks_PriorityAndLabels(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	outsider := signup(t, e, "Outra", "zeca@test.com")
	projectID := createProject(t, e, admin, "Projeto")
	otherProject := createProject(t, e, admin, "Outro")
	labels := "/api/projects/" + projectID + "/labels"

	if rec := do(e, "POST", labels, `{"name":"bug"}`, member.session); rec.Code != http.StatusForbidden {
		t.Errorf("a member creating a label = %d, want 403", rec.Code)
	}
	if rec := do(e, "POST", labels, `{"name":"bug"}`, outsider.session); rec.Code != http.StatusNotFound {
		t.Errorf("another organization creating a label = %d, want 404", rec.Code)
	}
	rec := do(e, "POST", labels, `{"name":"bug"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin creating a label = %d: %s", rec.Code, rec.Body.String())
	}
	bug := decode(t, rec)["id"].(string)
	design := decode(t, do(e, "POST", labels, `{"name":"design"}`, admin.session))["id"].(string)
	foreign := decode(t, do(e, "POST", "/api/projects/"+otherProject+"/labels", `{"name":"bug"}`, admin.session))["id"].(string)
	if rec := do(e, "POST", labels, `{"name":"BUG"}`, admin.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "label.name_taken") {
		t.Errorf("a repeated name = %d: %s", rec.Code, rec.Body.String())
	}
	if list := decodeList(t, do(e, "GET", labels, "", member.session)); len(list) != 2 {
		t.Errorf("a member reads %d labels, want 2", len(list))
	}

	tasks := "/api/projects/" + projectID + "/tasks"
	body := fmt.Sprintf(`{"name":"Com tudo","priority":"urgent","label_ids":["%s","%s"]}`, bug, design)
	rec = do(e, "POST", tasks, body, member.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create a task with priority and labels = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)
	if created["priority"] != "urgent" || len(created["labels"].([]any)) != 2 {
		t.Errorf("the task came back with %v and %v", created["priority"], created["labels"])
	}
	taskID := created["id"].(string)
	do(e, "POST", tasks, `{"name":"Simples"}`, member.session)
	do(e, "POST", tasks, fmt.Sprintf(`{"name":"Só bug","priority":"low","label_ids":["%s"]}`, bug), member.session)

	if rec := do(e, "POST", tasks, `{"name":"X","priority":"critical"}`, member.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "task.invalid_priority") {
		t.Errorf("an invalid priority = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "POST", tasks, fmt.Sprintf(`{"name":"X","label_ids":["%s"]}`, foreign), member.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "task.label_other_project") {
		t.Errorf("a label of another project = %d: %s", rec.Code, rec.Body.String())
	}

	count := func(query string) int {
		t.Helper()
		rec := do(e, "GET", tasks+query, "", member.session)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET tasks%s = %d: %s", query, rec.Code, rec.Body.String())
		}
		return int(decode(t, rec)["total"].(float64))
	}
	for query, want := range map[string]int{
		"?page=1":                                 3,
		"?page=1&priority=urgent":                 1,
		"?page=1&priority=urgent,low":             2,
		"?page=1&priority=none":                   1,
		"?page=1&label_id=" + bug:                 2,
		"?page=1&label_id=" + design:              1,
		"?page=1&label_id=" + bug + "," + design:  2,
		"?page=1&priority=low&label_id=" + design: 0,
		"?page=1&priority=low&label_id=" + bug:    1,
	} {
		if got := count(query); got != want {
			t.Errorf("GET tasks%s: total %d, want %d", query, got, want)
		}
	}
	for _, bad := range []string{"?priority=critical", "?label_id=nao-e-uuid"} {
		if rec := do(e, "GET", tasks+bad, "", member.session); rec.Code != http.StatusBadRequest {
			t.Errorf("GET tasks%s = %d, want 400", bad, rec.Code)
		}
	}

	// Editar: omitir mantém, e uma lista vazia tira as etiquetas.
	one := "/api/tasks/" + taskID
	if rec := do(e, "PATCH", one, `{"name":"Com tudo 2"}`, member.session); rec.Code != http.StatusOK {
		t.Fatalf("patch without priority and labels = %d: %s", rec.Code, rec.Body.String())
	} else if got := decode(t, rec); got["priority"] != "urgent" || len(got["labels"].([]any)) != 2 {
		t.Errorf("an update without them changed them: %v %v", got["priority"], got["labels"])
	}
	rec = do(e, "PATCH", one, `{"name":"Com tudo 2","priority":"none","label_ids":[]}`, member.session)
	if got := decode(t, rec); got["priority"] != "none" || len(got["labels"].([]any)) != 0 {
		t.Errorf("clearing came back with %v %v", got["priority"], got["labels"])
	}

	// Renomear e excluir são do admin; excluir deixa as tarefas.
	if rec := do(e, "PATCH", labels+"/"+bug, `{"name":"defeito"}`, member.session); rec.Code != http.StatusForbidden {
		t.Errorf("a member renaming a label = %d, want 403", rec.Code)
	}
	if rec := do(e, "PATCH", labels+"/"+bug, `{"name":"defeito"}`, admin.session); rec.Code != http.StatusOK {
		t.Errorf("admin renaming a label = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(e, "DELETE", labels+"/"+foreign, "", admin.session); rec.Code != http.StatusNotFound {
		t.Errorf("deleting a label through another project = %d, want 404", rec.Code)
	}
	if rec := do(e, "DELETE", labels+"/"+bug, "", admin.session); rec.Code != http.StatusNoContent {
		t.Errorf("admin deleting a label = %d: %s", rec.Code, rec.Body.String())
	}
	if got := count("?page=1"); got != 3 {
		t.Errorf("after deleting a label the project has %d tasks, want 3", got)
	}
}
