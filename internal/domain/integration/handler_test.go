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

	// O corpo é o mesmo para todos os tipos; sem enabled, a integração nasce ativa.
	req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/integrations",
		strings.NewReader(`{"type":"github","display_name":"GitHub","token":"ghp_secret","metadata":{"repo":"owner/repo"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	assertResponse(t, rec.Body.String(), "GitHub", true)
	id := jsonPath(rec.Body.String(), "id")

	// O corpo antigo, com tudo dentro de config, não é mais aceito.
	old := httptest.NewRequest("POST", "/api/projects/"+projectID+"/integrations",
		strings.NewReader(`{"type":"github","display_name":"GitHub","config":{"token":"ghp_secret","repo":"owner/repo"}}`))
	old.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, old)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Repositório") {
		t.Errorf("old body: got %d %s, want 400 naming the missing field", rec.Code, rec.Body.String())
	}

	// Edição sem token e sem metadata: muda só o que veio.
	patch := httptest.NewRequest("PATCH", "/api/integrations/"+id,
		strings.NewReader(`{"display_name":"Repositório do app","token":"","enabled":false}`))
	patch.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, patch)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	assertResponse(t, rec.Body.String(), "Repositório do app", false)

	list := httptest.NewRecorder()
	e.ServeHTTP(list, httptest.NewRequest("GET", "/api/projects/"+projectID+"/integrations", nil))
	if strings.Contains(list.Body.String(), "ghp_secret") || strings.Contains(list.Body.String(), "encrypted_data") {
		t.Errorf("list leaks the credential: %s", list.Body.String())
	}
}

// assertResponse confere a resposta comum a todos os tipos: o metadata volta, o
// token nunca.
func assertResponse(t *testing.T, body, wantName string, wantEnabled bool) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("invalid json: %v: %s", err, body)
	}
	for _, key := range []string{"token", "config", "credentials", "has_config"} {
		if _, ok := resp[key]; ok {
			t.Errorf("response has %q: %s", key, body)
		}
	}
	if strings.Contains(body, "ghp_secret") {
		t.Errorf("response leaks the token: %s", body)
	}
	if resp["has_token"] != true || resp["display_name"] != wantName || resp["enabled"] != wantEnabled {
		t.Errorf("response = %s, want has_token true, name %q and enabled %v", body, wantName, wantEnabled)
	}
	metadata, _ := resp["metadata"].(map[string]interface{})
	if len(metadata) != 1 || metadata["repo"] != "owner/repo" {
		t.Errorf("metadata = %v, want only the repository", resp["metadata"])
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
		strings.NewReader(`{"type":"invalid","display_name":"Bad","token":"x","metadata":{},"enabled":true}`))
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

	e.PATCH("/api/integrations/:integrationId", integH.Update)
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
