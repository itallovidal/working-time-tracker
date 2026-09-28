package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/testutil"
)

func setupDeps(t *testing.T) (*organization.Service, *person.Service, *project.Service, *team.Service, *team.MembershipService, *task.Service) {
	cleanup(t)

	orgSvc := organization.NewService(organization.NewStore(testClient))
	personSvc := person.NewService(person.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))
	teamSvc := team.NewService(team.NewStore(testClient))
	memberSvc := team.NewMembershipService(team.NewMembershipStore(testClient))
	taskSvc := task.NewService(task.NewStore(testClient), team.NewMembershipStore(testClient), nil)

	return orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc
}

// createIntegration grava uma integração direto pelo Ent, sem validar credenciais
// na API externa. Serve para testes que só precisam de uma FK válida.
func createIntegration(t *testing.T, projectID string) string {
	t.Helper()
	it, err := testClient.Integration.Create().
		SetProjectID(uuid.MustParse(projectID)).
		SetType("github").
		SetDisplayName("GitHub").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create integration: %v", err)
	}
	return it.ID.String()
}

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create_DefaultDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "desc", p.ID.String(), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if task1.Name != "Task A" {
		t.Errorf("name = %q, want %q", task1.Name, "Task A")
	}
	if task1.Deadline.IsZero() {
		t.Error("expected non-zero deadline")
	}
	expected := time.Now().Add(7 * 24 * time.Hour)
	if task1.Deadline.Before(expected.Add(-time.Hour)) || task1.Deadline.After(expected.Add(time.Hour)) {
		t.Errorf("deadline = %v, expected ~%v", task1.Deadline, expected)
	}
}

func TestService_Create_ExplicitDeadline(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	dl := time.Now().Add(30 * 24 * time.Hour)
	task1, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), &dl)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// O Postgres guarda microssegundos, então o prazo lido pode perder os nanossegundos.
	if task1.Deadline.Sub(dl).Abs() >= time.Microsecond {
		t.Errorf("deadline = %v, want %v", task1.Deadline, dl)
	}
}

func TestService_Create_AssigneeNotTeamMember(t *testing.T) {
	orgSvc, personSvc, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for non-member assignee, got nil")
	}
}

func TestService_Create_MissingAssignee(t *testing.T) {
	orgSvc, _, projSvc, _, _, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)

	_, err := taskSvc.Create(proj.ID.String(), "Task A", "", "", nil)
	if err == nil {
		t.Fatal("expected error for empty assignee, got nil")
	}
}

func TestService_Create_MissingName(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	_, err := taskSvc.Create(proj.ID.String(), "", "", p.ID.String(), nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Update(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	created, _ := taskSvc.Create(proj.ID.String(), "Old", "", p.ID.String(), nil)
	updated, err := taskSvc.Update(created.ID.String(), "New", "new desc", nil, nil)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("name = %q, want %q", updated.Name, "New")
	}
	if updated.Description != "new desc" {
		t.Errorf("description = %q, want %q", updated.Description, "new desc")
	}
}

func TestService_Delete(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	err := taskSvc.Delete(task1.ID.String())
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = taskSvc.Get(task1.ID.String())
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestService_LinkUnlinkExternalItem(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())

	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	linked, err := taskSvc.LinkExternalItem(task1.ID.String(), createIntegration(t, proj.ID.String()), "42", "https://example.com/42")
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}
	if linked.ExternalItemID == nil || *linked.ExternalItemID != "42" {
		t.Errorf("external_item_id = %v, want 42", linked.ExternalItemID)
	}

	unlinked, err := taskSvc.UnlinkExternalItem(task1.ID.String())
	if err != nil {
		t.Fatalf("unlink failed: %v", err)
	}
	if unlinked.ExternalItemID != nil {
		t.Errorf("expected nil external_item_id after unlink, got %v", *unlinked.ExternalItemID)
	}
}

func TestService_Delete_CascadesWorkSessions(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)
	ctx := context.Background()

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	testClient.WorkSession.Create().SetTaskID(task1.ID).SetPersonID(p.ID).
		SetStartAt(time.Now().Add(-time.Hour)).SetEndAt(time.Now()).SaveX(ctx)
	testClient.WorkSession.Create().SetTaskID(task1.ID).SetPersonID(p.ID).
		SetStartAt(time.Now()).SaveX(ctx)

	if err := taskSvc.Delete(task1.ID.String()); err != nil {
		t.Fatalf("delete task with work sessions failed: %v", err)
	}
	if n := testClient.WorkSession.Query().CountX(ctx); n != 0 {
		t.Errorf("work_sessions: %d rows left, want 0", n)
	}
}

func TestService_LinkExternalItem_IntegrationFromAnotherProject(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	otherProj, _ := projSvc.Create(org.ID.String(), "Other", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	foreign := createIntegration(t, otherProj.ID.String())
	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), foreign, "42", "https://example.com/42"); err == nil {
		t.Fatal("expected error when linking an integration from another project")
	}
	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), "not-a-uuid", "42", "https://example.com/42"); err == nil {
		t.Fatal("expected error for an invalid integration id")
	}
}

func TestService_Update_AssigneeMustBeProjectMember(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	outsider, _ := personSvc.Create(org.ID.String(), "Maria", "maria@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)

	outsiderID := outsider.ID.String()
	if _, err := taskSvc.Update(task1.ID.String(), "Task A", "", &outsiderID, nil); err == nil {
		t.Fatal("expected error when assigning someone outside the project's teams")
	}
	bad := "not-a-uuid"
	if _, err := taskSvc.Update(task1.ID.String(), "Task A", "", &bad, nil); err == nil {
		t.Fatal("expected error for an invalid assignee id")
	}
}

// O vínculo com a integração precisa sobreviver à leitura e à edição da tarefa.
// Antes, o store não copiava external_integration_id: /external-details sempre
// dizia que não havia vínculo, e editar a tarefa apagava a integração.
func TestService_LinkSurvivesReadAndUpdate(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc := setupDeps(t)

	org, _ := orgSvc.Create("Org")
	p, _ := personSvc.Create(org.ID.String(), "John", "john@test.com")
	proj, _ := projSvc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	tm, _ := teamSvc.Create(proj.ID.String(), "Team")
	memberSvc.Add(tm.ID.String(), p.ID.String())
	task1, _ := taskSvc.Create(proj.ID.String(), "Task A", "", p.ID.String(), nil)
	integrationID := createIntegration(t, proj.ID.String())

	if _, err := taskSvc.LinkExternalItem(task1.ID.String(), integrationID, "42", "https://example.com/42"); err != nil {
		t.Fatalf("link: %v", err)
	}
	got, _ := taskSvc.Get(task1.ID.String())
	if got.ExternalIntegrationID == nil || got.ExternalIntegrationID.String() != integrationID {
		t.Fatalf("external_integration_id after link = %v, want %s", got.ExternalIntegrationID, integrationID)
	}

	updated, err := taskSvc.Update(task1.ID.String(), "Task A renomeada", "", nil, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.ExternalIntegrationID == nil || updated.ExternalItemID == nil || *updated.ExternalItemID != "42" {
		t.Errorf("update dropped the link: integration=%v item=%v", updated.ExternalIntegrationID, updated.ExternalItemID)
	}
}
