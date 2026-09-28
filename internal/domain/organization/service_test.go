package organization_test

import (
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

func TestService_Update(t *testing.T) {
	cleanup(t)
	svc := organization.NewService(organization.NewStore(testClient))

	created, _ := svc.Create("Old Name")
	updated, err := svc.Update(created.ID.String(), "New Name")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name")
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
