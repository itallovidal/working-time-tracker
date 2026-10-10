package project_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/validate"
	"working-time-tracker/testutil"
)

func ptr[T any](v T) *T { return &v }

func cleanup(t *testing.T) {
	testutil.Truncate(t, testDB)
}

func TestService_Create(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "desc", 0, project.Routine{})
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
	_, err := svc.Create(org.ID.String(), "", "desc", 0, project.Routine{})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestService_Create_DefaultSprintDuration(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))

	org, _ := orgSvc.Create("Test Org")
	proj, err := svc.Create(org.ID.String(), "Project A", "", 0, project.Routine{})
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
	proj, err := svc.Create(org.ID.String(), "Project A", "", 21, project.Routine{})
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
	created, _ := svc.Create(org.ID.String(), "Old Name", "", 0, project.Routine{})
	updated, err := svc.Update(created.ID.String(), ptr("New Name"), ptr("new desc"), ptr(10), project.Routine{})
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
	proj, _ := svc.Create(org.ID.String(), "Project A", "", 0, project.Routine{})

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
	projSvc.Create(org.ID.String(), "Project A", "", 0, project.Routine{})

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
	proj, _ := svc.Create(org.ID.String(), "Projeto", "", 0, project.Routine{})

	p := testClient.Person.Create().SetName("Ana").SetEmail("ana@test.com").SetOrganizationID(org.ID).SaveX(ctx)
	tm := testClient.Team.Create().SetName("Time").SetProjectID(proj.ID).SaveX(ctx)
	testClient.TeamMembership.Create().SetTeamID(tm.ID).SetPersonID(p.ID).SaveX(ctx)
	it := testClient.Integration.Create().SetProjectID(proj.ID).SetType("github").SetDisplayName("GitHub").SaveX(ctx)
	task := testClient.Task.Create().SetName("Tarefa").SetProjectID(proj.ID).SetAssigneeID(p.ID).SaveX(ctx)
	testClient.IssueSync.Create().SetIntegrationID(it.ID).SetTaskID(task.ID).SetItemID("42").SaveX(ctx)
	endedAt := time.Now()
	testutil.Session(t, testClient, task.ID, p.ID, endedAt.Add(-time.Hour), &endedAt, nil, nil)

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
	daily, weekly, weeklyAt := "09:30", "Friday", "10:00"
	proj, err := svc.Create(org.ID.String(), "Projeto", "", 0, project.Routine{DailyTime: &daily, WeeklySyncDay: &weekly, WeeklySyncTime: &weeklyAt})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if proj.WeeklySyncDay == nil || *proj.WeeklySyncDay != "friday" {
		t.Errorf("weekly_sync_day = %v, want friday", proj.WeeklySyncDay)
	}

	kept, err := svc.Update(proj.ID.String(), ptr("Projeto"), nil, nil, project.Routine{})
	if err != nil {
		t.Fatalf("update keeping fields: %v", err)
	}
	if kept.DailyTime == nil || *kept.DailyTime != "09:30" || kept.WeeklySyncDay == nil {
		t.Errorf("omitted fields should be kept, got daily=%v weekly=%v", kept.DailyTime, kept.WeeklySyncDay)
	}

	empty := ""
	cleared, err := svc.Update(proj.ID.String(), ptr("Projeto"), nil, nil, project.Routine{DailyTime: &empty, WeeklySyncDay: &empty})
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
	proj, err := svc.Create(org.ID.String(), "So weekly", "", 0, project.Routine{WeeklySyncDay: &day, WeeklySyncTime: &at})
	if err != nil {
		t.Fatalf("create with weekly only: %v", err)
	}
	if proj.DailyTime != nil || proj.WeeklySyncDay == nil || *proj.WeeklySyncDay != "tuesday" ||
		proj.WeeklySyncTime == nil || *proj.WeeklySyncTime != "13:00" {
		t.Errorf("weekly only: daily=%v day=%v time=%v", proj.DailyTime, proj.WeeklySyncDay, proj.WeeklySyncTime)
	}

	// Omitir mantém o horário; trocar só o horário vale.
	kept, _ := svc.Update(proj.ID.String(), ptr("So weekly"), nil, nil, project.Routine{})
	if kept.WeeklySyncTime == nil || *kept.WeeklySyncTime != "13:00" {
		t.Errorf("omitted weekly time should be kept, got %v", kept.WeeklySyncTime)
	}
	later := "15:30"
	moved, err := svc.Update(proj.ID.String(), ptr("So weekly"), nil, nil, project.Routine{WeeklySyncTime: &later})
	if err != nil || moved.WeeklySyncTime == nil || *moved.WeeklySyncTime != "15:30" {
		t.Errorf("change only the weekly time: %v, %v", moved, err)
	}

	// Apagar o dia apaga o horário junto, e a daily segue.
	daily, empty := "10:00", ""
	if _, err := svc.Update(proj.ID.String(), ptr("So weekly"), nil, nil, project.Routine{DailyTime: &daily}); err != nil {
		t.Fatalf("add daily: %v", err)
	}
	cleared, err := svc.Update(proj.ID.String(), ptr("So weekly"), nil, nil, project.Routine{WeeklySyncDay: &empty})
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
	if _, err := svc.Update(proj.ID.String(), ptr("So weekly"), nil, nil, project.Routine{WeeklySyncTime: &at}); !errors.Is(err, project.ErrWeeklyTimeWithoutDay) {
		t.Errorf("time without day on update: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{WeeklySyncTime: &at}); !errors.Is(err, project.ErrWeeklyTimeWithoutDay) {
		t.Errorf("time without day on create: err = %v", err)
	}
	bad := "24:00"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{WeeklySyncDay: &day, WeeklySyncTime: &bad}); !errors.Is(err, project.ErrInvalidWeeklyTime) {
		t.Errorf("invalid weekly time: err = %v", err)
	}
}

func TestService_Create_InvalidSchedule(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")

	badTime, badDay := "25:00", "someday"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{DailyTime: &badTime}); !errors.Is(err, project.ErrInvalidDailyTime) {
		t.Errorf("invalid daily time: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{WeeklySyncDay: &badDay}); !errors.Is(err, project.ErrInvalidWeekday) {
		t.Errorf("invalid weekday: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 120, project.Routine{}); !errors.Is(err, project.ErrInvalidSprint) {
		t.Errorf("invalid sprint: err = %v", err)
	}
}

// A reunião com o cliente é um dia e um horário, como a weekly do time, e vale por si:
// não depende da weekly. Omitir mantém, vazio apaga, e o horário não existe sem o dia.
func TestService_CustomerMeeting(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")

	customerID := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa").SaveX(context.Background()).ID.String()

	day, at := "Wednesday", "10:30"
	proj, err := svc.CreateWithCustomer(org.ID.String(), project.CreateInput{
		Name: "Com reuniao", Routine: project.Routine{CustomerMeetingDay: &day, CustomerMeetingTime: &at}, CustomerID: customerID, BillRateCents: ptr(10000),
	})
	if err != nil {
		t.Fatalf("create with a customer meeting: %v", err)
	}
	if proj.CustomerMeetingDay == nil || *proj.CustomerMeetingDay != "wednesday" ||
		proj.CustomerMeetingTime == nil || *proj.CustomerMeetingTime != "10:30" {
		t.Errorf("meeting: day=%v time=%v, want wednesday 10:30", proj.CustomerMeetingDay, proj.CustomerMeetingTime)
	}
	if proj.WeeklySyncDay != nil || proj.WeeklySyncTime != nil {
		t.Errorf("the team's weekly must stay empty, got %v %v", proj.WeeklySyncDay, proj.WeeklySyncTime)
	}

	// Omitir mantém; trocar só o horário vale.
	kept, _ := svc.Update(proj.ID.String(), ptr("Com reuniao"), nil, nil, project.Routine{})
	if kept.CustomerMeetingDay == nil || kept.CustomerMeetingTime == nil {
		t.Errorf("an omitted meeting should be kept, got %v %v", kept.CustomerMeetingDay, kept.CustomerMeetingTime)
	}
	later := "16:00"
	moved, err := svc.Update(proj.ID.String(), ptr("Com reuniao"), nil, nil, project.Routine{CustomerMeetingTime: &later})
	if err != nil || moved.CustomerMeetingTime == nil || *moved.CustomerMeetingTime != "16:00" {
		t.Errorf("change only the meeting time: %v, %v", moved, err)
	}

	// Apagar o dia apaga o horário junto, e a weekly do time não é tocada.
	weekly, weeklyAt, empty := "friday", "09:00", ""
	if _, err := svc.Update(proj.ID.String(), ptr("Com reuniao"), nil, nil, project.Routine{WeeklySyncDay: &weekly, WeeklySyncTime: &weeklyAt}); err != nil {
		t.Fatalf("add the weekly: %v", err)
	}
	if _, err := svc.Update(proj.ID.String(), ptr("Com reuniao"), nil, nil, project.Routine{CustomerMeetingDay: &empty}); err != nil {
		t.Fatalf("clear the meeting: %v", err)
	}
	reloaded, _ := svc.Get(proj.ID.String())
	if reloaded.CustomerMeetingDay != nil || reloaded.CustomerMeetingTime != nil {
		t.Errorf("clearing the day must clear the time, got %v %v", reloaded.CustomerMeetingDay, reloaded.CustomerMeetingTime)
	}
	if reloaded.WeeklySyncDay == nil || *reloaded.WeeklySyncDay != "friday" {
		t.Errorf("clearing the meeting must keep the weekly, got %v", reloaded.WeeklySyncDay)
	}

	// Horário sem dia, dia inválido e horário inválido são recusados.
	if _, err := svc.Update(proj.ID.String(), ptr("Com reuniao"), nil, nil, project.Routine{CustomerMeetingTime: &at}); !errors.Is(err, project.ErrMeetingTimeWithoutDay) {
		t.Errorf("time without day on update: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{CustomerMeetingTime: &at}); !errors.Is(err, project.ErrMeetingTimeWithoutDay) {
		t.Errorf("time without day on create: err = %v", err)
	}
	badDay, badTime := "someday", "24:00"
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{CustomerMeetingDay: &badDay}); !errors.Is(err, project.ErrInvalidWeekday) {
		t.Errorf("invalid meeting day: err = %v", err)
	}
	if _, err := svc.Create(org.ID.String(), "P", "", 0, project.Routine{CustomerMeetingDay: &day, CustomerMeetingTime: &badTime}); !errors.Is(err, project.ErrInvalidMeetingTime) {
		t.Errorf("invalid meeting time: err = %v", err)
	}
}

// Dia e horário andam juntos, na weekly e na reunião com o cliente: gravar o dia sem o horário é recusado, na
// criação e na edição, com o horário no erro, como a tela exige. Weeklies antigas ficaram só com o dia; quem edita
// outro campo delas segue valendo, porque o corpo não mexeu no slot.
func TestService_SlotDayNeedsTime(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	orgID := org.ID.String()
	ctx := context.Background()
	customer := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa").SaveX(ctx)

	weekday, at, other, empty := "friday", "10:00", "monday", ""
	wantRequired := func(label string, err error, field string) {
		t.Helper()
		if !errors.Is(err, apperr.ErrFieldRequired) || fieldOf(err) != field {
			t.Errorf("%s: err = %v (field %q), want request.field_required on %s", label, err, fieldOf(err), field)
		}
	}

	// Na criação, o dia sozinho (ou com o horário vazio) é recusado.
	_, err := svc.Create(orgID, "P", "", 0, project.Routine{WeeklySyncDay: &weekday})
	wantRequired("create weekly day only", err, "weekly_sync_time")
	_, err = svc.Create(orgID, "P", "", 0, project.Routine{WeeklySyncDay: &weekday, WeeklySyncTime: &empty})
	wantRequired("create weekly day, empty time", err, "weekly_sync_time")
	_, err = svc.CreateWithCustomer(orgID, project.CreateInput{Name: "P", CustomerID: customer.ID.String(), BillRateCents: ptr(10000), Routine: project.Routine{CustomerMeetingDay: &weekday}})
	wantRequired("create meeting day only", err, "customer_meeting_time")

	// Com os dois, ou com os dois vazios, vale.
	ok, err := svc.CreateWithCustomer(orgID, project.CreateInput{Name: "Completo", CustomerID: customer.ID.String(), BillRateCents: ptr(10000), Routine: project.Routine{
		WeeklySyncDay: &weekday, WeeklySyncTime: &at, CustomerMeetingDay: &other, CustomerMeetingTime: &at,
	}})
	if err != nil {
		t.Fatalf("create with both: %v", err)
	}
	if _, err := svc.Create(orgID, "Vazio", "", 0, project.Routine{WeeklySyncDay: &empty, WeeklySyncTime: &empty}); err != nil {
		t.Errorf("create with an empty slot: %v", err)
	}
	id := ok.ID.String()

	// Na edição: trocar só o dia mantém o horário; limpar só o horário deixaria o dia sozinho.
	moved, err := svc.Update(id, ptr("Completo"), nil, nil, project.Routine{WeeklySyncDay: &other})
	if err != nil || moved.WeeklySyncTime == nil || *moved.WeeklySyncTime != at {
		t.Errorf("change only the weekly day: %v, %v", moved, err)
	}
	_, err = svc.Update(id, ptr("Completo"), nil, nil, project.Routine{WeeklySyncTime: &empty})
	wantRequired("clear only the weekly time", err, "weekly_sync_time")
	_, err = svc.Update(id, ptr("Completo"), nil, nil, project.Routine{CustomerMeetingTime: &empty})
	wantRequired("clear only the meeting time", err, "customer_meeting_time")
	if _, err := svc.Update(id, ptr("Completo"), nil, nil, project.Routine{WeeklySyncDay: &empty}); err != nil {
		t.Errorf("clearing the day clears the slot: %v", err)
	}

	// Linhas antigas, só com o dia: editar outro campo vale; mexer no slot sem dar o horário não.
	legacy := testClient.Project.Create().SetName("Antigo").SetOrganizationID(org.ID).SetCustomerID(customer.ID).
		SetWeeklySyncDay("friday").SetCustomerMeetingDay("monday").SaveX(ctx)
	lid := legacy.ID.String()
	desc := "nova descrição"
	if _, err := svc.Update(lid, ptr("Antigo renomeado"), &desc, nil, project.Routine{}); err != nil {
		t.Errorf("rename a project whose weekly has no time: %v", err)
	}
	_, err = svc.Update(lid, ptr("Antigo renomeado"), nil, nil, project.Routine{WeeklySyncDay: &other})
	wantRequired("legacy weekly, new day without time", err, "weekly_sync_time")
	_, err = svc.Update(lid, ptr("Antigo renomeado"), nil, nil, project.Routine{CustomerMeetingDay: &weekday})
	wantRequired("legacy meeting, new day without time", err, "customer_meeting_time")
	fixed, err := svc.Update(lid, ptr("Antigo renomeado"), nil, nil, project.Routine{WeeklySyncDay: &other, WeeklySyncTime: &at})
	if err != nil || fixed.WeeklySyncTime == nil || *fixed.WeeklySyncTime != at {
		t.Errorf("give the legacy weekly its time: %v, %v", fixed, err)
	}
}

// Projeto interno não tem reunião com o cliente: tirar o cliente apaga a reunião, e
// trocar de cliente mantém.
func TestService_Billing_ClearingTheCustomerClearsTheMeeting(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	ctx := context.Background()
	a := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa A").SaveX(ctx).ID.String()
	b := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa B").SaveX(ctx).ID.String()

	day, at := "monday", "09:00"
	proj, err := svc.CreateWithCustomer(org.ID.String(), project.CreateInput{
		Name: "Projeto", Routine: project.Routine{CustomerMeetingDay: &day, CustomerMeetingTime: &at}, CustomerID: a, BillRateCents: ptr(10000),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := proj.ID.String()

	if _, err := svc.SetBilling(id, &a, ptr(10000)); err != nil {
		t.Fatalf("set customer: %v", err)
	}
	if _, err := svc.SetBilling(id, &b, ptr(10000)); err != nil {
		t.Fatalf("change customer: %v", err)
	}
	if got, _ := svc.Get(id); got.CustomerMeetingDay == nil || got.CustomerMeetingTime == nil {
		t.Errorf("changing the customer must keep the meeting, got %v %v", got.CustomerMeetingDay, got.CustomerMeetingTime)
	}
	if _, err := svc.SetBilling(id, nil, nil); err != nil {
		t.Fatalf("clear customer: %v", err)
	}
	if got, _ := svc.Get(id); got.CustomerMeetingDay != nil || got.CustomerMeetingTime != nil {
		t.Errorf("an internal project has no customer meeting, got %v %v", got.CustomerMeetingDay, got.CustomerMeetingTime)
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
	proj, _ := svc.Create(org.ID.String(), "Projeto X", "", 0, project.Routine{})
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
	if _, err := svc.Update(id, ptr("Projeto X2"), nil, nil, project.Routine{}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if after, _ := svc.Billing(id); after.Customer == nil || after.BillRateCents == nil || *after.BillRateCents != 10000 {
		t.Errorf("billing after project update = %+v, want it unchanged", after)
	}

	if _, err := svc.SetBilling(id, &theirs, &rate); !errors.Is(err, project.ErrCustomerNotFound) {
		t.Errorf("customer from another organization: err = %v, want ErrCustomerNotFound", err)
	}
	negative := -1
	if _, err := svc.SetBilling(id, &mine, &negative); !errors.Is(err, project.ErrInvalidBillRate) {
		t.Errorf("negative rate: err = %v, want ErrInvalidBillRate", err)
	}

	// nil apaga os dois: o projeto volta a ser interno.
	cleared, err := svc.SetBilling(id, nil, nil)
	if err != nil || cleared.Customer != nil || cleared.BillRateCents != nil {
		t.Errorf("cleared billing = %+v, %v; want empty", cleared, err)
	}
}

func fieldOf(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		s, _ := e.Params["field"].(string)
		return s
	}
	return ""
}

// O nome também mantém quando não vem, como a descrição e a sprint (e como a organização e o cliente). Em branco, ou
// só espaços, continua recusado, e a recusa não troca o nome.
func TestService_Update_NameOmittedKeeps(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	proj, err := svc.Create(org.ID.String(), "Original", "texto", 0, project.Routine{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	desc := "nova"
	got, err := svc.Update(proj.ID.String(), nil, &desc, nil, project.Routine{})
	if err != nil || got.Name != "Original" || got.Description != "nova" {
		t.Fatalf("update without a name = %+v, %v; want the name kept and the description changed", got, err)
	}
	renamed, err := svc.Update(proj.ID.String(), ptr("  Renomeado  "), nil, nil, project.Routine{})
	if err != nil || renamed.Name != "Renomeado" || renamed.Description != "nova" {
		t.Fatalf("rename = %+v, %v; want the trimmed name and the description kept", renamed, err)
	}
	for _, blank := range []string{"", "   "} {
		if _, err := svc.Update(proj.ID.String(), ptr(blank), nil, nil, project.Routine{}); !errors.Is(err, project.ErrNameRequired) || fieldOf(err) != "name" {
			t.Errorf("update with name %q: err = %v, want project.name_required on name", blank, err)
		}
	}
	reloaded, _ := svc.Get(proj.ID.String())
	if reloaded.Name != "Renomeado" {
		t.Errorf("a refused blank name left the name as %q", reloaded.Name)
	}
}

// Nome (1 a 120, aparado) e descrição (até 2.000, aparada): na criação e na edição, com o campo no erro.
func TestService_NameAndDescriptionRules(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	orgID := org.ID.String()
	base, _ := svc.Create(orgID, "Base", "", 0, project.Routine{})
	rep := func(n int) string { return strings.Repeat("x", n) }

	cases := []struct {
		name, projName, desc string
		want                 error
		field                string
	}{
		{"name empty", "", "", project.ErrNameRequired, "name"},
		{"name spaces", "   ", "", project.ErrNameRequired, "name"},
		{"name at limit", rep(120), "", nil, ""},
		{"name at limit between spaces", "  " + rep(120) + "  ", "", nil, ""},
		{"name over limit", rep(121), "", apperr.ErrFieldTooLong, "name"},
		{"name with accents counts runes", strings.Repeat("ç", 120), "", nil, ""},
		{"description at limit", "P", rep(2000), nil, ""},
		{"description over limit", "P", rep(2001), apperr.ErrFieldTooLong, "long_description"},
		{"description spaces only", "P", "   ", nil, ""},
	}
	for _, c := range cases {
		check := func(where string, err error) {
			if c.want == nil {
				if err != nil {
					t.Errorf("%s %s: err = %v, want none", where, c.name, err)
				}
				return
			}
			if !errors.Is(err, c.want) || fieldOf(err) != c.field {
				t.Errorf("%s %s: err = %v field %q, want %v field %q", where, c.name, err, fieldOf(err), c.want, c.field)
			}
		}
		_, err := svc.Create(orgID, c.projName, c.desc, 0, project.Routine{})
		check("create", err)
		desc := c.desc
		_, err = svc.Update(base.ID.String(), ptr(c.projName), &desc, nil, project.Routine{})
		check("update", err)
	}

	// Aparado ao gravar.
	got, err := svc.Create(orgID, "  Com espaços  ", "  desc  ", 0, project.Routine{})
	if err != nil || got.Name != "Com espaços" || got.Description != "desc" {
		t.Errorf("trimmed create = %+v, %v", got, err)
	}
}

// Na edição, omitir a descrição mantém e "" apaga.
func TestService_Update_DescriptionOmittedKeeps(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	proj, _ := svc.Create(org.ID.String(), "P", "texto", 0, project.Routine{})
	id := proj.ID.String()

	kept, err := svc.Update(id, ptr("P"), nil, nil, project.Routine{})
	if err != nil || kept.Description != "texto" {
		t.Fatalf("omitted description = %q, %v; want it kept", kept.Description, err)
	}
	empty := ""
	cleared, err := svc.Update(id, ptr("P"), &empty, nil, project.Routine{})
	if err != nil || cleared.Description != "" {
		t.Fatalf("empty description = %q, %v; want it cleared", cleared.Description, err)
	}
}

// A sprint vai de 1 a 90, igual na criação e na edição; zero não é um valor. Ausente: 14 na criação, mantém na edição.
func TestService_SprintRules(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	orgID := org.ID.String()
	proj, _ := svc.Create(orgID, "P", "", 21, project.Routine{})
	id := proj.ID.String()

	for _, n := range []int{1, 90} {
		if _, err := svc.Create(orgID, "P", "", n, project.Routine{}); err != nil {
			t.Errorf("create with sprint %d: %v", n, err)
		}
		if _, err := svc.Update(id, ptr("P"), nil, ptr(n), project.Routine{}); err != nil {
			t.Errorf("update with sprint %d: %v", n, err)
		}
	}
	for _, n := range []int{-1, 91} {
		if _, err := svc.Create(orgID, "P", "", n, project.Routine{}); !errors.Is(err, project.ErrInvalidSprint) || fieldOf(err) != "sprint_duration_days" {
			t.Errorf("create with sprint %d: err = %v", n, err)
		}
	}
	for _, n := range []int{-1, 0, 91} {
		if _, err := svc.Update(id, ptr("P"), nil, ptr(n), project.Routine{}); !errors.Is(err, project.ErrInvalidSprint) || fieldOf(err) != "sprint_duration_days" {
			t.Errorf("update with sprint %d: err = %v", n, err)
		}
	}
	kept, err := svc.Update(id, ptr("P"), nil, nil, project.Routine{})
	if err != nil || kept.SprintDurationDays != 90 {
		t.Errorf("an omitted sprint should keep the current (90): %+v, %v", kept, err)
	}
}

// Reunião com o cliente só vale com cliente: na criação (com customer_id) e na edição.
func TestService_MeetingNeedsCustomer(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	other, _ := orgSvc.Create("Outra")
	orgID := org.ID.String()
	ctx := context.Background()
	mine := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("A").SaveX(ctx).ID.String()
	theirs := testClient.Customer.Create().SetOrganizationID(other.ID).SetName("B").SaveX(ctx).ID.String()
	day, at, empty := "monday", "10:00", ""

	if _, err := svc.Create(orgID, "P", "", 0, project.Routine{CustomerMeetingDay: &day}); !errors.Is(err, project.ErrMeetingNeedsCustomer) || fieldOf(err) != "customer_meeting_day" {
		t.Errorf("create with a meeting and no customer: err = %v", err)
	}
	// Dia vazio não é reunião.
	if _, err := svc.Create(orgID, "P", "", 0, project.Routine{CustomerMeetingDay: &empty}); err != nil {
		t.Errorf("create with an empty meeting day: %v", err)
	}
	if _, err := svc.CreateWithCustomer(orgID, project.CreateInput{Name: "P", CustomerID: theirs, BillRateCents: ptr(10000)}); !errors.Is(err, project.ErrCustomerNotFound) || fieldOf(err) != "customer_id" {
		t.Errorf("create with a customer of another organization: err = %v", err)
	}
	with, err := svc.CreateWithCustomer(orgID, project.CreateInput{Name: "P", CustomerID: mine, BillRateCents: ptr(10000), Routine: project.Routine{CustomerMeetingDay: &day, CustomerMeetingTime: &at}})
	if err != nil || with.Customer == nil || with.Customer.Name != "A" || with.CustomerMeetingDay == nil {
		t.Fatalf("create with customer and meeting = %+v, %v", with, err)
	}

	internal, _ := svc.Create(orgID, "Interno", "", 0, project.Routine{})
	if _, err := svc.Update(internal.ID.String(), ptr("Interno"), nil, nil, project.Routine{CustomerMeetingDay: &day}); !errors.Is(err, project.ErrMeetingNeedsCustomer) || fieldOf(err) != "customer_meeting_day" {
		t.Errorf("update with a meeting and no customer: err = %v", err)
	}
	if _, err := svc.SetBilling(internal.ID.String(), &mine, ptr(10000)); err != nil {
		t.Fatalf("set customer: %v", err)
	}
	if _, err := svc.Update(internal.ID.String(), ptr("Interno"), nil, nil, project.Routine{CustomerMeetingDay: &day, CustomerMeetingTime: &at}); err != nil {
		t.Errorf("update with a meeting and a customer: %v", err)
	}
}

// O valor cobrado vai de 10,00 a 1.000.000,00, com o campo no erro, e anda junto do cliente: com cliente é
// obrigatório, sem cliente é recusado.
func TestService_SetBilling_RateLimits(t *testing.T) {
	cleanup(t)
	orgSvc := organization.NewService(organization.NewStore(testClient))
	svc := project.NewService(project.NewStore(testClient))
	org, _ := orgSvc.Create("Org")
	proj, _ := svc.Create(org.ID.String(), "P", "", 0, project.Routine{})
	id := proj.ID.String()
	cust := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Empresa").SaveX(context.Background()).ID.String()

	for _, n := range []int{validate.MinRateCents, validate.MaxCents} {
		if _, err := svc.SetBilling(id, &cust, ptr(n)); err != nil {
			t.Errorf("rate %d: %v", n, err)
		}
	}
	for _, n := range []int{0, validate.MinRateCents - 1, -1, validate.MaxCents + 1} {
		if _, err := svc.SetBilling(id, &cust, ptr(n)); !errors.Is(err, project.ErrInvalidBillRate) || fieldOf(err) != "bill_rate_cents" {
			t.Errorf("rate %d: err = %v", n, err)
		}
	}
	// Com cliente o valor é obrigatório; sem cliente o valor é recusado, e os dois nulos voltam ao interno.
	if _, err := svc.SetBilling(id, &cust, nil); !errors.Is(err, project.ErrBillRateRequired) || fieldOf(err) != "bill_rate_cents" {
		t.Errorf("customer without a rate: err = %v", err)
	}
	if _, err := svc.SetBilling(id, nil, ptr(10000)); !errors.Is(err, project.ErrBillRateNeedsCustomer) || fieldOf(err) != "bill_rate_cents" {
		t.Errorf("rate without a customer: err = %v", err)
	}
	if b, err := svc.SetBilling(id, nil, nil); err != nil || b.Customer != nil || b.BillRateCents != nil {
		t.Errorf("back to internal: %+v, %v", b, err)
	}
	// A criação segue a mesma regra, numa chamada só.
	if _, err := svc.CreateWithCustomer(org.ID.String(), project.CreateInput{Name: "Sem valor", CustomerID: cust}); !errors.Is(err, project.ErrBillRateRequired) {
		t.Errorf("create with a customer and no rate: err = %v", err)
	}
	if _, err := svc.CreateWithCustomer(org.ID.String(), project.CreateInput{Name: "Sem cliente", BillRateCents: ptr(10000)}); !errors.Is(err, project.ErrBillRateNeedsCustomer) {
		t.Errorf("create with a rate and no customer: err = %v", err)
	}
	created, err := svc.CreateWithCustomer(org.ID.String(), project.CreateInput{Name: "Completo", CustomerID: cust, BillRateCents: ptr(10000)})
	if err != nil {
		t.Fatalf("create with a customer and a rate: %v", err)
	}
	if b, _ := svc.Billing(created.ID.String()); b == nil || b.BillRateCents == nil || *b.BillRateCents != 10000 {
		t.Errorf("billing after create = %+v, want 10000", b)
	}
	bad := "não-é-uuid"
	if _, err := svc.SetBilling(id, &bad, nil); !errors.Is(err, project.ErrCustomerNotFound) || fieldOf(err) != "customer_id" {
		t.Errorf("malformed customer: err = %v", err)
	}
}
