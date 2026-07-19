package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tmpl "working-time-tracker/internal/template"
	"working-time-tracker/internal/service"
	"working-time-tracker/internal/store"
	"working-time-tracker/web"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/working_time_tracker_test?sslmode=disable"
	}
	var err error
	testDB, err = store.Open(dsn)
	if err != nil {
		log.Fatalf("failed to connect to test database: %v", err)
	}
	if err := store.AutoMigrate(testDB); err != nil {
		log.Fatalf("failed to migrate test database: %v", err)
	}

	code := m.Run()
	os.Exit(code)
}

func cleanupDB(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE work_sessions CASCADE")
	testDB.Exec("TRUNCATE TABLE tasks CASCADE")
	testDB.Exec("TRUNCATE TABLE team_memberships CASCADE")
	testDB.Exec("TRUNCATE TABLE integrations CASCADE")
	testDB.Exec("TRUNCATE TABLE teams CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE persons CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

type testApp struct {
	e          *echo.Echo
	orgID      string
	personID   string
	projectID  string
	teamID     string
	taskID     string
}

func registerAllRoutes(
	e *echo.Echo,
	orgH *OrganizationHandler,
	personH *PersonHandler,
	projH *ProjectHandler,
	teamH *TeamHandler,
	taskH *TaskHandler,
	timeH *TimeEntryHandler,
	integH *IntegrationHandler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/", orgH.Create)
	orgs.GET("/", orgH.List)
	orgs.GET("/:orgId", orgH.Get)
	orgs.PATCH("/:orgId", orgH.Update)
	orgs.DELETE("/:orgId", orgH.Delete)

	if personH != nil {
		orgs.POST("/:orgId/persons", personH.Create)
		orgs.GET("/:orgId/persons", personH.ListByOrg)
	}
	if projH != nil {
		orgs.POST("/:orgId/projects", projH.Create)
		orgs.GET("/:orgId/projects", projH.ListByOrg)
	}

	if projH != nil {
		projects := e.Group("/api/projects")
		projects.GET("/:projectId", projH.Get)
		projects.PATCH("/:projectId", projH.Update)
		projects.DELETE("/:projectId", projH.Delete)

		if teamH != nil {
			projects.POST("/:projectId/teams", teamH.Create)
			projects.GET("/:projectId/teams", teamH.ListByProject)
		}
		if taskH != nil {
			projects.POST("/:projectId/tasks", taskH.Create)
			projects.GET("/:projectId/tasks", taskH.ListByProject)
		}
		if timeH != nil {
			projects.POST("/:projectId/time-entries/clock-in", timeH.ClockIn)
			projects.POST("/:projectId/time-entries/clock-out", timeH.ClockOut)
			projects.GET("/:projectId/time-entries", timeH.List)
			projects.GET("/:projectId/time-entries/total", timeH.Total)
		}
		if integH != nil {
			projects.POST("/:projectId/integrations", integH.Create)
			projects.GET("/:projectId/integrations", integH.ListByProject)
		}
	}

	if teamH != nil {
		teams := e.Group("/api/teams")
		teams.GET("/:teamId", teamH.Get)
		teams.PATCH("/:teamId", teamH.Update)
		teams.DELETE("/:teamId", teamH.Delete)
		teams.POST("/:teamId/members", teamH.AddMember)
		teams.DELETE("/:teamId/members", teamH.RemoveMember)
		teams.GET("/:teamId/members", teamH.ListMembers)
	}

	if taskH != nil {
		tasks := e.Group("/api/tasks")
		tasks.GET("/:taskId", taskH.Get)
		tasks.PATCH("/:taskId", taskH.Update)
		tasks.DELETE("/:taskId", taskH.Delete)
		tasks.POST("/:taskId/link-external-item", taskH.LinkExternalItem)
		tasks.DELETE("/:taskId/link-external-item", taskH.UnlinkExternalItem)
		tasks.GET("/:taskId/external-details", taskH.GetExternalDetails)
	}

	if personH != nil {
		persons := e.Group("/api/persons")
		persons.GET("/:personId", personH.Get)
		persons.PATCH("/:personId", personH.Update)
	}

	if integH != nil {
		integrations := e.Group("/api/integrations")
		integrations.GET("/:integrationId", integH.Get)
		integrations.PATCH("/:integrationId", integH.Update)
		integrations.DELETE("/:integrationId", integH.Delete)
	}
}

func setupTestApp(t *testing.T) *testApp {
	cleanupDB(t)

	orgStore := store.NewOrganizationStore(testDB)
	personStore := store.NewPersonStore(testDB)
	projStore := store.NewProjectStore(testDB)
	teamStore := store.NewTeamStore(testDB)
	memberStore := store.NewTeamMembershipStore(testDB)
	taskStore := store.NewTaskStore(testDB)
	sessionStore := store.NewWorkSessionStore(testDB)
	integrationStore := store.NewIntegrationStore(testDB)

	orgSvc := service.NewOrganizationService(orgStore)
	personSvc := service.NewPersonService(personStore)
	projSvc := service.NewProjectService(projStore)
	teamSvc := service.NewTeamService(teamStore)
	memberSvc := service.NewTeamMembershipService(memberStore)
	integrationSvc := service.NewIntegrationService(integrationStore, "test-32-byte-encryption-key!!!!")
	taskSvc := service.NewTaskService(taskStore, memberStore, integrationSvc)
	timeSvc := service.NewTimeEntryService(sessionStore, taskStore)

	orgH := NewOrganizationHandler(orgSvc)
	personH := NewPersonHandler(personSvc)
	projH := NewProjectHandler(projSvc)
	teamH := NewTeamHandler(teamSvc, memberSvc)
	taskH := NewTaskHandler(taskSvc)
	timeH := NewTimeEntryHandler(timeSvc)
	integH := NewIntegrationHandler(integrationSvc)

	e := echo.New()
	e.Renderer = tmpl.NewRendererFromFS(web.FS, "templates/*.gohtml")
	e.Use(middleware.Recover())
	registerAllRoutes(e, orgH, personH, projH, teamH, taskH, timeH, integH)

	org := mustCreate(t, e, "POST", "/api/orgs/", `{"name":"Test Org}`)
	orgID := gjsonPath(org, "id")

	person := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/persons", `{"name":"John","email":"john@test.com"}`)
	personID := gjsonPath(person, "id")

	project := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project A"}`)
	projectID := gjsonPath(project, "id")

	team := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Team A"}`)
	teamID := gjsonPath(team, "id")

	mustCreate(t, e, "POST", "/api/teams/"+teamID+"/members", `{"person_id":"`+personID+`"}`)

	task := mustCreate(t, e, "POST", "/api/projects/"+projectID+"/tasks", `{"name":"Task A","assignee_id":"`+personID+`"}`)
	taskID := gjsonPath(task, "id")

	return &testApp{
		e:         e,
		orgID:     orgID,
		personID:  personID,
		projectID: projectID,
		teamID:    teamID,
		taskID:    taskID,
	}
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

func gjsonPath(data, path string) string {
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

func TestOrgs_CreateAndList(t *testing.T) {
	cleanupDB(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgStore := store.NewOrganizationStore(testDB)
	orgSvc := service.NewOrganizationService(orgStore)
	orgH := NewOrganizationHandler(orgSvc)
	registerAllRoutes(e, orgH, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/orgs/", strings.NewReader(`{"name":"My Org"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/orgs/", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOrgs_Create_EmptyName(t *testing.T) {
	cleanupDB(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgStore := store.NewOrganizationStore(testDB)
	orgSvc := service.NewOrganizationService(orgStore)
	orgH := NewOrganizationHandler(orgSvc)
	registerAllRoutes(e, orgH, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest("POST", "/api/orgs/", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPerson_Create_EmptyEmail(t *testing.T) {
	cleanupDB(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgStore := store.NewOrganizationStore(testDB)
	personStore := store.NewPersonStore(testDB)
	orgSvc := service.NewOrganizationService(orgStore)
	personSvc := service.NewPersonService(personStore)
	orgH := NewOrganizationHandler(orgSvc)
	personH := NewPersonHandler(personSvc)
	registerAllRoutes(e, orgH, personH, nil, nil, nil, nil, nil)

	org := mustCreate(t, e, "POST", "/api/orgs/", `{"name":"Org"}`)
	orgID := gjsonPath(org, "id")

	req := httptest.NewRequest("POST", "/api/orgs/"+orgID+"/persons", strings.NewReader(`{"name":"John","email":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTask_Create_MissingAssignee(t *testing.T) {
	cleanupDB(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgStore := store.NewOrganizationStore(testDB)
	personStore := store.NewPersonStore(testDB)
	projStore := store.NewProjectStore(testDB)
	teamStore := store.NewTeamStore(testDB)
	memberStore := store.NewTeamMembershipStore(testDB)
	taskStore := store.NewTaskStore(testDB)
	integStore := store.NewIntegrationStore(testDB)

	integSvc := service.NewIntegrationService(integStore, "key")
	taskSvc := service.NewTaskService(taskStore, memberStore, integSvc)

	orgH := NewOrganizationHandler(service.NewOrganizationService(orgStore))
	personH := NewPersonHandler(service.NewPersonService(personStore))
	projH := NewProjectHandler(service.NewProjectService(projStore))
	teamH := NewTeamHandler(service.NewTeamService(teamStore), service.NewTeamMembershipService(memberStore))
	taskH := NewTaskHandler(taskSvc)
	registerAllRoutes(e, orgH, personH, projH, teamH, taskH, nil, nil)

	org := mustCreate(t, e, "POST", "/api/orgs/", `{"name":"Org"}`)
	orgID := gjsonPath(org, "id")
	mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/persons", `{"name":"John","email":"john@test.com"}`)
	project := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project"}`)
	projectID := gjsonPath(project, "id")

	req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/tasks", strings.NewReader(`{"name":"Task"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTimeEntry_ClockInOut(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/time-entries/clock-in",
		strings.NewReader(`{"task_id":"`+app.taskID+`","person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("clock-in expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/time-entries/clock-out",
		strings.NewReader(`{"person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clock-out expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTimeEntry_ClockIn_MissingTask(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/time-entries/clock-in",
		strings.NewReader(`{"task_id":"00000000-0000-0000-0000-000000000000","person_id":"`+app.personID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTask_LinkUnlinkExternalItem(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/tasks/"+app.taskID+"/link-external-item",
		strings.NewReader(`{"integration_id":"00000000-0000-0000-0000-000000000000","external_item_id":"42","external_item_url":"https://example.com/42"}`))
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

func TestIntegration_Create(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/integrations",
		strings.NewReader(`{"type":"github","display_name":"GitHub","config":{"token":"test","repo":"owner/repo"},"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["config"] != nil {
		t.Error("config should not be returned in response")
	}
}

func TestIntegration_Create_InvalidType(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("POST", "/api/projects/"+app.projectID+"/integrations",
		strings.NewReader(`{"type":"invalid","display_name":"Bad","config":{},"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTasks_UpdateAndDelete(t *testing.T) {
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

func TestNotFound(t *testing.T) {
	app := setupTestApp(t)

	req := httptest.NewRequest("GET", "/api/orgs/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	app.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
