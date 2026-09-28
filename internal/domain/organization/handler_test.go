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

func newTestEcho(svc *organization.Service) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Recover())
	orgH := organization.NewHandler(svc)
	orgs := e.Group("/api/orgs")
	orgs.GET("/:orgId", orgH.Get)
	orgs.PATCH("/:orgId", orgH.Update)
	orgs.DELETE("/:orgId", orgH.Delete)
	return e
}

func TestHandler_GetAndUpdate(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	e := newTestEcho(svc)
	org, _ := svc.Create("Minha Org")

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/orgs/"+org.ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest("PATCH", "/api/orgs/"+org.ID.String(), strings.NewReader(`{"name":"Novo Nome"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Novo Nome") {
		t.Errorf("update response does not contain the new name: %s", rec.Body.String())
	}
}

func TestHandler_Update_EmptyName(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	e := newTestEcho(svc)
	org, _ := svc.Create("Org")

	req := httptest.NewRequest("PATCH", "/api/orgs/"+org.ID.String(), strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_NotFound(t *testing.T) {
	cleanup(t)
	e := newTestEcho(organization.NewService(organization.NewStore(testClient)))

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", "/api/orgs/00000000-0000-0000-0000-000000000000", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
