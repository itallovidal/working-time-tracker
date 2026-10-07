package person_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
)

func newTestEcho(svc *person.Service) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Recover())
	personH := person.NewHandler(svc)
	e.GET("/api/orgs/:orgId/persons", personH.ListByOrg)
	e.PATCH("/api/persons/:personId", personH.Update)
	e.PATCH("/api/persons/:personId/role", personH.SetRole)
	e.PATCH("/api/persons/:personId/weekly-hours", personH.SetWeeklyHours)
	return e
}

func patch(e *echo.Echo, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PATCH", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestHandler_Update_EmptyEmail(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	e := newTestEcho(svc)

	org, _ := orgSvc.Create("Org")
	p, _ := svc.Create(org.ID.String(), "John", "john@test.com")

	if rec := patch(e, "/api/persons/"+p.ID.String(), `{"name":"John","email":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandler_SetRole(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	e := newTestEcho(svc)

	org, _ := orgSvc.Create("Org")
	p, _ := svc.Create(org.ID.String(), "John", "john@test.com")

	rec := patch(e, "/api/persons/"+p.ID.String()+"/role", `{"role":"admin"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("promote expected 200 with role admin, got %d: %s", rec.Code, rec.Body.String())
	}

	// Ele é o único admin, então não pode ser rebaixado.
	if rec := patch(e, "/api/persons/"+p.ID.String()+"/role", `{"role":"member"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("demoting the last admin expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := patch(e, "/api/persons/"+p.ID.String()+"/role", `{"role":"owner"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid role expected 400, got %d", rec.Code)
	}
}

func TestHandler_SetWeeklyHours(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))
	e := newTestEcho(svc)

	org, _ := orgSvc.Create("Org")
	p, _ := svc.Create(org.ID.String(), "John", "john@test.com")
	path := "/api/persons/" + p.ID.String() + "/weekly-hours"

	rec := patch(e, path, `{"weekly_hours":40}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"weekly_hours":40`) {
		t.Fatalf("set expected 200 with 40 hours, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = patch(e, path, `{"weekly_hours":0}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"weekly_hours":null`) {
		t.Fatalf("clearing expected 200 with null, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := patch(e, path, `{"weekly_hours":200}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("200 hours expected 400, got %d", rec.Code)
	}
	if rec := patch(e, path, `{"weekly_hours":"quarenta"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("text instead of a number expected 400, got %d", rec.Code)
	}
	if rec := patch(e, "/api/persons/00000000-0000-0000-0000-000000000000/weekly-hours", `{"weekly_hours":40}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown person expected 404, got %d", rec.Code)
	}
}
