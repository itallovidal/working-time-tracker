package customer_test

import (
	"testing"

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
		{"email", customer.Input{Name: ptr("X"), ContactEmail: ptr("carla")}, customer.ErrInvalidEmail},
		{"phone", customer.Input{Name: ptr("X"), ContactPhone: ptr("ligar")}, customer.ErrInvalidPhone},
	}
	for _, c := range cases {
		if _, err := svc.Create(orgID, c.in); err != c.want {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
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
	if _, err := svc.Update(id, customer.Input{Name: ptr("")}); err != customer.ErrNameRequired {
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
	proj, _ := projSvc.Create(orgID, "Projeto X", "", 0, nil, nil, nil)
	if _, err := projSvc.SetBilling(proj.ID.String(), &id, ptr(10000)); err != nil {
		t.Fatalf("set billing: %v", err)
	}

	got, _ := svc.Get(id)
	if got.ProjectCount != 1 {
		t.Errorf("project_count = %d, want 1", got.ProjectCount)
	}
	if err := svc.Delete(id); err != customer.ErrHasProjects {
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
