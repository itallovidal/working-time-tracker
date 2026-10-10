package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// acctExpect confere o status, o código do erro e a chave do campo (params.field) de uma resposta.
func acctExpect(t *testing.T, name string, rec *httptest.ResponseRecorder, status int, code, field string) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("%s: status = %d, want %d: %s", name, rec.Code, status, rec.Body.String())
		return
	}
	detail, _ := decode(t, rec)["error"].(map[string]any)
	if detail["code"] != code {
		t.Errorf("%s: code = %v, want %s: %s", name, detail["code"], code, rec.Body.String())
		return
	}
	params, _ := detail["params"].(map[string]any)
	if field != "" && params["field"] != field {
		t.Errorf("%s: params.field = %v, want %q: %s", name, params["field"], field, rec.Body.String())
	}
}

func acctLong(n int) string { return strings.Repeat("a", n) }

func acctEmail(n int) string { return strings.Repeat("a", n-len("@b.co")) + "@b.co" }

func TestFieldErrors_Signup(t *testing.T) {
	e := newServer(t)
	signup(t, e, "Org", "ana@test.com")
	body := func(org, name, email, pass string) string {
		return `{"organization_name":"` + org + `","name":"` + name + `","email":"` + email + `","password":"` + pass + `"}`
	}
	cases := []struct {
		name, body string
		status     int
		code       string
		field      string
	}{
		{"organização vazia", body("   ", "Ana", "n@test.com", "senha-forte-1"), 400, "auth.org_name_required", "organization_name"},
		{"organização no teto+1", body(acctLong(121), "Ana", "n@test.com", "senha-forte-1"), 400, "request.field_too_long", "organization_name"},
		{"nome vazio", body("Org", "", "n@test.com", "senha-forte-1"), 400, "auth.name_required", "name"},
		{"nome só espaços", body("Org", "   ", "n@test.com", "senha-forte-1"), 400, "auth.name_required", "name"},
		{"nome no teto+1", body("Org", acctLong(121), "n@test.com", "senha-forte-1"), 400, "request.field_too_long", "name"},
		{"e-mail sem domínio", body("Org", "Ana", "n@test", "senha-forte-1"), 400, "person.invalid_email", "email"},
		{"e-mail no teto+1", body("Org", "Ana", acctEmail(256), "senha-forte-1"), 400, "request.field_too_long", "email"},
		{"e-mail repetido em outra caixa", body("Org", "Ana", "ANA@test.com", "senha-forte-1"), 409, "person.email_in_use", "email"},
		{"senha curta", body("Org", "Ana", "n@test.com", "curta"), 400, "auth.weak_password", "password"},
		{"senha acima de 72 bytes", body("Org", "Ana", "n@test.com", acctLong(73)), 400, "auth.long_password", "password"},
		{"país", `{"organization_name":"Org","name":"Ana","email":"n@test.com","password":"senha-forte-1","country":"Marte"}`, 400, "auth.invalid_country", "country"},
	}
	for _, tc := range cases {
		acctExpect(t, tc.name, do(e, "POST", "/api/auth/signup", tc.body, ""), tc.status, tc.code, tc.field)
	}
	// No teto, com espaços e maiúsculas.
	rec := do(e, "POST", "/api/auth/signup", body(" "+acctLong(120)+" ", acctLong(120), strings.ToUpper(acctEmail(255)), "senha-forte-1"), "")
	if rec.Code != http.StatusCreated {
		t.Errorf("signup at the limits = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFieldErrors_AcceptInvite(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	rec := do(e, "POST", "/api/orgs/"+admin.orgID+"/invites", `{"email":"bia@test.com"}`, admin.session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite = %d: %s", rec.Code, rec.Body.String())
	}
	path := "/api/auth/invites/" + decode(t, rec)["token"].(string) + "/accept"

	acctExpect(t, "nome vazio", do(e, "POST", path, `{"name":" ","email":"bia@test.com","password":"senha-forte-1"}`, ""), 400, "auth.name_required", "name")
	acctExpect(t, "nome no teto+1", do(e, "POST", path, `{"name":"`+acctLong(121)+`","email":"bia@test.com","password":"senha-forte-1"}`, ""), 400, "request.field_too_long", "name")
	acctExpect(t, "e-mail de outro convite", do(e, "POST", path, `{"name":"Bia","email":"outra@test.com","password":"senha-forte-1"}`, ""), 400, "auth.invite_email_mismatch", "email")
	acctExpect(t, "e-mail inválido", do(e, "POST", path, `{"name":"Bia","email":"bia","password":"senha-forte-1"}`, ""), 400, "person.invalid_email", "email")
	acctExpect(t, "senha curta", do(e, "POST", path, `{"name":"Bia","email":"bia@test.com","password":"x"}`, ""), 400, "auth.weak_password", "password")
}

func TestFieldErrors_ChangePassword(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com") // senha: senha-forte-1
	cases := []struct {
		name, body string
		status     int
		code       string
		field      string
	}{
		{"atual errada", `{"current_password":"errada-errada","new_password":"outra-senha-1"}`, 400, "auth.wrong_password", "current_password"},
		{"nova igual à atual", `{"current_password":"senha-forte-1","new_password":"senha-forte-1"}`, 400, "auth.same_password", "new_password"},
		{"nova curta", `{"current_password":"senha-forte-1","new_password":"curta"}`, 400, "auth.weak_password", "new_password"},
		{"nova acima de 72 bytes", `{"current_password":"senha-forte-1","new_password":"` + acctLong(73) + `"}`, 400, "auth.long_password", "new_password"},
	}
	for _, tc := range cases {
		acctExpect(t, tc.name, do(e, "POST", "/api/auth/password", tc.body, admin.session), tc.status, tc.code, tc.field)
	}
	if rec := do(e, "POST", "/api/auth/password", `{"current_password":"senha-forte-1","new_password":"senha-forte-2"}`, admin.session); rec.Code != http.StatusNoContent {
		t.Errorf("a valid change = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFieldErrors_Invites(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	path := "/api/orgs/" + admin.orgID + "/invites"

	acctExpect(t, "e-mail inválido", do(e, "POST", path, `{"email":"bia"}`, admin.session), 400, "person.invalid_email", "email")
	acctExpect(t, "e-mail no teto+1", do(e, "POST", path, `{"email":"`+acctEmail(256)+`"}`, admin.session), 400, "request.field_too_long", "email")
	acctExpect(t, "e-mail de conta existente", do(e, "POST", path, `{"email":"BIA@test.com"}`, admin.session), 409, "auth.account_exists", "email")
	acctExpect(t, "papel desconhecido", do(e, "POST", path, `{"role":"dono"}`, admin.session), 400, "person.invalid_role", "role")
	if rec := do(e, "POST", path, `{"email":"`+acctEmail(255)+`"}`, admin.session); rec.Code != http.StatusCreated {
		t.Errorf("invite at the limit = %d: %s", rec.Code, rec.Body.String())
	}
	// A regra de papel é uma só, na lista de pessoas e nos primeiros passos (a mesma rota): convidar admin é do dono.
	carol := invite(t, e, admin, "carol@test.com", "member")
	if rec := do(e, "PATCH", "/api/persons/"+carol.id+"/role", `{"role":"admin"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d: %s", rec.Code, rec.Body.String())
	}
	acctExpect(t, "admin que não é dono convidando admin", do(e, "POST", path, `{"role":"admin"}`, carol.session), 403, "auth.owner_only", "")
	if rec := do(e, "POST", path, `{"role":"member"}`, carol.session); rec.Code != http.StatusCreated {
		t.Errorf("admin inviting a member = %d", rec.Code)
	}
	_ = bia
}

func TestFieldErrors_Organization(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	path := "/api/orgs/" + admin.orgID
	cases := []struct {
		name, body string
		status     int
		code       string
		field      string
	}{
		{"nome vazio", `{"name":"  "}`, 400, "organization.name_required", "name"},
		{"nome no teto+1", `{"name":"` + acctLong(121) + `"}`, 400, "organization.field_too_long", "name"},
		{"site sem ponto", `{"website":"https://localhost"}`, 400, "organization.invalid_url", "website"},
		{"site acima do teto", `{"website":"https://` + acctLong(250) + `.com"}`, 400, "organization.invalid_url", "website"},
		{"LinkedIn inválido", `{"linkedin_url":"não é link"}`, 400, "organization.invalid_url", "linkedin_url"},
		{"e-mail inválido", `{"contact_email":"contato@acme"}`, 400, "organization.invalid_email", "contact_email"},
		{"CNPJ inválido", `{"cnpj":"11.222.333/0001-80"}`, 400, "organization.invalid_cnpj", "cnpj"},
		{"fuso", `{"timezone":"Marte/Olympus"}`, 400, "organization.invalid_timezone", "timezone"},
		{"tipo errado", `{"name":5}`, 400, "request.invalid_body", ""},
	}
	for _, tc := range cases {
		acctExpect(t, tc.name, do(e, "PATCH", path, tc.body, admin.session), tc.status, tc.code, tc.field)
	}
	if rec := do(e, "PATCH", path, `{"name":"`+acctLong(120)+`","website":"exemplo.com.br","contact_email":"`+acctEmail(255)+`"}`, admin.session); rec.Code != http.StatusOK {
		t.Errorf("update at the limits = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFieldErrors_DeleteOrganization(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	carol := invite(t, e, admin, "carol@test.com", "member")
	if rec := do(e, "PATCH", "/api/persons/"+carol.id+"/role", `{"role":"admin"}`, admin.session); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d", rec.Code)
	}
	path := "/api/orgs/" + admin.orgID
	// Só o dono exclui: outro admin recebe 403 com o código certo, e a organização continua.
	acctExpect(t, "admin que não é dono", do(e, "DELETE", path, "", carol.session), 403, "auth.owner_only", "")
	acctExpect(t, "membro", do(e, "DELETE", path, "", invite(t, e, admin, "dan@test.com", "member").session), 403, "auth.owner_only", "")
	if rec := do(e, "GET", path, "", admin.session); rec.Code != http.StatusOK {
		t.Fatalf("the organization is gone: %d", rec.Code)
	}
	// Com projeto, nem o dono: é conflito (409), e não corpo inválido.
	createProject(t, e, admin, "P")
	acctExpect(t, "com projetos", do(e, "DELETE", path, "", admin.session), 409, "organization.has_projects", "")
}

func TestFieldErrors_Person(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	bia := invite(t, e, admin, "bia@test.com", "member")
	path := "/api/persons/" + bia.id

	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
		field                    string
	}{
		{"nome vazio", "PATCH", path, `{"name":"  ","email":"bia@test.com"}`, 400, "person.name_required", "name"},
		{"nome no teto+1", "PATCH", path, `{"name":"` + acctLong(121) + `","email":"bia@test.com"}`, 400, "request.field_too_long", "name"},
		{"e-mail ausente", "PATCH", path, `{"name":"Bia"}`, 400, "person.email_required", "email"},
		{"e-mail inválido", "PATCH", path, `{"name":"Bia","email":"bia@"}`, 400, "person.invalid_email", "email"},
		{"e-mail no teto+1", "PATCH", path, `{"name":"Bia","email":"` + acctEmail(256) + `"}`, 400, "request.field_too_long", "email"},
		{"e-mail em uso", "PATCH", path, `{"name":"Bia","email":"ANA@test.com"}`, 409, "person.email_in_use", "email"},
		{"jornada ausente", "PATCH", path + "/weekly-hours", `{}`, 400, "request.field_required", "weekly_hours"},
		{"jornada acima de 168", "PATCH", path + "/weekly-hours", `{"weekly_hours":169}`, 400, "person.invalid_week_hours", "weekly_hours"},
		{"jornada negativa", "PATCH", path + "/weekly-hours", `{"weekly_hours":-1}`, 400, "person.invalid_week_hours", "weekly_hours"},
		{"jornada com texto", "PATCH", path + "/weekly-hours", `{"weekly_hours":"20"}`, 400, "request.invalid_body", ""},
		{"pagamento vazio", "PATCH", path + "/payment", `{}`, 400, "request.field_required", "frequency"},
		{"pagamento sem frequência", "PATCH", path + "/payment", `{"day":5}`, 400, "request.field_required", "frequency"},
		{"frequência desconhecida", "PATCH", path + "/payment", `{"frequency":"yearly"}`, 400, "person.invalid_payment_frequency", "frequency"},
		{"dia 0", "PATCH", path + "/payment", `{"frequency":"monthly","day":0}`, 400, "person.invalid_payment_day", "day"},
		{"início ausente", "PATCH", path + "/payment", `{"frequency":"biweekly"}`, 400, "person.invalid_payment_start", "start"},
		{"papel", "PATCH", path + "/role", `{"role":"dono"}`, 400, "person.invalid_role", "role"},
	}
	for _, tc := range cases {
		acctExpect(t, tc.name, do(e, tc.method, tc.path, tc.body, admin.session), tc.status, tc.code, tc.field)
	}

	// Null e zero apagam a jornada; frequência vazia apaga a regra; no teto passa.
	for _, ok := range []struct{ path, body string }{
		{path + "/weekly-hours", `{"weekly_hours":null}`},
		{path + "/weekly-hours", `{"weekly_hours":0}`},
		{path + "/weekly-hours", `{"weekly_hours":168}`},
		{path + "/payment", `{"frequency":""}`},
		{path, `{"name":"` + acctLong(120) + `","email":"` + acctEmail(255) + `"}`},
	} {
		if rec := do(e, "PATCH", ok.path, ok.body, admin.session); rec.Code != http.StatusOK {
			t.Errorf("PATCH %s %s = %d: %s", ok.path, ok.body, rec.Code, rec.Body.String())
		}
	}
}

func TestFieldErrors_PaymentHistory(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	path := "/api/persons/" + admin.id + "/payments"
	for _, bad := range []string{"abc", "0", "-3", "1.5"} {
		acctExpect(t, "history="+bad, do(e, "GET", path+"?history="+bad, "", admin.session), 400, "payment.invalid_history", "history")
	}
	if rec := do(e, "GET", path+"?history=2", "", admin.session); rec.Code != http.StatusOK {
		t.Errorf("history=2 = %d: %s", rec.Code, rec.Body.String())
	}
}
