package integration_test

import (
	"testing"

	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/testutil"
)

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	_, err := svc.Create(proj.ID.String(), "invalid-type", "Bad", nil, true)
	if err == nil {
		t.Fatal("expected error for invalid integration type, got nil")
	}
}

func TestService_Get_NoCredentials(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	created, err := svc.Create(proj.ID.String(), "github", "My GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	created, err := svc.Create(proj.ID.String(), "github", "Old Name", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	created, err := svc.Create(proj.ID.String(), "github", "Test", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	err = svc.Delete(created.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = svc.Get(created.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_Create_InvalidToken(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	_, err := svc.Create(proj.ID.String(), "github", "GitHub", map[string]interface{}{
		"token": invalidGitHubToken,
		"repo":  "owner/repo",
	}, true)
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
}

func TestService_FetchItemDetails(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	created, err := svc.Create(proj.ID.String(), "github", "GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, true)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	found, err := svc.FetchItemDetails(created.ID.String(), "42")
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if found.Details == nil || found.Details.Title != "Corrigir login" || found.Details.State != "open" {
		t.Errorf("details = %+v, want title %q and state %q", found.Details, "Corrigir login", "open")
	}

	// Item inexistente: a chamada não falha, só devolve details nulo com a mensagem de erro.
	missing, err := svc.FetchItemDetails(created.ID.String(), "999")
	if err != nil {
		t.Fatalf("fetch of missing item should degrade gracefully, got error: %v", err)
	}
	if missing.Details != nil || missing.Error == nil {
		t.Errorf("expected nil details and an error message, got %+v", missing)
	}
}

func TestService_HasConfigAndDisabledFetch(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	svc := integration.NewService(integration.NewStore(testClient), "test-32-byte-encryption-key!!!!")

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil, nil)

	// Criada desativada: has_config tem que refletir a credencial, não o enabled.
	created, err := svc.Create(proj.ID.String(), "github", "GitHub", map[string]interface{}{
		"token": "ghp_test",
		"repo":  "owner/repo",
	}, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := svc.Get(created.ID.String())
	if !got.HasConfig || got.Config != nil {
		t.Errorf("HasConfig = %v, Config = %v; want true and nil", got.HasConfig, got.Config)
	}

	res, err := svc.FetchItemDetails(created.ID.String(), "42")
	if err != nil {
		t.Fatalf("fetch on disabled integration should degrade gracefully, got %v", err)
	}
	if res.Details != nil || res.Error == nil {
		t.Errorf("disabled integration returned %+v, want nil details and an error message", res)
	}
}
