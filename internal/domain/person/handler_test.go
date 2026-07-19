package person_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
)

func TestHandler_Create_EmptyEmail(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgSvc := organization.NewService(organization.NewStore(testDB))
	orgH := organization.NewHandler(orgSvc)
	personH := person.NewHandler(person.NewService(person.NewStore(testDB)))
	registerRoutes(e, orgH, personH)

	org := mustCreate(t, e, "POST", "/api/orgs/", `{"name":"Org"}`)
	orgID := jsonPath(org, "id")

	req := httptest.NewRequest("POST", "/api/orgs/"+orgID+"/persons", strings.NewReader(`{"name":"John","email":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func registerRoutes(e *echo.Echo, orgH *organization.Handler, personH *person.Handler) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/", orgH.Create)
	orgs.GET("/", orgH.List)
	orgs.GET("/:orgId", orgH.Get)
	orgs.POST("/:orgId/persons", personH.Create)
	orgs.GET("/:orgId/persons", personH.ListByOrg)
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
