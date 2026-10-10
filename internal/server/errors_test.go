package server_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		{"owner only", "PATCH", "/api/orgs/" + admin.orgID, `{"summary":"X"}`, member.session, 403, "auth.owner_only"},
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

// fieldUses são os jeitos de um campo aparecer no código Go: o par "field" nos parâmetros de um erro
// (ErrX.With("field", "name")), o primeiro argumento dos validadores de internal/validate e a constante que
// guarda o nome (fieldName = "name"), que alimenta um dos dois.
var fieldUses = []*regexp.Regexp{
	regexp.MustCompile(`\.With\((?:[^()]|\([^()]*\))*?"field",\s*"(\w+)"`),
	regexp.MustCompile(`validate\.(?:Text|Required|Email|Money)\(\s*"(\w+)"`),
	regexp.MustCompile(`(?m)^\s*field\w*\s*=\s*"(\w+)"`),
}

// Todo campo que o código Go manda em params.field tem rótulo em fields.* nos dois idiomas: a tela mostra o
// rótulo no texto do erro. O teste lê o fonte de internal/ (sem os _test.go), então um campo novo sem rótulo
// falha aqui, e não na tela de quem o usa.
func TestErrors_EveryFieldInTheSourceHasALabel(t *testing.T) {
	used := map[string]string{} // campo -> um arquivo que o usa
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, re := range fieldUses {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				if _, ok := used[m[1]]; !ok {
					used[m[1]] = path
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sem nenhum uso o teste não está lendo o fonte: o servidor já usa campos em dezenas de lugares.
	if len(used) < 10 {
		t.Fatalf("found only %d fields in the source (%v): is the scan reading internal/?", len(used), used)
	}

	for _, lang := range i18n.Supported() {
		texts := flatCatalog(t, lang)
		var missing []string
		for field, path := range used {
			if _, ok := texts["fields."+field]; !ok {
				missing = append(missing, field+" ("+path+")")
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s has no label in fields.* for %v", lang, missing)
		}
	}
}

// Um texto longo demais devolve o campo e o limite.
func TestErrors_FieldTooLongCarriesTheFieldAndTheLimit(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	limits := map[string]int{
		"name": 120, "long_description": 2000, "legal_name": 200, "address_line1": 200,
		"address_line2": 200, "summary": 160,
	}
	jsonName := map[string]string{"long_description": "description"}

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
