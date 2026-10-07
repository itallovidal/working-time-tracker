package server_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/i18n"
)

var placeholderNames = regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)

// flatCatalog lista as mensagens simples de um idioma (chave -> texto).
func flatCatalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	cat, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var walk func(prefix string, node any)
	walk = func(prefix string, node any) {
		switch v := node.(type) {
		case string:
			out[prefix] = v
		case map[string]any:
			for k, child := range v {
				key := k
				if prefix != "" {
					key = prefix + "." + k
				}
				walk(key, child)
			}
		}
	}
	walk("", cat.Tree(lang))
	return out
}

// Todo código que a API pode devolver tem texto em todos os idiomas, e o texto só usa
// os parâmetros que o código declara. O contrário também vale: nenhum texto de erro
// sobra no catálogo sem um código por trás.
func TestErrorCodes_HaveTextsInEveryLanguage(t *testing.T) {
	registered := apperr.Registered()
	if len(registered) < 50 {
		t.Fatalf("only %d codes registered: are the domain packages imported?", len(registered))
	}
	for _, lang := range i18n.Supported() {
		texts := flatCatalog(t, lang)
		for code, info := range registered {
			text, ok := texts["errors."+code]
			if !ok {
				t.Errorf("%s has no text for the code %q (errors.%s)", lang, code, code)
				continue
			}
			allowed := map[string]bool{}
			for _, p := range info.Params {
				allowed[p] = true
			}
			for _, m := range placeholderNames.FindAllStringSubmatch(text, -1) {
				if !allowed[m[1]] {
					t.Errorf("%s: errors.%s uses {{.%s}}, but the code declares %v", lang, code, m[1], info.Params)
				}
			}
			for _, p := range info.Params {
				if !strings.Contains(text, "{{."+p+"}}") {
					t.Errorf("%s: errors.%s never uses the declared parameter %q", lang, code, p)
				}
			}
		}
		// Chaves errors.<domínio>.<motivo> são códigos; as de dois níveis são do cliente.
		for key := range texts {
			parts := strings.Split(key, ".")
			if parts[0] != "errors" || len(parts) != 3 {
				continue
			}
			if _, ok := registered[parts[1]+"."+parts[2]]; !ok {
				t.Errorf("%s has %q, but no code %s.%s exists", lang, key, parts[1], parts[2])
			}
		}
	}
}

// A API responde só com o código: nenhuma resposta de erro leva texto.
func TestErrors_ResponsesCarryOnlyTheCode(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	member := invite(t, e, admin, "bia@test.com", "member")

	cases := []struct {
		name, method, path, body, session string
		status                            int
		code                              string
	}{
		{"wrong login", "POST", "/api/auth/login", `{"email":"ana@test.com","password":"errada-errada"}`, "", 401, "auth.invalid_credentials"},
		{"no session", "GET", "/api/auth/me", "", "", 401, "auth.unauthenticated"},
		{"bad body", "POST", "/api/auth/login", `{`, "", 400, "request.invalid_body"},
		{"admin only", "POST", "/api/orgs/" + admin.orgID + "/customers", `{"name":"X"}`, member.session, 403, "auth.permission_required"},
		{"admin only", "PATCH", "/api/orgs/" + admin.orgID, `{"summary":"X"}`, member.session, 403, "auth.admin_only"},
		{"validation", "PATCH", "/api/orgs/" + admin.orgID, `{"cnpj":"11.222.333/0001-80"}`, admin.session, 400, "organization.invalid_cnpj"},
		{"not found in org", "GET", "/api/projects/00000000-0000-0000-0000-000000000000", "", admin.session, 404, "request.not_found"},
		{"weak password", "POST", "/api/auth/signup", `{"organization_name":"X","name":"Y","email":"y@test.com","password":"123"}`, "", 400, "auth.weak_password"},
	}
	for _, tc := range cases {
		rec := do(e, tc.method, tc.path, tc.body, tc.session)
		got := decode(t, rec)
		detail, _ := got["error"].(map[string]any)
		if rec.Code != tc.status || detail["code"] != tc.code {
			t.Errorf("%s = %d %s, want %d with code %s", tc.name, rec.Code, rec.Body.String(), tc.status, tc.code)
		}
		if strings.Contains(rec.Body.String(), "message") {
			t.Errorf("%s: the error response has a message: %s", tc.name, rec.Body.String())
		}
	}

	// Corpo que não é JSON.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), `"request.json_required"`) {
		t.Errorf("non-JSON body = %d %s", rec.Code, rec.Body.String())
	}
}

// Um texto longo demais devolve o campo e o limite, e todo campo tem rótulo no catálogo.
func TestErrors_FieldTooLongCarriesTheFieldAndTheLimit(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	limits := map[string]int{
		"name": 120, "long_description": 2000, "industry": 100, "legal_name": 200, "address_line1": 200,
		"address_line2": 200, "city": 100, "state": 100, "postal_code": 16, "country": 100, "summary": 160,
	}
	jsonName := map[string]string{"long_description": "description"}

	for _, lang := range i18n.Supported() {
		texts := flatCatalog(t, lang)
		var missing []string
		for field := range limits {
			if _, ok := texts["fields."+field]; !ok {
				missing = append(missing, field)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s has no label in fields.* for %v", lang, missing)
		}
	}

	for field, max := range limits {
		key := field
		if j, ok := jsonName[field]; ok {
			key = j
		}
		body := `{"` + key + `":"` + strings.Repeat("a", max+1) + `"}`
		rec := do(e, "PATCH", "/api/orgs/"+admin.orgID, body, admin.session)
		detail, _ := decode(t, rec)["error"].(map[string]any)
		params, _ := detail["params"].(map[string]any)
		if rec.Code != http.StatusBadRequest || detail["code"] != "organization.field_too_long" || params["field"] != field || params["max"] != float64(max) {
			t.Errorf("%s too long = %d %s", key, rec.Code, rec.Body.String())
		}
	}
}
