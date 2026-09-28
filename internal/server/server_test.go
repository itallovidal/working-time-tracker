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
	return server.New(testClient, "test-key")
}

func do(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// As rotas documentadas funcionam com e sem barra no final.
func TestRoutes_DocumentedPathsResolve(t *testing.T) {
	e := newServer(t)

	rec := do(e, "POST", "/api/orgs", `{"name":"Org"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/orgs = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var org map[string]any
	json.Unmarshal(rec.Body.Bytes(), &org)
	orgID, _ := org["id"].(string)

	cases := []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/orgs", http.StatusOK},
		{"GET", "/api/orgs/", http.StatusOK},
		{"GET", "/api/orgs/" + orgID, http.StatusOK},
		{"GET", "/api/orgs/" + orgID + "/", http.StatusOK},
		{"GET", "/api/orgs/" + orgID + "/persons", http.StatusOK},
		{"GET", "/api/orgs/" + orgID + "/projects", http.StatusOK},
		{"GET", "/healthcheck", http.StatusOK},
		{"GET", "/healthcheck/", http.StatusOK},
		{"GET", "/healthcheck/hello", http.StatusOK},
	}
	for _, tc := range cases {
		if rec := do(e, tc.method, tc.path, ""); rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d: %s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}

// A tabela de rotas é o contrato da API. Uma rota que some ou muda de caminho
// quebra este teste antes de chegar ao cliente.
func TestRoutes_Table(t *testing.T) {
	e := newServer(t)

	want := []string{
		"GET /healthcheck",
		"GET /healthcheck/hello",

		"POST /api/orgs",
		"GET /api/orgs",
		"GET /api/orgs/:orgId",
		"PATCH /api/orgs/:orgId",
		"DELETE /api/orgs/:orgId",
		"POST /api/orgs/:orgId/persons",
		"GET /api/orgs/:orgId/persons",
		"POST /api/orgs/:orgId/projects",
		"GET /api/orgs/:orgId/projects",

		"GET /api/persons/:personId",
		"PATCH /api/persons/:personId",

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

	var got []string
	for _, r := range e.Router().Routes() {
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
