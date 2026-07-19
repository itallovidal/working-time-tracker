package service

import (
	"testing"
	"working-time-tracker/internal/store"
)

func TestPersonService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	person, err := svc.Create(org.ID.String(), "John", "john@test.com")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if person.Name != "John" {
		t.Errorf("name = %q, want %q", person.Name, "John")
	}
	if person.Email != "john@test.com" {
		t.Errorf("email = %q, want %q", person.Email, "john@test.com")
	}
}

func TestPersonService_Create_DuplicateEmail(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	svc.Create(org.ID.String(), "John", "john@test.com")
	_, err := svc.Create(org.ID.String(), "Jane", "john@test.com")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

func TestPersonService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "john@test.com")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestPersonService_Create_EmptyEmail(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "John", "")
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
}

func TestPersonService_ListByOrg(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

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

func TestPersonService_ListByOrg_SameEmailDifferentOrg(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewPersonService(store.NewPersonStore(testDB))

	orgA, _ := orgSvc.Create("Org A")
	orgB, _ := orgSvc.Create("Org B")

	svc.Create(orgA.ID.String(), "John", "john@test.com")
	_, err := svc.Create(orgB.ID.String(), "John", "john@test.com")
	if err != nil {
		t.Fatalf("same email in different org should be allowed: %v", err)
	}
}
