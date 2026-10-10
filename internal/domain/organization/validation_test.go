package organization_test

import (
	"errors"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/organization"
)

func str(s string) *string { return &s }

// wantField confere o código do erro e o parâmetro field (a chave do campo no corpo da requisição).
func wantField(t *testing.T, name string, err error, code *apperr.Error, field string, max any) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || !errors.Is(err, code) {
		t.Errorf("%s: err = %v, want %s", name, err, code.Code)
		return
	}
	if e.Params["field"] != field {
		t.Errorf("%s: field = %v, want %q", name, e.Params["field"], field)
	}
	if max != nil && e.Params["max"] != max {
		t.Errorf("%s: max = %v, want %v", name, e.Params["max"], max)
	}
}

func TestService_Update_TextFieldRules(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	org, _ := svc.Create("Org")
	id := org.ID.String()

	cases := []struct {
		name  string
		in    organization.UpdateInput
		field string
		max   int
	}{
		{"nome", organization.UpdateInput{Name: str(strings.Repeat("n", 121))}, "name", 120},
		{"resumo", organization.UpdateInput{Summary: str(strings.Repeat("r", 161))}, "summary", 160},
		{"descrição", organization.UpdateInput{Description: str(strings.Repeat("d", 2001))}, "long_description", 2000},
		{"razão social", organization.UpdateInput{LegalName: str(strings.Repeat("l", 201))}, "legal_name", 200},
		{"endereço", organization.UpdateInput{AddressLine1: str(strings.Repeat("a", 201))}, "address_line1", 200},
		{"complemento", organization.UpdateInput{AddressLine2: str(strings.Repeat("c", 201))}, "address_line2", 200},
	}
	for _, tc := range cases {
		_, err := svc.Update(id, tc.in)
		wantField(t, tc.name+" acima do teto", err, organization.ErrFieldTooLong, tc.field, tc.max)
	}

	// No teto (em acentos: conta caracteres, e não bytes) e com espaços nas pontas passa, aparado.
	got, err := svc.Update(id, organization.UpdateInput{
		Name:        str("  " + strings.Repeat("ã", 120) + "  "),
		Summary:     str(strings.Repeat("r", 160)),
		Description: str(strings.Repeat("d", 2000)),
		LegalName:   str(" " + strings.Repeat("l", 200) + " "),
	})
	if err != nil {
		t.Fatalf("update at the limits: %v", err)
	}
	if got.Name != strings.Repeat("ã", 120) || got.LegalName != strings.Repeat("l", 200) {
		t.Errorf("name / legal name = %q / %q, want trimmed", got.Name, got.LegalName)
	}

	// Vazio e só espaços: o nome é obrigatório, o resto apaga.
	_, err = svc.Update(id, organization.UpdateInput{Name: str("   ")})
	wantField(t, "nome só com espaços", err, organization.ErrNameRequired, "name", nil)
	got, err = svc.Update(id, organization.UpdateInput{Summary: str("  "), Description: str("   ")})
	if err != nil || got.Summary != "" || got.Description != "" {
		t.Errorf("blank summary/description = %+v, %v; want cleared", got, err)
	}
}

func TestService_Update_LinkAndEmailFields(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	org, _ := svc.Create("Org")
	id := org.ID.String()
	long := "https://" + strings.Repeat("a", 250) + ".com"

	cases := []struct {
		name  string
		in    organization.UpdateInput
		code  *apperr.Error
		field string
	}{
		{"site sem ponto no host", organization.UpdateInput{Website: str("https://localhost")}, organization.ErrInvalidURL, "website"},
		{"site com espaço", organization.UpdateInput{Website: str("https://a b.com")}, organization.ErrInvalidURL, "website"},
		{"site acima do teto", organization.UpdateInput{Website: str(long)}, organization.ErrInvalidURL, "website"},
		{"site de outro esquema", organization.UpdateInput{Website: str("ftp://exemplo.com")}, organization.ErrInvalidURL, "website"},
		{"LinkedIn inválido", organization.UpdateInput{LinkedinURL: str("não é um link")}, organization.ErrInvalidURL, "linkedin_url"},
		{"LinkedIn acima do teto", organization.UpdateInput{LinkedinURL: str(long)}, organization.ErrInvalidURL, "linkedin_url"},
		{"e-mail sem domínio", organization.UpdateInput{ContactEmail: str("contato@acme")}, organization.ErrInvalidEmail, "contact_email"},
		{"e-mail com espaço", organization.UpdateInput{ContactEmail: str("con tato@acme.com")}, organization.ErrInvalidEmail, "contact_email"},
		{"e-mail acima do teto", organization.UpdateInput{ContactEmail: str(strings.Repeat("a", 251) + "@b.co")}, organization.ErrInvalidEmail, "contact_email"},
		{"CNPJ inválido", organization.UpdateInput{CNPJ: str("11.222.333/0001-80")}, organization.ErrInvalidCNPJ, "cnpj"},
		{"EIN inválido", organization.UpdateInput{EIN: str("123")}, organization.ErrInvalidEIN, "ein"},
		{"país desconhecido", organization.UpdateInput{Country: str("Atlantida")}, organization.ErrInvalidCountry, "country"},
		{"regime desconhecido", organization.UpdateInput{WorkMode: str("nuvem")}, organization.ErrInvalidWorkMode, "work_mode"},
		{"fuso desconhecido", organization.UpdateInput{Timezone: str("Marte/Olympus")}, organization.ErrInvalidTimezone, "timezone"},
		{"moeda desconhecida", organization.UpdateInput{Currency: str("XYZ")}, organization.ErrInvalidCurrency, "currency"},
	}
	for _, tc := range cases {
		_, err := svc.Update(id, tc.in)
		wantField(t, tc.name, err, tc.code, tc.field, nil)
	}

	got, err := svc.Update(id, organization.UpdateInput{
		Website:      str("  exemplo.com.br  "),
		LinkedinURL:  str("https://www.linkedin.com/company/acme"),
		ContactEmail: str("  CONTATO@Acme.com "),
	})
	if err != nil {
		t.Fatalf("valid links and email: %v", err)
	}
	if got.Website != "https://exemplo.com.br" || got.ContactEmail != "contato@acme.com" {
		t.Errorf("website / email = %q / %q, want https assumed and lowercase", got.Website, got.ContactEmail)
	}
}
