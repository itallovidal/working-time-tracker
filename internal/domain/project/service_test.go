package project_test

import (
	"testing"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
)

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "desc", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.Name != "Project A" {
		t.Errorf("name = %q, want %q", proj.Name, "Project A")
	}
	if proj.OrganizationID.String() != org.ID.String() {
		t.Errorf("org id mismatch")
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "desc", 0, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_DefaultSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.SprintDurationDays != 14 {
		t.Errorf("sprint_duration_days = %d, want 14", proj.SprintDurationDays)
	}
}

func TestService_Create_ExplicitSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 21, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.SprintDurationDays != 21 {
		t.Errorf("sprint_duration_days = %d, want 21", proj.SprintDurationDays)
	}
}

func TestService_Update(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	created, _ := svc.Create(org.ID.String(), "Old Name", "", 0, nil, nil)
	updated, err := svc.Update(created.ID.String(), "New Name", "new desc", 10, nil, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %q, want %q", updated.Name, "New Name")
	}
	if updated.Description != "new desc" {
		t.Errorf("description = %q, want %q", updated.Description, "new desc")
	}
	if updated.SprintDurationDays != 10 {
		t.Errorf("sprint_duration_days = %d, want 10", updated.SprintDurationDays)
	}
}

func TestService_Delete(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	svc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	proj, _ := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	err := svc.Delete(proj.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(proj.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

// TestService_OrgDeletionBlockedByProjects valida que organization.Service.Delete
// rejeita a deleção quando há projetos ativos. Movido para cá para evitar
// dependência cíclica (organization não pode importar project).
func TestService_OrgDeletionBlockedByProjects(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	err := orgSvc.Delete(org.ID.String())
	if err == nil {
		t.Fatal("expected error deleting org with active projects, got nil")
	}
}
