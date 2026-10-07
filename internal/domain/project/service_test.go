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
	proj, err := svc.Create(org.ID.String(), "Project A", "desc", 0, nil, nil, nil)
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
	_, err := svc.Create(org.ID.String(), "", "desc", 0, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_DefaultSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if proj.SprintDurationDays != 14 {
		t.Errorf("sprint_duration_days = %d, want 14", proj.SprintDurationDays)
	}
}

func TestService_Create_ExplicitSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 21, nil, nil, nil)
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
	created, _ := svc.Create(org.ID.String(), "Old Name", "", 0, nil, nil, nil)
	updated, err := svc.Update(created.ID.String(), "New Name", "new desc", 10, nil, nil, nil)
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
	proj, _ := svc.Create(org.ID.String(), "Project A", "", 0, nil, nil, nil)

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
	projSvc.Create(org.ID.String(), "Project A", "", 0, nil, nil, nil)

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
	proj, _ := svc.Create(org.ID.String(), "Projeto", "", 0, nil, nil, nil)

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
	proj, err := svc.Create(org.ID.String(), "Projeto", "", 0, &daily, &weekly, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if proj.WeeklySyncDay == nil || *proj.WeeklySyncDay != "friday" {
		t.Errorf("weekly_sync_day = %v, want friday", proj.WeeklySyncDay)
	}

	kept, err := svc.Update(proj.ID.String(), "Projeto", "", 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("update keeping fields: %v", err)
	}
	if kept.DailyTime == nil || *kept.DailyTime != "09:30" || kept.WeeklySyncDay == nil {
		t.Errorf("omitted fields should be kept, got daily=%v weekly=%v", kept.DailyTime, kept.WeeklySyncDay)
	}

	empty := ""
	cleared, err := svc.Update(proj.ID.String(), "Projeto", "", 0, &empty, &empty, nil)
	if err != nil {
		t.Fatalf("update clearing fields: %v", err)
	}
	reloaded, _ := svc.Get(proj.ID.String())
	if cleared.DailyTime != nil || reloaded.DailyTime != nil || reloaded.WeeklySyncDay != nil {
		t.Errorf("empty strings should clear, got daily=%v weekly=%v", reloaded.DailyTime, reloaded.WeeklySyncDay)
	}
}

// A daily e a weekly são independentes: o projeto pode ter só uma, e a weekly
// tem dia e horário. Sem o dia, o horário da weekly não existe.
func TestService_WeeklySyncTime(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")

	day, at := "Tuesday", "13:00"
	proj, err := svc.Create(org.ID.String(), "So weekly", "", 0, nil, &day, &at)
	if err != nil {
		t.Fatalf("create with weekly only: %v", err)
	}
	if proj.DailyTime != nil || proj.WeeklySyncDay == nil || *proj.WeeklySyncDay != "tuesday" ||
		proj.WeeklySyncTime == nil || *proj.WeeklySyncTime != "13:00" {
		t.Errorf("weekly only: daily=%v day=%v time=%v", proj.DailyTime, proj.WeeklySyncDay, proj.WeeklySyncTime)
	}

	// Omitir mantém o horário; trocar só o horário vale.
	kept, _ := svc.Update(proj.ID.String(), "So weekly", "", 0, nil, nil, nil)
	if kept.WeeklySyncTime == nil || *kept.WeeklySyncTime != "13:00" {
		t.Errorf("omitted weekly time should be kept, got %v", kept.WeeklySyncTime)
	}
	later := "15:30"
	moved, err := svc.Update(proj.ID.String(), "So weekly", "", 0, nil, nil, &later)
	if err != nil || moved.WeeklySyncTime == nil || *moved.WeeklySyncTime != "15:30" {
		t.Errorf("change only the weekly time: %v, %v", moved, err)
	}

	// Apagar o dia apaga o horário junto, e a daily segue.
	daily, empty := "10:00", ""
	if _, err := svc.Update(proj.ID.String(), "So weekly", "", 0, &daily, nil, nil); err != nil {
		t.Fatalf("add daily: %v", err)
	}
	cleared, err := svc.Update(proj.ID.String(), "So weekly", "", 0, nil, &empty, nil)
	if err != nil {
		t.Fatalf("clear weekly: %v", err)
	}
	reloaded, _ := svc.Get(proj.ID.String())
	if cleared.WeeklySyncTime != nil || reloaded.WeeklySyncDay != nil || reloaded.WeeklySyncTime != nil {
		t.Errorf("clearing the day must clear the time, got day=%v time=%v", reloaded.WeeklySyncDay, reloaded.WeeklySyncTime)
	}
	if reloaded.DailyTime == nil || *reloaded.DailyTime != "10:00" {
		t.Errorf("clearing the weekly must keep the daily, got %v", reloaded.DailyTime)
	}

	// Horário sem dia e horário inválido são recusados.
	if _, err := svc.Update(proj.ID.String(), "So weekly", "", 0, nil, nil, &at); err != project.ErrWeeklyTimeWithoutDay {
		t.Errorf("time without day on update: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, nil, nil, &at); err != project.ErrWeeklyTimeWithoutDay {
		t.Errorf("time without day on create: err = %v", err)
	}
	bad := "24:00"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, nil, &day, &bad); err != project.ErrInvalidWeeklyTime {
		t.Errorf("invalid weekly time: err = %v", err)
	}
}

func TestService_Create_InvalidSchedule(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")

	badTime, badDay := "25:00", "someday"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, &badTime, nil, nil); err != project.ErrInvalidDailyTime {
		t.Errorf("invalid daily time: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, nil, &badDay, nil); err != project.ErrInvalidWeekday {
		t.Errorf("invalid weekday: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 120, nil, nil, nil); err != project.ErrInvalidSprint {
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
	proj, _ := svc.Create(org.ID.String(), "Projeto X", "", 0, nil, nil, nil)
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
	if _, err := svc.Update(id, "Projeto X2", "", 0, nil, nil, nil); err != nil {
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
