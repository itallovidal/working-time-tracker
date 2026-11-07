package work_session_test

import (
	"context"
	"testing"
	"time"

	"working-time-tracker/internal/domain/project"
)

// O projeto de um cliente mensal não tem valor cobrado por hora (o contrato é um valor por mês). O ponto funciona do
// mesmo jeito: a sessão guarda o valor cobrado nulo, e as horas e o custo continuam contando.
func TestService_MonthlyCustomerProject_ClockInAndOut(t *testing.T) {
	orgSvc, personSvc, projSvc, teamSvc, memberSvc, taskSvc, wsSvc := setupDeps(t)
	ctx := context.Background()

	org, _ := orgSvc.Create("Org")
	testClient.Organization.UpdateOneID(org.ID).SetSolo(true).ExecX(ctx)
	owner, _ := personSvc.Create(org.ID.String(), "Ana", "ana@test.com")
	testClient.Person.UpdateOneID(owner.ID).SetIsOwner(true).ExecX(ctx)
	member, _ := personSvc.Create(org.ID.String(), "Bia", "bia@test.com")

	customerID := testClient.Customer.Create().SetOrganizationID(org.ID).SetName("Mensal").
		SetBillingMode("monthly").SetMonthlyFeeCents(630000).SetContractStart("2026-11-01").SaveX(ctx).ID.String()
	proj, _ := projSvc.Create(org.ID.String(), "Contrato", "", 0, project.Routine{})
	projectID := proj.ID.String()
	if _, err := projSvc.SetBilling(projectID, &customerID, nil); err != nil {
		t.Fatalf("set billing without a rate: %v", err)
	}
	tm, _ := teamSvc.Create(projectID, "Time")
	join(t, memberSvc, tm, member.ID.String())
	setRate(t, projectID, member.ID.String(), 2000)
	task1, err := taskSvc.Create(projectID, "Tarefa", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// O dono (a pessoa do modo solo) bate o ponto sem valor por hora: custo zero e valor cobrado nulo.
	opened, err := wsSvc.ClockIn(projectID, task1.ID.String(), owner.ID.String())
	if err != nil {
		t.Fatalf("the owner clocks in on a monthly customer's project: %v", err)
	}
	if !opened.OwnerHours || opened.PayRateCents == nil || *opened.PayRateCents != 0 || opened.BillRateCents != nil || opened.BillAmountCents != nil {
		t.Errorf("owner session = owner %v, pay %v, bill %v / %v; want owner, pay 0 and no billed rate or amount",
			opened.OwnerHours, opened.PayRateCents, opened.BillRateCents, opened.BillAmountCents)
	}
	time.Sleep(5 * time.Millisecond)
	closed, err := wsSvc.ClockOut(projectID, owner.ID.String())
	if err != nil {
		t.Fatalf("the owner clocks out: %v", err)
	}
	if closed.EndAt == nil || !closed.EndAt.After(closed.StartAt) || closed.BillRateCents != nil || closed.BillAmountCents != nil {
		t.Errorf("closed owner session = end %v, bill %v / %v; want an end and no billed rate or amount", closed.EndAt, closed.BillRateCents, closed.BillAmountCents)
	}

	// Quem tem custo no projeto também: o valor pago conta, o cobrado fica nulo.
	if _, err := wsSvc.ClockIn(projectID, task1.ID.String(), member.ID.String()); err != nil {
		t.Fatalf("a collaborator clocks in: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	memberSession, err := wsSvc.ClockOut(projectID, member.ID.String())
	if err != nil {
		t.Fatalf("a collaborator clocks out: %v", err)
	}
	if memberSession.PayRateCents == nil || *memberSession.PayRateCents != 2000 || memberSession.PayAmountCents == nil ||
		memberSession.BillRateCents != nil || memberSession.BillAmountCents != nil {
		t.Errorf("collaborator session = pay %v / %v, bill %v / %v; want 2000 with an amount, and no billed rate or amount",
			memberSession.PayRateCents, memberSession.PayAmountCents, memberSession.BillRateCents, memberSession.BillAmountCents)
	}

	// As horas contam: o projeto soma o tempo das duas sessões, e nada de receita por hora.
	sessions, err := wsSvc.ListByProject(projectID, nil, nil)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("list = %d sessions, %v; want 2", len(sessions), err)
	}
	ownerID := owner.ID.String()
	total, err := wsSvc.TotalTime(projectID, nil, &ownerID)
	if err != nil || total.TotalSeconds <= 0 || total.BillAmountCents != nil {
		t.Errorf("owner total = %+v, %v; want time and no billed amount", total, err)
	}
}
