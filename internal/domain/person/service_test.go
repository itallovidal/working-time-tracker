package person_test

import (
	"context"
	"testing"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/testutil"
)

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	p, err := svc.Create(org.ID.String(), "John", "john@test.com")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if p.Name != "John" {
		t.Errorf("name = %q, want %q", p.Name, "John")
	}
	if p.Email != "john@test.com" {
		t.Errorf("email = %q, want %q", p.Email, "john@test.com")
	}
}

func TestService_Create_DuplicateEmail(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	svc.Create(org.ID.String(), "John", "john@test.com")
	_, err := svc.Create(org.ID.String(), "Jane", "john@test.com")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "john@test.com")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_EmptyEmail(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "John", "")
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

func TestService_ListByOrg(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	svc.Create(org.ID.String(), "John", "john@test.com")
	svc.Create(org.ID.String(), "Jane", "jane@test.com")

	persons, err := svc.ListByOrg(org.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(persons) != 2 {
		t.Errorf("got %d persons, want 2", len(persons))
	}
}

func TestService_ListByOrg_SameEmailDifferentOrg(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	orgA, _ := orgSvc.Create("Org A")
	orgB, _ := orgSvc.Create("Org B")

	svc.Create(orgA.ID.String(), "John", "john@test.com")
	_, err := svc.Create(orgB.ID.String(), "John", "john@test.com")
	if err != nil {
		t.Fatalf("same email in different org should be allowed: %v", err)
	}
}

// A org só pode ser excluída sem projetos. Quando isso acontece, as pessoas dela vão junto.
func TestOrganizationDelete_CascadesPersons(t *testing.T) {
	cleanup(t)
	ctx := context.Background()
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := person.NewService(person.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	other, _ := orgSvc.Create("Outra Org")
	svc.Create(org.ID.String(), "Ana", "ana@test.com")
	svc.Create(other.ID.String(), "Bia", "bia@test.com")

	if err := orgSvc.Delete(org.ID.String()); err != nil {
		t.Fatalf("delete organization with persons failed: %v", err)
	}
	if n := testClient.Person.Query().CountX(ctx); n != 1 {
		t.Errorf("persons: %d rows, want 1 (only the other organization's person)", n)
	}
}
