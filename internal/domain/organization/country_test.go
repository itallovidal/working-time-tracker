package organization_test

import (
	"testing"

	"working-time-tracker/internal/domain/organization"
)

func newOrg(t *testing.T) (*organization.Service, string) {
	t.Helper()
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	created, err := svc.Create("Acme")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, created.ID.String()
}

func TestService_Update_Country(t *testing.T) {
	svc, id := newOrg(t)
	if got, _ := svc.Get(id); got.Country != "BR" {
		t.Fatalf("a new organization starts in BR, got %q", got.Country)
	}

	// Em ordem fixa, terminando em US: o fim do teste confere que um país recusado não tira a organização de lá.
	for _, c := range []struct{ in, want string }{{"us", "US"}, {" EUA ", "US"}, {"Brasil", "BR"}, {"", "BR"}, {"US", "US"}} {
		if _, err := svc.Update(id, organization.UpdateInput{Country: ptr(c.in)}); err != nil {
			t.Fatalf("country %q: %v", c.in, err)
		}
		if got, _ := svc.Get(id); got.Country != c.want {
			t.Errorf("country %q saved as %q, want %q", c.in, got.Country, c.want)
		}
	}

	if _, err := svc.Update(id, organization.UpdateInput{Country: ptr("Portugal")}); err != organization.ErrInvalidCountry {
		t.Errorf("unknown country: err = %v, want ErrInvalidCountry", err)
	}
	// Um código que o cadastro não tem, mesmo sendo ISO, também é recusado: a organização é BR ou US.
	if _, err := svc.Update(id, organization.UpdateInput{Country: ptr("DE")}); err != organization.ErrInvalidCountry {
		t.Errorf("DE: err = %v, want ErrInvalidCountry", err)
	}
	if got, _ := svc.Get(id); got.Country != "US" {
		t.Errorf("a failed update changed the country to %q", got.Country)
	}
}

func TestService_Update_LegalIDs(t *testing.T) {
	svc, id := newOrg(t)

	// Cada documento é conferido pela regra do país dono dele, qualquer que seja o país da organização.
	if _, err := svc.Update(id, organization.UpdateInput{EIN: ptr("12-3456789")}); err != nil {
		t.Fatalf("EIN: %v", err)
	}
	if got, _ := svc.Get(id); got.EIN != "123456789" {
		t.Errorf("EIN = %q, want 123456789 (no mask)", got.EIN)
	}
	for name, c := range map[string]struct {
		in   organization.UpdateInput
		want error
	}{
		"EIN prefix 00":    {organization.UpdateInput{EIN: ptr("00-3456789")}, organization.ErrInvalidEIN},
		"EIN too short":    {organization.UpdateInput{EIN: ptr("12-34567")}, organization.ErrInvalidEIN},
		"a CNPJ as an EIN": {organization.UpdateInput{EIN: ptr("11.222.333/0001-81")}, organization.ErrInvalidEIN},
		"a bad CNPJ":       {organization.UpdateInput{CNPJ: ptr("11.222.333/0001-80")}, organization.ErrInvalidCNPJ},
		"an EIN as a CNPJ": {organization.UpdateInput{CNPJ: ptr("12-3456789")}, organization.ErrInvalidCNPJ},
	} {
		if _, err := svc.Update(id, c.in); err != c.want {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}

	// O país e o documento no mesmo pedido não dependem da ordem dos campos, e o documento do outro país fica guardado.
	if _, err := svc.Update(id, organization.UpdateInput{CNPJ: ptr("11.222.333/0001-81")}); err != nil {
		t.Fatalf("CNPJ: %v", err)
	}
	if _, err := svc.Update(id, organization.UpdateInput{Country: ptr("US"), EIN: ptr("98-7654321")}); err != nil {
		t.Fatalf("switch to US with an EIN: %v", err)
	}
	got, _ := svc.Get(id)
	if got.Country != "US" || got.EIN != "987654321" || got.CNPJ != "11222333000181" {
		t.Errorf("after BR→US: country=%q ein=%q cnpj=%q; want US 987654321 and the CNPJ kept", got.Country, got.EIN, got.CNPJ)
	}

	// Vazio apaga só o documento pedido.
	if _, err := svc.Update(id, organization.UpdateInput{EIN: ptr("")}); err != nil {
		t.Fatalf("clearing the EIN: %v", err)
	}
	if got, _ := svc.Get(id); got.EIN != "" || got.CNPJ != "11222333000181" {
		t.Errorf("ein=%q cnpj=%q, want the EIN cleared and the CNPJ kept", got.EIN, got.CNPJ)
	}
}

// Fuso e moeda vazios voltam para o padrão do país; trocar o país não os muda sozinho.
func TestService_Update_TimezoneAndCurrencyDefaultsFollowTheCountry(t *testing.T) {
	svc, id := newOrg(t)
	if _, err := svc.Update(id, organization.UpdateInput{Timezone: ptr("America/Recife"), Currency: ptr("EUR")}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.Update(id, organization.UpdateInput{Country: ptr("US")}); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if got, _ := svc.Get(id); got.Timezone != "America/Recife" || got.Currency != "EUR" {
		t.Errorf("switching the country changed timezone/currency to %q / %q", got.Timezone, got.Currency)
	}
	if _, err := svc.Update(id, organization.UpdateInput{Timezone: ptr(""), Currency: ptr("")}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := svc.Get(id); got.Timezone != "America/New_York" || got.Currency != "USD" {
		t.Errorf("empty timezone/currency in a US org = %q / %q, want America/New_York / USD", got.Timezone, got.Currency)
	}
	// Mandar o país junto também vale: o padrão é o do país novo.
	if _, err := svc.Update(id, organization.UpdateInput{Country: ptr("BR"), Timezone: ptr(""), Currency: ptr("")}); err != nil {
		t.Fatalf("back to BR: %v", err)
	}
	if got, _ := svc.Get(id); got.Timezone != "America/Sao_Paulo" || got.Currency != "BRL" {
		t.Errorf("BR defaults = %q / %q, want America/Sao_Paulo / BRL", got.Timezone, got.Currency)
	}
}
