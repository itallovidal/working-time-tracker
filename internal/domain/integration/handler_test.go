package integration_test

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
	"working-time-tracker/internal/domain/project"
)

func TestHandler_Create(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	integSvc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	orgH := organization.NewHandler(orgSvc)
	projH := project.NewHandler(projSvc)
	integH := integration.NewHandler(integSvc)
	registerRoutes(e, orgH, projH, integH)

	org, err := orgSvc.Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	orgID := org.ID.String()
	proj := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project"}`)
	projectID := jsonPath(proj, "id")

	req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/integrations",
		strings.NewReader(`{"type":"github","display_name":"GitHub","config":{"token":"test","repo":"owner/repo"},"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["config"] != nil {
		t.Error("config should not be returned in response")
	}
}

func TestHandler_Create_InvalidType(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	integSvc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	orgH := organization.NewHandler(orgSvc)
	projH := project.NewHandler(projSvc)
	integH := integration.NewHandler(integSvc)
	registerRoutes(e, orgH, projH, integH)

	org, err := orgSvc.Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	orgID := org.ID.String()
	proj := mustCreate(t, e, "POST", "/api/orgs/"+orgID+"/projects", `{"name":"Project"}`)
	projectID := jsonPath(proj, "id")

	req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/integrations",
		strings.NewReader(`{"type":"invalid","display_name":"Bad","config":{},"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d: %s", rec.Code, rec.Body.String())
	}
}

func registerRoutes(
	e *echo.Echo,
	orgH *organization.Handler,
	projH *project.Handler,
	integH *integration.Handler,
) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/:orgId/projects", projH.Create)

	projects := e.Group("/api/projects")
	projects.POST("/:projectId/integrations", integH.Create)
	projects.GET("/:projectId/integrations", integH.ListByProject)
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
