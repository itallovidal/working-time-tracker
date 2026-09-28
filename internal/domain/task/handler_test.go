package task_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
