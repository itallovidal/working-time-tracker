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

// O PATCH aceita os campos do perfil, e o GET devolve todos eles.
func TestHandler_UpdateProfile(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	e := newTestEcho(svc)
	org, _ := svc.Create("Org")
	path := "/api/orgs/" + org.ID.String()

	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PATCH", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	if rec := patch(`{"summary":"Entregas rápidas","cnpj":"12.ABC.345/01DE-35","weekly_hours":44}`); rec.Code != http.StatusOK {
		t.Fatalf("update expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	body := rec.Body.String()
	for _, want := range []string{
		`"name":"Org"`, `"summary":"Entregas rápidas"`, `"cnpj":"12ABC34501DE35"`, `"weekly_hours":44`,
		`"founded_year":null`, `"timezone":"America/Sao_Paulo"`, `"currency":"BRL"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET body does not contain %s: %s", want, body)
		}
	}

	rec = patch(`{"cnpj":"11.222.333/0001-80"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "CNPJ inválido") {
		t.Errorf("invalid CNPJ = %d %s, want 400 with the reason", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest("PATCH", "/api/orgs/00000000-0000-0000-0000-000000000000", strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH on a missing organization = %d, want 404", rec.Code)
	}
}
