package organization_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/domain/organization"
)

func TestHandler_CreateAndList(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgH := organization.NewHandler(organization.NewService(organization.NewStore(testDB)))
	registerRoutes(e, orgH)

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

func TestHandler_Create_EmptyName(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgH := organization.NewHandler(organization.NewService(organization.NewStore(testDB)))
	registerRoutes(e, orgH)

	req := httptest.NewRequest("POST", "/api/orgs/", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_NotFound(t *testing.T) {
	cleanup(t)
	e := echo.New()
	e.Use(middleware.Recover())

	orgH := organization.NewHandler(organization.NewService(organization.NewStore(testDB)))
	registerRoutes(e, orgH)

	req := httptest.NewRequest("GET", "/api/orgs/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func registerRoutes(e *echo.Echo, orgH *organization.Handler) {
	orgs := e.Group("/api/orgs")
	orgs.POST("/", orgH.Create)
	orgs.GET("/", orgH.List)
	orgs.GET("/:orgId", orgH.Get)
}
