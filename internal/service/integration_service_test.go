package service

import (
	"testing"
	"working-time-tracker/internal/store"
)

func TestIntegrationService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	integration, err := svc.Create(project.ID.String(), "github", "My GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if integration.DisplayName != "My GitHub" {
		t.Errorf("display_name = %q, want %q", integration.DisplayName, "My GitHub")
	}
	if integration.Config != nil {
		t.Error("expected config to be nil in response (credentials stripped)")
	}
}

func TestIntegrationService_Create_InvalidType(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := svc.Create(project.ID.String(), "invalid-type", "Bad", nil, true)
	if err == nil {
		t.Fatal("expected error for invalid integration type, got nil")
	}
}

func TestIntegrationService_Get_NoCredentials(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(project.ID.String(), "github", "My GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)

	got, err := svc.Get(created.ID.String())
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Config != nil {
		t.Error("config should be nil in get response (credentials must never be returned)")
	}
}

func TestIntegrationService_List_NoCredentials(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	svc.Create(project.ID.String(), "github", "GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)

	list, err := svc.ListByProject(project.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	for _, item := range list {
		if item.Config != nil {
			t.Error("config should be nil in list response (credentials must never be returned)")
		}
	}
}

func TestIntegrationService_Update(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(project.ID.String(), "github", "Old Name", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)

	enabled := true
	updated, err := svc.Update(created.ID.String(), "New Name", nil, &enabled)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.DisplayName != "New Name" {
		t.Errorf("display_name = %q, want %q", updated.DisplayName, "New Name")
	}
}

func TestIntegrationService_Delete(t *testing.T) {
	cleanup(t)
	orgSvc := NewOrganizationService(store.NewOrganizationStore(testDB))
	projSvc := NewProjectService(store.NewProjectStore(testDB))
	svc := NewIntegrationService(store.NewIntegrationStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	project, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(project.ID.String(), "github", "Test", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)

	err := svc.Delete(created.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(created.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}
