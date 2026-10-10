package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// projErrorOf devolve o código e os parâmetros de uma resposta de erro.
func projErrorOf(t *testing.T, body string) (code string, params map[string]any) {
	t.Helper()
	var wrapper struct {
		Error struct {
			Code   string         `json:"code"`
			Params map[string]any `json:"params"`
		} `json:"error"`
	}
	if err := projJSONUnmarshal(body, &wrapper); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return wrapper.Error.Code, wrapper.Error.Params
}

// Cliente, projeto, time, valor por hora e convite de projeto: o corpo recusado devolve status, código e o campo
// (a chave do corpo) para a tela marcá-lo.
func TestValidation_CustomersProjectsTeamsAllocations(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	orgID := admin.orgID

	rec := do(e, "POST", "/api/orgs/"+orgID+"/customers", `{"name":"Cliente"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create customer = %d: %s", rec.Code, rec.Body.String())
	}
	customerID := decode(t, rec)["id"].(string)
	projectID := createProject(t, e, admin, "Projeto")
	rec = do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"Time"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create team = %d: %s", rec.Code, rec.Body.String())
	}
	teamID := decode(t, rec)["id"].(string)
	allocate(t, e, admin, projectID, member.id, 1000)

	rep := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name, method, path, body string
		status                   int
		code, field              string
		max                      float64 // 0: sem max
	}{
		// Cliente.
		{"customer name empty", "POST", "/api/orgs/" + orgID + "/customers", `{"name":""}`, 400, "customer.name_required", "name", 0},
		{"customer name missing", "POST", "/api/orgs/" + orgID + "/customers", `{}`, 400, "customer.name_required", "name", 0},
		{"customer name spaces", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"   "}`, 400, "customer.name_required", "name", 0},
		{"customer name over limit", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"` + rep(121) + `"}`, 400, "customer.name_too_long", "name", 120},
		{"customer contact over limit", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"X","contact_name":"` + rep(121) + `"}`, 400, "customer.contact_too_long", "contact_name", 120},
		{"customer email format", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"X","contact_email":"carla@empresa"}`, 400, "request.field_invalid", "contact_email", 0},
		{"customer email too long", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"X","contact_email":"` + rep(250) + `@ex.com"}`, 400, "request.field_too_long", "contact_email", 255},
		{"customer phone", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"X","contact_phone":"--------"}`, 400, "customer.invalid_phone", "contact_phone", 0},
		{"customer document", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"X","document":"11.222.333/0001-80"}`, 400, "customer.invalid_document", "document", 0},
		{"customer country", "PATCH", "/api/customers/" + customerID, `{"country":"ZZZ"}`, 400, "customer.invalid_country", "country", 0},
		{"customer patch name spaces", "PATCH", "/api/customers/" + customerID, `{"name":"  "}`, 400, "customer.name_required", "name", 0},

		// Projeto.
		{"project name empty", "POST", "/api/orgs/" + orgID + "/projects", `{"name":""}`, 400, "project.name_required", "name", 0},
		{"project name spaces", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"  "}`, 400, "project.name_required", "name", 0},
		{"project name over limit", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"` + rep(121) + `"}`, 400, "request.field_too_long", "name", 120},
		{"project description over limit", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","description":"` + rep(2001) + `"}`, 400, "request.field_too_long", "long_description", 2000},
		{"project sprint zero on create", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","sprint_duration_days":0}`, 400, "project.invalid_sprint", "sprint_duration_days", 0},
		{"project sprint over", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","sprint_duration_days":91}`, 400, "project.invalid_sprint", "sprint_duration_days", 0},
		{"project meeting without customer", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","customer_meeting_day":"monday"}`, 400, "project.meeting_needs_customer", "customer_meeting_day", 0},
		{"project meeting day invalid", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","customer_meeting_day":"someday"}`, 400, "project.invalid_weekday", "customer_meeting_day", 0},
		{"project meeting time without day", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","customer_meeting_time":"10:00"}`, 400, "project.customer_meeting_time_without_day", "customer_meeting_time", 0},
		{"project meeting day without time", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","customer_id":"` + customerID + `","customer_meeting_day":"monday"}`, 400, "request.field_required", "customer_meeting_time", 0},
		{"project weekly day without time", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","weekly_sync_day":"friday"}`, 400, "request.field_required", "weekly_sync_time", 0},
		{"project weekly day with empty time", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","weekly_sync_day":"friday","weekly_sync_time":""}`, 400, "request.field_required", "weekly_sync_time", 0},
		{"project patch weekly day without time", "PATCH", "/api/projects/" + projectID, `{"name":"Projeto","weekly_sync_day":"friday"}`, 400, "request.field_required", "weekly_sync_time", 0},
		{"project daily time", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","daily_time":"25:00"}`, 400, "project.invalid_daily_time", "daily_time", 0},
		{"project weekly time", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","weekly_sync_day":"friday","weekly_sync_time":"9"}`, 400, "project.invalid_weekly_time", "weekly_sync_time", 0},
		{"project customer of nobody", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"P","customer_id":"00000000-0000-0000-0000-000000000000"}`, 400, "project.customer_not_found", "customer_id", 0},
		{"project patch meeting without customer", "PATCH", "/api/projects/" + projectID, `{"name":"Projeto","customer_meeting_day":"monday"}`, 400, "project.meeting_needs_customer", "customer_meeting_day", 0},
		{"project patch sprint zero", "PATCH", "/api/projects/" + projectID, `{"name":"Projeto","sprint_duration_days":0}`, 400, "project.invalid_sprint", "sprint_duration_days", 0},
		{"project patch name over limit", "PATCH", "/api/projects/" + projectID, `{"name":"` + rep(121) + `"}`, 400, "request.field_too_long", "name", 120},
		{"project billing over limit", "PUT", "/api/projects/" + projectID + "/billing", `{"bill_rate_cents":100000001}`, 400, "project.invalid_bill_rate", "bill_rate_cents", 0},
		{"project billing negative", "PUT", "/api/projects/" + projectID + "/billing", `{"bill_rate_cents":-1}`, 400, "project.invalid_bill_rate", "bill_rate_cents", 0},
		{"project billing customer", "PUT", "/api/projects/" + projectID + "/billing", `{"customer_id":"nao-e-uuid"}`, 400, "project.customer_not_found", "customer_id", 0},

		// Time.
		{"team name empty", "POST", "/api/projects/" + projectID + "/teams", `{"name":""}`, 400, "team.name_required", "name", 0},
		{"team name spaces", "POST", "/api/projects/" + projectID + "/teams", `{"name":"   "}`, 400, "team.name_required", "name", 0},
		{"team name over limit", "POST", "/api/projects/" + projectID + "/teams", `{"name":"` + rep(121) + `"}`, 400, "request.field_too_long", "name", 120},
		{"team patch name spaces", "PATCH", "/api/teams/" + teamID, `{"name":" "}`, 400, "team.name_required", "name", 0},
		{"team patch name over limit", "PATCH", "/api/teams/" + teamID, `{"name":"` + rep(121) + `"}`, 400, "request.field_too_long", "name", 120},
		{"team add without person", "POST", "/api/teams/" + teamID + "/members", `{}`, 400, "team.person_required", "person_id", 0},
		{"team add malformed person", "POST", "/api/teams/" + teamID + "/members", `{"person_id":"xyz"}`, 400, "request.field_invalid", "person_id", 0},
		{"team remove malformed person", "DELETE", "/api/teams/" + teamID + "/members", `{"person_id":"xyz"}`, 400, "request.field_invalid", "person_id", 0},
		{"team remove without person", "DELETE", "/api/teams/" + teamID + "/members", `{}`, 400, "team.person_required", "person_id", 0},

		// Valor por hora.
		{"rate negative", "PUT", "/api/projects/" + projectID + "/allocations/" + member.id, `{"pay_rate_cents":-1}`, 400, "allocation.invalid_rate", "pay_rate_cents", 0},
		{"rate over limit", "PUT", "/api/projects/" + projectID + "/allocations/" + member.id, `{"pay_rate_cents":100000001}`, 400, "allocation.invalid_rate", "pay_rate_cents", 0},
		{"rate missing", "PUT", "/api/projects/" + projectID + "/allocations/" + admin.id, `{}`, 400, "allocation.rate_required", "pay_rate_cents", 0},
		{"preset invalid", "PUT", "/api/projects/" + projectID + "/allocations/" + member.id, `{"preset":"deus"}`, 400, "allocation.invalid_preset", "preset", 0},
		{"invite rate missing", "POST", "/api/projects/" + projectID + "/invites", `{"email":"x@test.com"}`, 400, "allocation.rate_required", "pay_rate_cents", 0},
		{"invite rate over limit", "POST", "/api/projects/" + projectID + "/invites", `{"email":"x@test.com","pay_rate_cents":100000001}`, 400, "allocation.invalid_rate", "pay_rate_cents", 0},
		{"invite team elsewhere", "POST", "/api/projects/" + projectID + "/invites", `{"email":"x@test.com","pay_rate_cents":100,"team_id":"nao-e-uuid"}`, 400, "projectinvite.team_not_in_project", "team_id", 0},
	}
	for _, tc := range cases {
		rec := do(e, tc.method, tc.path, tc.body, admin.session)
		code, params := projErrorOf(t, rec.Body.String())
		if rec.Code != tc.status || code != tc.code {
			t.Errorf("%s = %d %s, want %d %s", tc.name, rec.Code, rec.Body.String(), tc.status, tc.code)
			continue
		}
		if params["field"] != tc.field {
			t.Errorf("%s: field = %v, want %q (%s)", tc.name, params["field"], tc.field, rec.Body.String())
		}
		if tc.max != 0 && params["max"] != tc.max {
			t.Errorf("%s: max = %v, want %v", tc.name, params["max"], tc.max)
		}
	}

	// Os limites valem: nome no teto, valor no teto e descrição no teto passam.
	ok := []struct{ name, method, path, body string }{
		{"customer name at limit", "POST", "/api/orgs/" + orgID + "/customers", `{"name":"` + rep(120) + `"}`},
		{"project name and description at limit", "POST", "/api/orgs/" + orgID + "/projects", `{"name":"` + rep(120) + `","description":"` + rep(2000) + `"}`},
		{"team name at limit", "POST", "/api/projects/" + projectID + "/teams", `{"name":"` + rep(120) + `"}`},
		{"rate at limit", "PUT", "/api/projects/" + projectID + "/allocations/" + member.id, `{"pay_rate_cents":100000000}`},
		{"billing at limit", "PUT", "/api/projects/" + projectID + "/billing", `{"bill_rate_cents":100000000}`},
	}
	for _, tc := range ok {
		rec := do(e, tc.method, tc.path, tc.body, admin.session)
		if rec.Code >= 300 {
			t.Errorf("%s = %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

// Um projeto com cliente aceita a reunião, e o cliente volta na resposta da criação.
func TestValidation_ProjectWithCustomerAndMeeting(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/customers", `{"name":"Cliente"}`, admin.session)
	customerID := decode(t, rec)["id"].(string)

	rec = do(e, "POST", "/api/orgs/"+admin.orgID+"/projects",
		`{"name":"P","customer_id":"`+customerID+`","customer_meeting_day":"Monday","customer_meeting_time":"10:30"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	customer, _ := got["customer"].(map[string]any)
	if customer["name"] != "Cliente" || got["customer_meeting_day"] != "monday" || got["sprint_duration_days"] != float64(14) {
		t.Errorf("created project = %v", got)
	}

	// PATCH sem description mantém; "" apaga.
	id := got["id"].(string)
	do(e, "PATCH", "/api/projects/"+id, `{"name":"P","description":"texto"}`, admin.session)
	rec = do(e, "PATCH", "/api/projects/"+id, `{"name":"P2"}`, admin.session)
	if body := decode(t, rec); body["description"] != "texto" || body["sprint_duration_days"] != float64(14) {
		t.Errorf("omitted fields should be kept: %v", body)
	}
	rec = do(e, "PATCH", "/api/projects/"+id, `{"name":"P2","description":""}`, admin.session)
	if body := decode(t, rec); body["description"] != "" {
		t.Errorf("empty description should clear it: %v", body)
	}

	// O nome também mantém quando não vem (ausente ou null), como na organização e no cliente. Em branco é recusado.
	for _, body := range []string{`{}`, `{"description":"outra"}`, `{"name":null}`, `{"name":null,"description":"outra"}`} {
		rec = do(e, "PATCH", "/api/projects/"+id, body, admin.session)
		if got := decode(t, rec); rec.Code != http.StatusOK || got["name"] != "P2" {
			t.Errorf("PATCH %s = %d, name %v; want 200 and the name kept", body, rec.Code, got["name"])
		}
	}
	for _, body := range []string{`{"name":""}`, `{"name":"   "}`} {
		rec = do(e, "PATCH", "/api/projects/"+id, body, admin.session)
		if code, params := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "project.name_required" || params["field"] != "name" {
			t.Errorf("PATCH %s = %d %s, want 400 project.name_required on name", body, rec.Code, rec.Body.String())
		}
	}
	if got := decode(t, do(e, "GET", "/api/projects/"+id, "", admin.session)); got["name"] != "P2" {
		t.Errorf("a refused blank name changed the name to %v", got["name"])
	}
}

// Apagar um cliente com projetos é conflito (409), não corpo inválido.
func TestValidation_CustomerWithProjectsIsAConflict(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/customers", `{"name":"Cliente"}`, admin.session)
	customerID := decode(t, rec)["id"].(string)
	projectID := createProject(t, e, admin, "P")
	if rec := do(e, "PUT", "/api/projects/"+projectID+"/billing", `{"customer_id":"`+customerID+`"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("set billing = %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "DELETE", "/api/customers/"+customerID, "", admin.session)
	if code, _ := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusConflict || code != "customer.has_projects" {
		t.Errorf("delete = %d %s", rec.Code, rec.Body.String())
	}
}

// Uma corrida no cadastro de membro vira team.already_member, e o segundo cadastro também.
func TestValidation_TeamAlreadyMember(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")
	projectID := createProject(t, e, admin, "P")
	allocate(t, e, admin, projectID, member.id, 1000)
	rec := do(e, "POST", "/api/projects/"+projectID+"/teams", `{"name":"T"}`, admin.session)
	teamID := decode(t, rec)["id"].(string)

	body := `{"person_id":"` + member.id + `"}`
	if rec := do(e, "POST", "/api/teams/"+teamID+"/members", body, admin.session); rec.Code != http.StatusCreated {
		t.Fatalf("add = %d: %s", rec.Code, rec.Body.String())
	}
	rec = do(e, "POST", "/api/teams/"+teamID+"/members", body, admin.session)
	if code, params := projErrorOf(t, rec.Body.String()); rec.Code != http.StatusBadRequest || code != "team.already_member" || params["field"] != "person_id" {
		t.Errorf("add twice = %d %s", rec.Code, rec.Body.String())
	}
}

func projJSONUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
