package work_session_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
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
	integSvc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), integSvc)
	wsSvc := work_session.NewService(work_session.NewStore(testClient), task.NewStore(testClient), allocation.NewStore(testClient))

	e := echo.New()
	e.Use(middleware.Recover())
	registerRoutes(e,
		organization.NewHandler(orgSvc),
		person.NewHandler(personSvc),
		project.NewHandler(projSvc),
		team.NewHandler(teamSvc, memberSvc),
		task.NewHandler(taskSvc),
		work_session.NewHandler(wsSvc),
	)

	org, err := orgSvc.Create("Test Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	orgID := org.ID.String()

	personObj, err := personSvc.Create(orgID, "John", "john@test.com")
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	personID := personObj.ID.String()

	projObj := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project A"}`)
	projectID := jsonPath(projObj, "id")

	teamObj := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Team A"}`)
	teamID := jsonPath(teamObj, "id")

	mustCreate(t, e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+personID+`"}`)

	taskObj := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Task A","assignee_id":"`+personID+`"}`)
	taskID := jsonPath(taskObj, "id")

	// Sem valor por hora no projeto a pessoa não bate ponto.
	if _, err := allocation.NewService(allocation.NewStore(testClient)).Set(projectID, personID, 2000); err != nil {
		t.Fatalf("set rate: %v", err)
	}

	return &testApp{
		e:         e,
		orgID:     orgID,
		personID:  personID,
		projectID: projectID,
		teamID:    teamID,
		taskID:    taskID,
	}
}

func TestHandler_ClockInOut(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/work-sessions/clock-in",
		strings.NewReader(`{"task_id":"`+app.taskID+`","person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("clock-in expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/work-sessions/clock-out",
		strings.NewReader(`{"person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clock-out expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Este handler está montado sem login: sem saber quem pede, nenhum valor sai.
	if !strings.Contains(rec.Body.String(), `"pay_rate_cents":null`) || !strings.Contains(rec.Body.String(), `"pay_amount_cents":null`) {
		t.Errorf("response without a logged-in person shows rates: %s", rec.Body.String())
	}
}

func TestHandler_ClockIn_MissingTask(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/work-sessions/clock-in",
		strings.NewReader(`{"task_id":"00000000-0000-0000-0000-000000000000","person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func registerRoutes(
	e *echo.Echo,
	orgH *organization.Handler,
	personH *person.Handler,
	projH *project.Handler,
	teamH *team.Handler,
	taskH *task.Handler,
	wsH *work_session.Handler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/:orgId/projects", projH.Create)

	projects := e.Group("/api/projects")
	projects.POST("/:projectId/teams", teamH.Create)
	projects.POST("/:projectId/tasks", taskH.Create)
	projects.POST("/:projectId/work-sessions/clock-in", wsH.ClockIn)
	projects.POST("/:projectId/work-sessions/clock-out", wsH.ClockOut)
	projects.GET("/:projectId/work-sessions", wsH.List)
	projects.GET("/:projectId/work-sessions/total", wsH.Total)

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
