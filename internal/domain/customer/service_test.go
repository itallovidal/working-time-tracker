package customer_test

import (
	"errors"
	"strings"
	"testing"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/testutil"
)

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) (*customer.Service, *project.Service, string) {
	t.Helper()
	testutil.Truncate(t, testDB)
	org, err := organization.NewService(organization.NewStore(testClient)).Create("Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	return customer.NewService(customer.NewStore(testClient)), project.NewService(project.NewStore(testClient)), org.ID.String()
}

func TestService_CreateAndList(t *testing.T) {
	svc, _, orgID := setup(t)

	created, err := svc.Create(orgID, customer.Input{
		Name:         ptr("  Empresa A "),
		Document:     ptr("11.222.333/0001-81"),
		ContactName:  ptr("Carla Dias"),
		ContactEmail: ptr(" Carla@EmpresaA.com "),
		ContactPhone: ptr("(11) 4002-8922"),
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.Name != "Empresa A" || created.Document != "11222333000181" || created.ContactEmail != "carla@empresaa.com" {
		t.Errorf("created = %+v", created)
	}
	if _, err := svc.Create(orgID, customer.Input{Name: ptr("Armazém Zeta")}); err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := svc.ListByOrg(orgID)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 2 || list[0].Name != "Armazém Zeta" || list[1].Name != "Empresa A" {
		t.Errorf("list should come in alphabetical order, got %+v", list)
	}
}

func TestService_Validation(t *testing.T) {
	svc, _, orgID := setup(t)

	cases := []struct {
		name string
		in   customer.Input
		want error
	}{
		{"no name", customer.Input{}, customer.ErrNameRequired},
		{"blank name", customer.Input{Name: ptr("  ")}, customer.ErrNameRequired},
		{"document", customer.Input{Name: ptr("X"), Document: ptr("11.222.333/0001-80")}, customer.ErrInvalidDocument},
		{"email", customer.Input{Name: ptr("X"), ContactEmail: ptr("carla")}, apperr.ErrFieldInvalid},
		{"phone", customer.Input{Name: ptr("X"), ContactPhone: ptr("ligar")}, customer.ErrInvalidPhone},
	}
	for _, c := range cases {
		if _, err := svc.Create(orgID, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// fieldOf devolve o parâmetro field do erro, ou "" quando não há.
func fieldOf(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		s, _ := e.Params["field"].(string)
		return s
	}
	return ""
}

// Cada campo do cliente: vazio, só espaços, no teto, teto+1, formato e caixa, com o campo que o erro devolve.
func TestService_FieldRules(t *testing.T) {
	svc, _, orgID := setup(t)
	rep := func(s string, n int) string { return strings.Repeat(s, n) }
	longEmail := rep("a", 244) + "@ex.com" // 251 caracteres
	maxEmail := rep("a", 248) + "@ex.com"  // 255
	overEmail := rep("a", 249) + "@ex.com" // 256

	cases := []struct {
		name  string
		in    customer.Input
		want  error // nil: aceito
		field string
	}{
		{"name empty", customer.Input{Name: ptr("")}, customer.ErrNameRequired, "name"},
		{"name spaces", customer.Input{Name: ptr("   ")}, customer.ErrNameRequired, "name"},
		{"name at limit", customer.Input{Name: ptr(rep("n", 120))}, nil, ""},
		{"name at limit with outer spaces", customer.Input{Name: ptr(" " + rep("n", 120) + " ")}, nil, ""},
		{"name over limit", customer.Input{Name: ptr(rep("n", 121))}, customer.ErrNameTooLong, "name"},
		{"contact name at limit", customer.Input{Name: ptr("X"), ContactName: ptr(rep("c", 120))}, nil, ""},
		{"contact name over limit", customer.Input{Name: ptr("X"), ContactName: ptr(rep("c", 121))}, customer.ErrContactTooLong, "contact_name"},
		{"email blank is empty", customer.Input{Name: ptr("X"), ContactEmail: ptr("  ")}, nil, ""},
		{"email upper case", customer.Input{Name: ptr("X"), ContactEmail: ptr("CARLA@Empresa.COM")}, nil, ""},
		{"email without tld", customer.Input{Name: ptr("X"), ContactEmail: ptr("carla@empresa")}, apperr.ErrFieldInvalid, "contact_email"},
		{"email with space inside", customer.Input{Name: ptr("X"), ContactEmail: ptr("car la@empresa.com")}, apperr.ErrFieldInvalid, "contact_email"},
		{"email below limit", customer.Input{Name: ptr("X"), ContactEmail: ptr(longEmail)}, nil, ""},
		{"email at limit", customer.Input{Name: ptr("X"), ContactEmail: ptr(maxEmail)}, nil, ""},
		{"email over limit", customer.Input{Name: ptr("X"), ContactEmail: ptr(overEmail)}, apperr.ErrFieldTooLong, "contact_email"},
		{"phone with 7 digits", customer.Input{Name: ptr("X"), ContactPhone: ptr("123 4567")}, nil, ""},
		{"phone with 6 digits", customer.Input{Name: ptr("X"), ContactPhone: ptr("123 456 ")}, customer.ErrInvalidPhone, "contact_phone"},
		{"phone only punctuation", customer.Input{Name: ptr("X"), ContactPhone: ptr("--------")}, customer.ErrInvalidPhone, "contact_phone"},
		{"phone over limit", customer.Input{Name: ptr("X"), ContactPhone: ptr(rep("1", 33))}, customer.ErrInvalidPhone, "contact_phone"},
		{"phone at limit", customer.Input{Name: ptr("X"), ContactPhone: ptr(rep("1", 32))}, nil, ""},
		{"document", customer.Input{Name: ptr("X"), Document: ptr("11.222.333/0001-80")}, customer.ErrInvalidDocument, "document"},
		{"country", customer.Input{Name: ptr("X"), Country: ptr("ZZZ")}, customer.ErrInvalidCountry, "country"},
	}
	for _, c := range cases {
		_, err := svc.Create(orgID, c.in)
		if c.want == nil {
			if err != nil {
				t.Errorf("%s: err = %v, want none", c.name, err)
			}
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
			continue
		}
		if got := fieldOf(err); got != c.field {
			t.Errorf("%s: field = %q, want %q", c.name, got, c.field)
		}
	}

	got, err := svc.Create(orgID, customer.Input{Name: ptr("Caixa"), ContactEmail: ptr(" CARLA@Empresa.COM ")})
	if err != nil || got.ContactEmail != "carla@empresa.com" {
		t.Errorf("email should be trimmed and lower-cased: %+v, %v", got, err)
	}
}

func TestService_Update(t *testing.T) {
	svc, _, orgID := setup(t)
	created, _ := svc.Create(orgID, customer.Input{Name: ptr("Empresa A"), ContactName: ptr("Carla"), Document: ptr("11222333000181")})
	id := created.ID.String()

	// Só o que vem é alterado; texto vazio apaga.
	updated, err := svc.Update(id, customer.Input{Name: ptr("Empresa Alfa"), Document: ptr("")})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "Empresa Alfa" || updated.Document != "" || updated.ContactName != "Carla" {
		t.Errorf("updated = %+v", updated)
	}
	if _, err := svc.Update(id, customer.Input{Name: ptr("")}); !errors.Is(err, customer.ErrNameRequired) {
		t.Errorf("empty name: err = %v, want ErrNameRequired", err)
	}
	if _, err := svc.Update("00000000-0000-0000-0000-000000000000", customer.Input{Name: ptr("X")}); err != database.ErrNotFound {
		t.Errorf("missing customer: err = %v, want ErrNotFound", err)
	}
}

// Um cliente com projetos não é excluído, para nenhum projeto perder o cliente
// sem alguém decidir.
func TestService_DeleteRefusedWithProjects(t *testing.T) {
	svc, projSvc, orgID := setup(t)
	created, _ := svc.Create(orgID, customer.Input{Name: ptr("Empresa A")})
	id := created.ID.String()
	proj, _ := projSvc.Create(orgID, "Projeto X", "", 0, project.Routine{})
	if _, err := projSvc.SetBilling(proj.ID.String(), &id, ptr(10000)); err != nil {
		t.Fatalf("set billing: %v", err)
	}

	got, _ := svc.Get(id)
	if got.ProjectCount != 1 {
		t.Errorf("project_count = %d, want 1", got.ProjectCount)
	}
	if err := svc.Delete(id); !errors.Is(err, customer.ErrHasProjects) {
		t.Fatalf("delete with projects: err = %v, want ErrHasProjects", err)
	}

	if _, err := projSvc.SetBilling(proj.ID.String(), nil, nil); err != nil {
		t.Fatalf("clear billing: %v", err)
	}
	if err := svc.Delete(id); err != nil {
		t.Fatalf("delete without projects: %v", err)
	}
	if _, err := svc.Get(id); err != database.ErrNotFound {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
}
