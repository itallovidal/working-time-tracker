package organization_test

import (
	"strings"
	"testing"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/testutil"
)

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	org, err := svc.Create("Test Org")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if org.Name != "Test Org" {
		t.Errorf("name = %q, want %q", org.Name, "Test Org")
	}
	if org.ID.String() == "" {
		t.Error("expected non-empty ID")
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	_, err := svc.Create("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_List(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	svc.Create("Org A")
	svc.Create("Org B")

	orgs, err := svc.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(orgs) != 2 {
		t.Errorf("got %d orgs, want 2", len(orgs))
	}
}

func TestService_Get(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	created, _ := svc.Create("Test Org")
	got, err := svc.Get(created.ID.String())
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name != "Test Org" {
		t.Errorf("name = %q, want %q", got.Name, "Test Org")
	}
}

func ptr[T any](v T) *T { return &v }

func TestService_Update(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	created, _ := svc.Create("Old Name")
	updated, err := svc.Update(created.ID.String(), organization.UpdateInput{Name: ptr("  New Name ")})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name")
	}
	if _, err := svc.Update(created.ID.String(), organization.UpdateInput{Name: ptr("   ")}); err != organization.ErrNameRequired {
		t.Errorf("blank name: err = %v, want ErrNameRequired", err)
	}
}

// Uma organização nova já nasce com o fuso e a moeda padrão.
func TestService_Create_Defaults(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	org, err := svc.Create("Org")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if org.Timezone != organization.DefaultTimezone || org.Currency != organization.DefaultCurrency {
		t.Errorf("defaults = %q, %q; want %q, %q", org.Timezone, org.Currency, organization.DefaultTimezone, organization.DefaultCurrency)
	}
	if org.WorkMode != "" || org.Summary != "" {
		t.Errorf("optional fields should start empty: %+v", org)
	}
}

func TestService_Update_Profile(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	created, _ := svc.Create("Acme")
	id := created.ID.String()

	_, err := svc.Update(id, organization.UpdateInput{
		Summary:      ptr("Entregas no mesmo dia\n  para lojas de bairro."),
		Description:  ptr("  Primeira linha.\nSegunda linha.  "),
		Website:      ptr("acme.com.br"),
		ContactEmail: ptr(" Contato@Acme.com.br "),
		LinkedinURL:  ptr("https://www.linkedin.com/company/acme"),
		LegalName:    ptr("Acme Entregas Ltda"),
		CNPJ:         ptr("11.222.333/0001-81"),
		Country:      ptr("Brasil"),
		WorkMode:     ptr(" Hybrid "),
		Timezone:     ptr("America/Recife"),
		Currency:     ptr("usd"),
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Relê do banco para conferir o que foi gravado.
	got, err := svc.Get(id)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name != "Acme" {
		t.Errorf("name changed to %q without being sent", got.Name)
	}
	for field, pair := range map[string][2]string{
		"summary":       {got.Summary, "Entregas no mesmo dia para lojas de bairro."},
		"description":   {got.Description, "Primeira linha.\nSegunda linha."},
		"website":       {got.Website, "https://acme.com.br"},
		"contact_email": {got.ContactEmail, "contato@acme.com.br"},
		"linkedin_url":  {got.LinkedinURL, "https://www.linkedin.com/company/acme"},
		"legal_name":    {got.LegalName, "Acme Entregas Ltda"},
		"cnpj":          {got.CNPJ, "11222333000181"},
		"work_mode":     {got.WorkMode, "hybrid"},
		"timezone":      {got.Timezone, "America/Recife"},
		"currency":      {got.Currency, "USD"},
		// "Brasil" é uma grafia antiga: o que fica guardado é o código.
		"country": {got.Country, "BR"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}

	// Texto vazio e zero apagam; o que não vem continua como estava.
	if _, err := svc.Update(id, organization.UpdateInput{
		Summary: ptr(""), CNPJ: ptr(""), Website: ptr(""), WorkMode: ptr(""),
		Timezone: ptr(""), Currency: ptr(""),
	}); err != nil {
		t.Fatalf("clearing failed: %v", err)
	}
	got, _ = svc.Get(id)
	if got.Summary != "" || got.CNPJ != "" || got.Website != "" || got.WorkMode != "" {
		t.Errorf("fields were not cleared: %+v", got)
	}
	if got.Timezone != organization.DefaultTimezone || got.Currency != organization.DefaultCurrency {
		t.Errorf("empty timezone and currency should go back to the defaults, got %q and %q", got.Timezone, got.Currency)
	}
	if got.Description != "Primeira linha.\nSegunda linha." || got.LegalName != "Acme Entregas Ltda" {
		t.Errorf("fields that were not sent changed: %+v", got)
	}
}

func TestService_Update_Validation(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))
	created, _ := svc.Create("Acme")
	id := created.ID.String()

	cases := []struct {
		name string
		in   organization.UpdateInput
		want error
	}{
		{"website scheme", organization.UpdateInput{Website: ptr("javascript:alert(1)")}, organization.ErrInvalidURL},
		{"linkedin", organization.UpdateInput{LinkedinURL: ptr("ftp://linkedin.com/acme")}, organization.ErrInvalidURL},
		{"email", organization.UpdateInput{ContactEmail: ptr("contato")}, organization.ErrInvalidEmail},
		{"cnpj", organization.UpdateInput{CNPJ: ptr("11.222.333/0001-80")}, organization.ErrInvalidCNPJ},
		{"timezone", organization.UpdateInput{Timezone: ptr("Marte/Olympus")}, organization.ErrInvalidTimezone},
		{"timezone local", organization.UpdateInput{Timezone: ptr("Local")}, organization.ErrInvalidTimezone},
		{"work mode", organization.UpdateInput{WorkMode: ptr("nômade")}, organization.ErrInvalidWorkMode},
		{"currency", organization.UpdateInput{Currency: ptr("BTC")}, organization.ErrInvalidCurrency},
	}
	for _, c := range cases {
		if _, err := svc.Update(id, c.in); err != c.want {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}

	if _, err := svc.Update(id, organization.UpdateInput{Summary: ptr(strings.Repeat("a", 161))}); err == nil {
		t.Error("summary with 161 characters was accepted")
	}
	// O limite é em caracteres, não em bytes.
	if _, err := svc.Update(id, organization.UpdateInput{Summary: ptr(strings.Repeat("ã", 160))}); err != nil {
		t.Errorf("summary with 160 accented characters was refused: %v", err)
	}
	// Uma validação que falha não grava os outros campos do mesmo pedido.
	if _, err := svc.Update(id, organization.UpdateInput{LegalName: ptr("Varejo Ltda"), Currency: ptr("BTC")}); err == nil {
		t.Fatal("expected an error")
	}
	if got, _ := svc.Get(id); got.LegalName != "" {
		t.Errorf("legal name = %q after a failed update, want it unchanged", got.LegalName)
	}
}

func TestService_Delete(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	org, _ := svc.Create("Test Org")

	err := svc.Delete(org.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(org.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
