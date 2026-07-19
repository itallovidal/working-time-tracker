package integration_test

import (
	"testing"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
)

func cleanup(t *testing.T) {
	t.Helper()
	testDB.Exec("TRUNCATE TABLE integrations CASCADE")
	testDB.Exec("TRUNCATE TABLE projects CASCADE")
	testDB.Exec("TRUNCATE TABLE organizations CASCADE")
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	it, err := svc.Create(proj.ID.String(), "github", "My GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if it.DisplayName != "My GitHub" {
		t.Errorf("display_name = %q, want %q", it.DisplayName, "My GitHub")
	}
	if it.Config != nil {
		t.Error("expected config to be nil in response (credentials stripped)")
	}
}

func TestService_Create_InvalidType(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := svc.Create(proj.ID.String(), "invalid-type", "Bad", nil, true)
	if err == nil {
		t.Fatal("expected error for invalid integration type, got nil")
	}
}

func TestService_Get_NoCredentials(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(proj.ID.String(), "github", "My GitHub", map[string]interface{}{
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

func TestService_List_NoCredentials(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	svc.Create(proj.ID.String(), "github", "GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)

	list, err := svc.ListByProject(proj.ID.String())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	for _, item := range list {
		if item.Config != nil {
			t.Error("config should be nil in list response (credentials must never be returned)")
		}
	}
}

func TestService_Update(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(proj.ID.String(), "github", "Old Name", map[string]interface{}{
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

func TestService_Delete(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testDB))
	projSvc := project.NewService(project.NewStore(testDB))
	svc := integration.NewService(integration.NewStore(testDB), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	created, _ := svc.Create(proj.ID.String(), "github", "Test", map[string]interface{}{
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
