package service

import (
	"testing"
	"working-time-tracker/internal/store"
)

func TestOrganizationService_Create(t *testing.T) {
	cleanup(t)
	svc := NewOrganizationService(store.NewOrganizationStore(testDB))

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

func TestOrganizationService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	svc := NewOrganizationService(store.NewOrganizationStore(testDB))

	_, err := svc.Create("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestOrganizationService_List(t *testing.T) {
	cleanup(t)
	svc := NewOrganizationService(store.NewOrganizationStore(testDB))

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

func TestOrganizationService_Get(t *testing.T) {
	cleanup(t)
	svc := NewOrganizationService(store.NewOrganizationStore(testDB))

	created, _ := svc.Create("Test Org")
	got, err := svc.Get(created.ID.String())
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name != "Test Org" {
		t.Errorf("name = %q, want %q", got.Name, "Test Org")
	}
}

func TestOrganizationService_Update(t *testing.T) {
	cleanup(t)
	svc := NewOrganizationService(store.NewOrganizationStore(testDB))

	created, _ := svc.Create("Old Name")
	updated, err := svc.Update(created.ID.String(), "New Name")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name")
	}
}

func TestOrganizationService_Delete(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))

	org, _ := orgSvc.Create("Test Org")

	err := orgSvc.Delete(org.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = orgSvc.Get(org.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestOrganizationService_DeleteWithProjects_Rejected(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	err := orgSvc.Delete(org.ID.String())
	if err == nil {
		t.Fatal("expected error deleting org with active projects, got nil")
	}
}
