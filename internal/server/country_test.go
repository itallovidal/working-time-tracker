package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// O país escolhido no cadastro decide a moeda e o fuso da organização nova, e aparece no perfil dela.
func TestSignup_CountryDecidesTheOrganizationDefaults(t *testing.T) {
	e := newServer(t)

	for _, tc := range []struct {
		email, body, country, currency, timezone string
	}{
		{"br@test.com", `"country":"BR",`, "BR", "BRL", "America/Sao_Paulo"},
		{"sem@test.com", ``, "BR", "BRL", "America/Sao_Paulo"},
		{"us@test.com", `"country":"US",`, "US", "USD", "America/New_York"},
	} {
		rec := do(e, "POST", "/api/auth/signup",
			`{"organization_name":"Org","name":"Admin","email":"`+tc.email+`",`+tc.body+`"password":"senha-forte-1"}`, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s: signup = %d: %s", tc.email, rec.Code, rec.Body.String())
		}
		session := sessionFrom(t, rec)
		orgID := decode(t, rec)["organization_id"].(string)

		org := decode(t, do(e, "GET", "/api/orgs/"+orgID, "", session))
		if org["country"] != tc.country || org["currency"] != tc.currency || org["timezone"] != tc.timezone {
			t.Errorf("%s: organization = %v %v %v, want %s %s %s", tc.email,
				org["country"], org["currency"], org["timezone"], tc.country, tc.currency, tc.timezone)
		}
		if me := decode(t, do(e, "GET", "/api/auth/me", "", session)); me["organization_currency"] != tc.currency {
			t.Errorf("%s: organization_currency = %v, want %s", tc.email, me["organization_currency"], tc.currency)
		}
	}

	rec := do(e, "POST", "/api/auth/signup",
		`{"organization_name":"Org","name":"Admin","email":"pt@test.com","country":"Portugal","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"auth.invalid_country"`) {
		t.Errorf("unknown country = %d %s, want 400 auth.invalid_country", rec.Code, rec.Body.String())
	}
}

// O perfil de uma empresa dos EUA pela API: EIN, estado e ZIP; e os documentos de um país não valem no outro.
func TestOrganization_ProfileOfAUSCompany(t *testing.T) {
	e := newServer(t)
	rec := do(e, "POST", "/api/auth/signup",
		`{"organization_name":"Acme Inc","country":"US","name":"Joe","email":"joe@test.com","password":"senha-forte-1"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup = %d: %s", rec.Code, rec.Body.String())
	}
	session, orgID := sessionFrom(t, rec), decode(t, rec)["organization_id"].(string)
	path := "/api/orgs/" + orgID

	if rec := do(e, "PATCH", path, `{"ein":"12-3456789","state":"tx","postal_code":"78701-1234","city":"Austin"}`, session); rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	org := decode(t, do(e, "GET", path, "", session))
	if org["ein"] != "123456789" || org["state"] != "TX" || org["postal_code"] != "78701-1234" || org["country"] != "US" {
		t.Errorf("profile = %v", org)
	}

	for body, code := range map[string]string{
		`{"ein":"123"}`:               "organization.invalid_ein",
		`{"cnpj":"12-3456789"}`:       "organization.invalid_cnpj",
		`{"state":"SP"}`:              "organization.invalid_state",
		`{"postal_code":"01310-100"}`: "organization.invalid_postal_code",
		`{"country":"DE"}`:            "organization.invalid_country",
	} {
		if rec := do(e, "PATCH", path, body, session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"`+code+`"`) {
			t.Errorf("%s = %d %s, want 400 %s", body, rec.Code, rec.Body.String(), code)
		}
	}
}

// O cadastro de países vai no window.BOOT das páginas que o desenham (o cadastro, a volta do Clerk e as páginas da
// organização), no idioma da requisição, e a tela de cadastro tem o seletor de país.
func TestPages_CountriesReachTheBrowser(t *testing.T) {
	e := newServer(t)

	rec := getPage(e, "/signup", "", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="signup-country"`) {
		t.Fatalf("/signup = %d, want the page with the country select", rec.Code)
	}
	list := func(lang string) []any {
		t.Helper()
		reg, ok := bootOf(t, getPage(e, "/signup", "", lang, "").Body.String())["countries"].(map[string]any)
		if !ok {
			t.Fatalf("/signup (%q) has no countries in window.BOOT", lang)
		}
		return reg["list"].([]any)
	}

	pt := list("pt-BR")
	if len(pt) != 2 {
		t.Fatalf("countries = %d, want BR and US", len(pt))
	}
	br, us := pt[0].(map[string]any), pt[1].(map[string]any)
	if br["code"] != "BR" || us["code"] != "US" || br["currency"] != "BRL" || us["currency"] != "USD" {
		t.Errorf("countries = %v / %v", br, us)
	}
	if id := br["legal_id"].(map[string]any); id["field"] != "cnpj" || id["label"] != "CNPJ" || id["mask"] != "**.***.***/****-99" {
		t.Errorf("BR legal id = %v", id)
	}
	if id := us["legal_id"].(map[string]any); id["field"] != "ein" || id["label"] != "EIN" || id["mask"] != "99-9999999" {
		t.Errorf("US legal id = %v", id)
	}
	if st := br["state"].(map[string]any); st["label"] != "Estado" || len(st["options"].([]any)) != 27 {
		t.Errorf("BR states = %v", st)
	}
	if st := us["state"].(map[string]any); len(st["options"].([]any)) != 59 {
		t.Errorf("US states = %d, want 59", len(st["options"].([]any)))
	}
	if postal := br["postal"].(map[string]any)["label"]; postal != "CEP" {
		t.Errorf("BR postal label = %v, want CEP", postal)
	}

	// Em inglês os textos são os de lá.
	en := list("en")
	if postal := en[1].(map[string]any)["postal"].(map[string]any)["label"]; postal != "ZIP code" {
		t.Errorf("US postal label in English = %v, want ZIP code", postal)
	}
	if state := en[0].(map[string]any)["state"].(map[string]any)["label"]; state != "State" {
		t.Errorf("BR state label in English = %v, want State", state)
	}

	// As páginas que não desenham países não carregam o cadastro.
	if _, has := bootOf(t, getPage(e, "/help", "", "", "").Body.String())["countries"]; has {
		t.Error("/help must not carry the countries")
	}

	// A configuração da organização traz o cadastro e o país da organização.
	admin := signup(t, e, "Org", "ana@test.com")
	settings := getPage(e, "/orgs/"+admin.orgID+"/settings", admin.session, "", "")
	boot := bootOf(t, settings.Body.String())
	if _, has := boot["countries"]; !has {
		t.Error("the organization settings page must carry the countries")
	}
	if org := boot["org"].(map[string]any); org["country"] != "BR" || org["ein"] != "" {
		t.Errorf("BOOT org = country %v, ein %v; want BR and an empty EIN", org["country"], org["ein"])
	}
}

// O cliente tem país (o da organização, se ninguém escolhe outro), e o documento dele é conferido por esse país.
func TestCustomers_CountryAndDocument(t *testing.T) {
	e := newServer(t)
	admin := signup(t, e, "Org", "ana@test.com")
	path := "/api/orgs/" + admin.orgID + "/customers"

	post := func(body string) map[string]any {
		t.Helper()
		rec := do(e, "POST", path, body, admin.session)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", body, rec.Code, rec.Body.String())
		}
		return decode(t, rec)
	}
	if c := post(`{"name":"Padrão","document":"11.222.333/0001-81"}`); c["country"] != "BR" || c["document"] != "11222333000181" {
		t.Errorf("default customer = %v", c)
	}
	if c := post(`{"name":"Acme Inc","country":"US","document":"12-3456789"}`); c["country"] != "US" || c["document"] != "123456789" {
		t.Errorf("US customer = %v", c)
	}
	if c := post(`{"name":"Beispiel","country":"de","document":"DE 123456789"}`); c["country"] != "DE" || c["document"] != "DE 123456789" {
		t.Errorf("DE customer = %v", c)
	}

	for body, code := range map[string]string{
		`{"name":"X","country":"US","document":"11.222.333/0001-81"}`: "customer.invalid_document",
		`{"name":"X","country":"ZZ"}`:                                 "customer.invalid_country",
	} {
		if rec := do(e, "POST", path, body, admin.session); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"`+code+`"`) {
			t.Errorf("%s = %d %s, want 400 %s", body, rec.Code, rec.Body.String(), code)
		}
	}
	if list := decodeList(t, do(e, "GET", path, "", admin.session)); len(list) != 3 {
		t.Errorf("customers = %d, want the 3 valid ones", len(list))
	}
}
