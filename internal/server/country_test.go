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
