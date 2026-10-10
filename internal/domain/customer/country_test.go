package customer_test

import (
	"errors"
	"testing"

	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
)

// O país do cliente novo é o da organização, a menos que venha outro; o documento é conferido pela regra do país do
// cliente: CNPJ no Brasil, EIN nos EUA, e texto livre nos outros.
func TestService_Country(t *testing.T) {
	svc, _, orgID := setup(t)

	brazilian, err := svc.Create(orgID, customer.Input{Name: ptr("Empresa BR"), Document: ptr("11.222.333/0001-81")})
	if err != nil {
		t.Fatalf("create in the default country: %v", err)
	}
	if brazilian.Country != "BR" || brazilian.Document != "11222333000181" {
		t.Errorf("default customer = %q %q, want BR and the CNPJ unmasked", brazilian.Country, brazilian.Document)
	}

	american, err := svc.Create(orgID, customer.Input{Name: ptr("Acme Inc"), Country: ptr("us"), Document: ptr("12-3456789")})
	if err != nil {
		t.Fatalf("create a US customer: %v", err)
	}
	if american.Country != "US" || american.Document != "123456789" {
		t.Errorf("US customer = %q %q, want US and the EIN unmasked", american.Country, american.Document)
	}

	german, err := svc.Create(orgID, customer.Input{Name: ptr("Beispiel GmbH"), Country: ptr("DE"), Document: ptr("  DE   123456789 ")})
	if err != nil {
		t.Fatalf("create a customer in a country with no rule: %v", err)
	}
	if german.Country != "DE" || german.Document != "DE 123456789" {
		t.Errorf("DE customer = %q %q, want DE and a free-text tax id with the spaces collapsed", german.Country, german.Document)
	}

	for name, c := range map[string]struct {
		in   customer.Input
		want error
	}{
		"a CNPJ for a US customer": {customer.Input{Name: ptr("X"), Country: ptr("US"), Document: ptr("11.222.333/0001-81")}, customer.ErrInvalidDocument},
		"an EIN for a BR customer": {customer.Input{Name: ptr("X"), Document: ptr("12-3456789")}, customer.ErrInvalidDocument},
		"a bad free-text tax id":   {customer.Input{Name: ptr("X"), Country: ptr("DE"), Document: ptr("DE<1>")}, customer.ErrInvalidDocument},
		"an unknown country":       {customer.Input{Name: ptr("X"), Country: ptr("ZZ")}, customer.ErrInvalidCountry},
		"a country name, not code": {customer.Input{Name: ptr("X"), Country: ptr("Brasil")}, customer.ErrInvalidCountry},
	} {
		if _, err := svc.Create(orgID, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}

	// O país de uma organização dos EUA é o padrão dos clientes dela.
	orgs := organization.NewService(organization.NewStore(testClient))
	if _, err := orgs.Update(orgID, organization.UpdateInput{Country: ptr("US")}); err != nil {
		t.Fatalf("switch the organization to the US: %v", err)
	}
	inUS, err := svc.Create(orgID, customer.Input{Name: ptr("Local Co"), Country: ptr("")})
	if err != nil {
		t.Fatalf("create with an empty country: %v", err)
	}
	if inUS.Country != "US" {
		t.Errorf("customer of a US organization = %q, want US", inUS.Country)
	}
}

// Trocar o país de um cliente confere o documento que já está guardado pelo país novo, e recusa se não serve: nada é
// apagado em silêncio. Mandar o documento (ou vazio) no mesmo pedido resolve.
func TestService_Update_ChangingTheCountryChecksTheStoredDocument(t *testing.T) {
	svc, _, orgID := setup(t)
	c, err := svc.Create(orgID, customer.Input{Name: ptr("Empresa"), Document: ptr("11.222.333/0001-81")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()

	if _, err := svc.Update(id, customer.Input{Country: ptr("US")}); !errors.Is(err, customer.ErrInvalidDocument) {
		t.Fatalf("a CNPJ under a US customer: err = %v, want ErrInvalidDocument", err)
	}
	if got, _ := svc.Get(id); got.Country != "BR" || got.Document != "11222333000181" {
		t.Errorf("a refused change altered the customer: %+v", got)
	}

	// Um país sem regra aceita o documento que já estava (é texto livre), e a troca com o documento novo também vale.
	if _, err := svc.Update(id, customer.Input{Country: ptr("DE")}); err != nil {
		t.Fatalf("a CNPJ under a customer with no rule: %v", err)
	}
	if got, _ := svc.Get(id); got.Country != "DE" || got.Document != "11222333000181" {
		t.Errorf("after BR→DE = %q %q, want DE and the same document", got.Country, got.Document)
	}
	if _, err := svc.Update(id, customer.Input{Country: ptr("US"), Document: ptr("98-7654321")}); err != nil {
		t.Fatalf("switch to US with an EIN: %v", err)
	}
	if got, _ := svc.Get(id); got.Country != "US" || got.Document != "987654321" {
		t.Errorf("after DE→US = %q %q, want US and the EIN", got.Country, got.Document)
	}
	if _, err := svc.Update(id, customer.Input{Country: ptr("BR"), Document: ptr("")}); err != nil {
		t.Fatalf("switch to BR clearing the document: %v", err)
	}
	if got, _ := svc.Get(id); got.Country != "BR" || got.Document != "" {
		t.Errorf("after US→BR = %q %q, want BR and no document", got.Country, got.Document)
	}

	// O país nunca fica sem valor, e vazio vale o mesmo na criação e na edição: o país da organização.
	if _, err := svc.Update(id, customer.Input{Country: ptr("DE")}); err != nil {
		t.Fatalf("switch to DE: %v", err)
	}
	orgs := organization.NewService(organization.NewStore(testClient))
	if _, err := orgs.Update(orgID, organization.UpdateInput{Country: ptr("US")}); err != nil {
		t.Fatalf("switch the organization to the US: %v", err)
	}
	if _, err := svc.Update(id, customer.Input{Country: ptr("  ")}); err != nil {
		t.Fatalf("an empty country on update: %v", err)
	}
	if got, _ := svc.Get(id); got.Country != "US" {
		t.Errorf("an empty country on update = %q, want the organization's (US)", got.Country)
	}
}
