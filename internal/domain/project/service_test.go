package project_test

import (
	"context"
	"testing"
	"time"

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
	svc := project.NewService(project.NewStore(testClient))

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	_, err := svc.Create(org.ID.String(), "", "desc", 0, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_DefaultSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.SprintDurationDays != 14 {
		t.Errorf("sprint_duration_days = %d, want 14", proj.SprintDurationDays)
	}
}

// Sem duração informada, o projeto novo usa a sprint padrão da organização.
func TestService_Create_OrganizationSprintDefault(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	days := 7
	if _, err := orgSvc.Update(org.ID.String(), organization.UpdateInput{DefaultSprintDays: &days}); err != nil {
		t.Fatalf("set default sprint: %v", err)
	}

	proj, err := svc.Create(org.ID.String(), "Project", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.SprintDurationDays != 7 {
		t.Errorf("sprint_duration_days = %d, want the organization default 7", proj.SprintDurationDays)
	}
	explicit, err := svc.Create(org.ID.String(), "Other", "", 21, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if explicit.SprintDurationDays != 21 {
		t.Errorf("sprint_duration_days = %d, want the explicit 21", explicit.SprintDurationDays)
	}
}

func TestService_Create_ExplicitSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

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
	orgSvc := organization.NewService(organization.NewStore(testClient))
	projSvc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil)

	err := orgSvc.Delete(org.ID.String())
	if err == nil {
		t.Fatal("expected error deleting org with active projects, got nil")
	}
}

// Excluir um projeto apaga times, membros, tarefas, sessões e integrações dele.
func TestService_Delete_CascadesChildren(t *testing.T) {
	cleanup(t)
	ctx := context.Background()
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	proj, _ := svc.Create(org.ID.String(), "Projeto", "", 0, nil, nil)

	p := testClient.Person.Create().SetName("Ana").SetEmail("ana@test.com").SetOrganizationID(org.ID).SaveX(ctx)
	tm := testClient.Team.Create().SetName("Time").SetProjectID(proj.ID).SaveX(ctx)
	testClient.TeamMembership.Create().SetTeamID(tm.ID).SetPersonID(p.ID).SaveX(ctx)
	it := testClient.Integration.Create().SetProjectID(proj.ID).SetType("github").SetDisplayName("GitHub").SaveX(ctx)
	task := testClient.Task.Create().SetName("Tarefa").SetProjectID(proj.ID).SetAssigneeID(p.ID).
		SetExternalIntegrationID(it.ID).SaveX(ctx)
	testClient.WorkSession.Create().SetTaskID(task.ID).SetPersonID(p.ID).
		SetStartAt(time.Now().Add(-time.Hour)).SetEndAt(time.Now()).SaveX(ctx)

	if err := svc.Delete(proj.ID.String()); err != nil {
		t.Fatalf("delete project with children failed: %v", err)
	}

	counts := map[string]int{
		"teams":            testClient.Team.Query().CountX(ctx),
		"team_memberships": testClient.TeamMembership.Query().CountX(ctx),
		"tasks":            testClient.Task.Query().CountX(ctx),
		"work_sessions":    testClient.WorkSession.Query().CountX(ctx),
		"integrations":     testClient.Integration.Query().CountX(ctx),
	}
	for table, n := range counts {
		if n != 0 {
			t.Errorf("%s: %d rows left after deleting the project, want 0", table, n)
		}
	}
	if n := testClient.Person.Query().CountX(ctx); n != 1 {
		t.Errorf("persons: %d rows, want 1 (people belong to the organization, not the project)", n)
	}
}

// Nos campos opcionais do update, omitir mantém o valor e texto vazio apaga.
func TestService_Update_OptionalScheduleFields(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Org")
	daily, weekly := "09:30", "Friday"
	proj, err := svc.Create(org.ID.String(), "Projeto", "", 0, &daily, &weekly)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if proj.WeeklySyncDay == nil || *proj.WeeklySyncDay != "friday" {
		t.Errorf("weekly_sync_day = %v, want friday", proj.WeeklySyncDay)
	}

	kept, err := svc.Update(proj.ID.String(), "Projeto", "", 0, nil, nil)
	if err != nil {
		t.Fatalf("update keeping fields: %v", err)
	}
	if kept.DailyTime == nil || *kept.DailyTime != "09:30" || kept.WeeklySyncDay == nil {
		t.Errorf("omitted fields should be kept, got daily=%v weekly=%v", kept.DailyTime, kept.WeeklySyncDay)
	}

	empty := ""
	cleared, err := svc.Update(proj.ID.String(), "Projeto", "", 0, &empty, &empty)
	if err != nil {
		t.Fatalf("update clearing fields: %v", err)
	}
	reloaded, _ := svc.Get(proj.ID.String())
	if cleared.DailyTime != nil || reloaded.DailyTime != nil || reloaded.WeeklySyncDay != nil {
		t.Errorf("empty strings should clear, got daily=%v weekly=%v", reloaded.DailyTime, reloaded.WeeklySyncDay)
	}
}

func TestService_Create_InvalidSchedule(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")

	badTime, badDay := "25:00", "someday"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, &badTime, nil); err != project.ErrInvalidDailyTime {
		t.Errorf("invalid daily time: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, nil, &badDay); err != project.ErrInvalidWeekday {
		t.Errorf("invalid weekday: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 120, nil, nil); err != project.ErrInvalidSprint {
		t.Errorf("invalid sprint: err = %v", err)
	}
}

// O cliente e o valor cobrado ficam em Billing. O projeto mostra só o nome do
// cliente, nunca o valor.
func TestService_Billing(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	other, _ := orgSvc.Create("Outra")
	proj, _ := svc.Create(org.ID.String(), "Projeto X", "", 0, nil, nil)
	id := proj.ID.String()

	ctx := context.Background()
	mine := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa A").SaveX(ctx).ID.String()
	theirs := testClient.Customer.Create().SetOrganizationID(other.ID).SetName("Empresa B").SaveX(ctx).ID.String()

	empty, err := svc.Billing(id)
	if err != nil || empty.Customer != nil || empty.BillRateCents != nil {
		t.Fatalf("new project billing = %+v, %v; want empty", empty, err)
	}

	rate := 10000
	billing, err := svc.SetBilling(id, &mine, &rate)
	if err != nil {
		t.Fatalf("set billing: %v", err)
	}
	if billing.Customer == nil || billing.Customer.Name != "Empresa A" || billing.BillRateCents == nil || *billing.BillRateCents != 10000 {
		t.Errorf("billing = %+v", billing)
	}

	got, _ := svc.Get(id)
	if got.Customer == nil || got.Customer.Name != "Empresa A" {
		t.Errorf("project customer = %+v, want Empresa A", got.Customer)
	}
	// Editar o projeto não mexe no cliente nem no valor.
	if _, err := svc.Update(id, "Projeto X2", "", 0, nil, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	if after, _ := svc.Billing(id); after.Customer == nil || after.BillRateCents == nil || *after.BillRateCents != 10000 {
		t.Errorf("billing after project update = %+v, want it unchanged", after)
	}

	if _, err := svc.SetBilling(id, &theirs, &rate); err != project.ErrCustomerNotFound {
		t.Errorf("customer from another organization: err = %v, want ErrCustomerNotFound", err)
	}
	negative := -1
	if _, err := svc.SetBilling(id, &mine, &negative); err != project.ErrInvalidBillRate {
		t.Errorf("negative rate: err = %v, want ErrInvalidBillRate", err)
	}

	// nil apaga os dois: o projeto volta a ser interno.
	cleared, err := svc.SetBilling(id, nil, nil)
	if err != nil || cleared.Customer != nil || cleared.BillRateCents != nil {
		t.Errorf("cleared billing = %+v, %v; want empty", cleared, err)
	}
}
