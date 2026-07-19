package service

import (
	"testing"
	"working-time-tracker/internal/store"
)

func TestProjectService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, err := svc.Create(org.ID.String(), "Project A", "desc", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if project.Name != "Project A" {
		t.Errorf("name = %q, want %q", project.Name, "Project A")
	}
	if project.OrganizationID.String() != org.ID.String() {
		t.Errorf("org id mismatch")
	}
}

func TestProjectService_Create_EmptyName(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "desc", 0, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestProjectService_Create_DefaultSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, err := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if project.SprintDurationDays != 14 {
		t.Errorf("sprint_duration_days = %d, want 14", project.SprintDurationDays)
	}
}

func TestProjectService_Create_ExplicitSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, err := svc.Create(org.ID.String(), "Project A", "", 21, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if project.SprintDurationDays != 21 {
		t.Errorf("sprint_duration_days = %d, want 21", project.SprintDurationDays)
	}
}

func TestProjectService_Update(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

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

func TestProjectService_Delete(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	svc := NewProjectService(store.NewProjectStore(testDB))

	org, _ := orgSvc.Create("Test Org")
	project, _ := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	err := svc.Delete(project.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(project.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
