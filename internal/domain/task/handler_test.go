package task_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
)

type testApp struct {
	e         *echo.Echo
	orgID     string
	personID  string
	projectID string
	teamID    string
	taskID    string
}

func setupTestApp(t *testing.T) *testApp {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	integSvc := integration.NewService(integration.NewStore(testClient), "key")
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), integSvc)

	e := echo.New()
	e.Use(middleware.Recover())
	registerRoutes(e,
		organization.NewHandler(orgSvc),
		person.NewHandler(personSvc),
		project.NewHandler(projSvc),
		team.NewHandler(teamSvc, memberSvc),
		task.NewHandler(taskSvc),
	)

	org, err := orgSvc.Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	orgID := org.ID.String()
	personObj, err := personSvc.Create(orgID, "John", "john@test.com")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	personID := personObj.ID.String()
	projObj := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project"}`)
	projectID := jsonPath(projObj, "id")
	teamObj := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Team A"}`)
	teamID := jsonPath(teamObj, "id")
	mustCreate(t, e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+personID+`"}`)
	taskObj := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Task A","assignee_id":"`+personID+`"}`)
	taskID := jsonPath(taskObj, "id")

	return &testApp{
		e:         e,
		orgID:     orgID,
		personID:  personID,
		projectID: projectID,
		teamID:    teamID,
		taskID:    taskID,
	}
}

func TestHandler_Create_MissingAssignee(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	integSvc := integration.NewService(integration.NewStore(testClient), "key")
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), integSvc)

	orgH := organization.NewHandler(orgSvc)
	personH := person.NewHandler(personSvc)
	projH := project.NewHandler(projSvc)
	teamH := team.NewHandler(teamSvc, memberSvc)
	taskH := task.NewHandler(taskSvc)
	registerRoutes(e, orgH, personH, projH, teamH, taskH)

	org, err := orgSvc.Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	orgID := org.ID.String()
	if _, err := personSvc.Create(orgID, "John", "john@test.com"); err != nil {
		t.Fatalf("create person: %v", err)
	}
	proj := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project"}`)
	projectID := jsonPath(proj, "id")

	req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/tasks", strings.NewReader(`{"name":"Task"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_UpdateAndDelete(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("PATCH", "/api/tasks/"+app.taskID,
		strings.NewReader(`{"name":"Updated Name","description":"New desc"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/tasks/"+app.taskID, nil)
	rec = httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_LinkUnlinkExternalItem(t *testing.T) {
	app := setupTestApp(t)

	integrationID := createIntegration(t, app.projectID)
	req := httptest.NewRequest("POST", "/api/tasks/"+app.taskID+"/link-external-item",
		strings.NewReader(`{"integration_id":"`+integrationID+`","external_item_id":"42","external_item_url":"https://example.com/42"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("link expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/tasks/"+app.taskID+"/link-external-item", nil)
	rec = httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlink expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_ListByProject(t *testing.T) {
	app := setupTestApp(t)
	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		app.e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/projects/"+app.projectID+"/tasks"+query, nil))
		return rec
	}
	type page struct {
		Items     []map[string]any `json:"items"`
		Total     int              `json:"total"`
		Page      int              `json:"page"`
		PerPage   int              `json:"per_page"`
		Assignees []map[string]any `json:"assignees"`
	}

	// Sem page a resposta é o array inteiro, que é o que a aba Ponto consome.
	rec := get("")
	var all []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &all); rec.Code != http.StatusOK || err != nil || len(all) != 1 {
		t.Fatalf("no params: status=%d err=%v body=%s, want an array with one task", rec.Code, err, rec.Body.String())
	}
	rec = get("?q=nothing-like-this")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("no match without page: status=%d body=%s, want []", rec.Code, rec.Body.String())
	}

	// Com page vem a página com o total e os responsáveis.
	rec = get("?page=1")
	var first page
	if err := json.Unmarshal(rec.Body.Bytes(), &first); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("page=1: status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
	}
	if first.Total != 1 || first.Page != 1 || first.PerPage != task.DefaultPerPage || len(first.Items) != 1 || len(first.Assignees) != 1 {
		t.Errorf("page=1 = %+v, want one task, one assignee, page 1 of %d per page", first, task.DefaultPerPage)
	}

	deadline := url.QueryEscape(time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339))
	rec = get("?page=1&per_page=5&q=task&assignee_id=" + app.personID + "&deadline_to=" + deadline)
	var filtered page
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("all filters: status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
	}
	if filtered.Total != 1 || filtered.PerPage != 5 {
		t.Errorf("all filters = %+v, want the task with per_page 5", filtered)
	}

	// Página vazia continua com as listas presentes, e não null.
	rec = get("?page=1&q=nothing-like-this")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("empty page: status=%d body=%s, want items as an empty array", rec.Code, rec.Body.String())
	}

	for _, query := range []string{
		"?assignee_id=not-a-uuid",
		"?deadline_to=2026-10-12",
		"?page=0",
		"?page=two",
		"?page=1&per_page=0",
		"?q=" + strings.Repeat("a", 101),
	} {
		if rec := get(query); rec.Code != http.StatusBadRequest {
			t.Errorf("GET tasks%s: status=%d, want 400: %s", query, rec.Code, rec.Body.String())
		}
	}
}

func registerRoutes(
	e *echo.Echo,
	orgH *organization.Handler,
	personH *person.Handler,
	projH *project.Handler,
	teamH *team.Handler,
	taskH *task.Handler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/:orgId/projects", projH.Create)

	projects := e.Group("/api/projects")
	projects.POST("/:projectId/teams", teamH.Create)
	projects.POST("/:projectId/tasks", taskH.Create)
	projects.GET("/:projectId/tasks", taskH.ListByProject)

	tasks := e.Group("/api/tasks")
	tasks.PATCH("/:taskId", taskH.Update)
	tasks.DELETE("/:taskId", taskH.Delete)
	tasks.POST("/:taskId/link-external-item", taskH.LinkExternalItem)
	tasks.DELETE("/:taskId/link-external-item", taskH.UnlinkExternalItem)

	teams := e.Group("/api/teams")
	teams.POST("/:teamId/members", teamH.AddMember)
}

func mustCreate(t *testing.T, e *echo.Echo, method, path, body string) string {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code >= 400 {
		t.Fatalf("setup %s %s returned %d: %s", method, path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func jsonPath(data, path string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return ""
	}
	val, ok := m[path]
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}
