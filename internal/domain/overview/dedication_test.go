package overview_test

import (
	"context"
	"math"
	"testing"

	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/collaborator"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/integration"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/team"
	"working-time-tracker/internal/domain/work_session"
)

// A dedicação aos contratos mensais com estimativa de horas (Decisão 26) na tela inicial do modo solo e na visão geral
// do projeto. O cenário: hoje é quarta 18/11/2026, meio-dia em São Paulo.
const dedicationNow = "2026-11-18T15:00:00Z"

func roundHours(seconds float64) float64 { return math.Round(seconds/36) / 100 }

// monthlyClosing cria um cliente mensal com estimativa de horas, que fecha no dia closing.
func (f *soloFixture) monthlyClosing(name string, closing int, start string, hours int) *ent.Customer {
	f.t.Helper()
	return testClient.Customer.Create().SetOrganizationID(f.orgID).SetName(name).
		SetBillingMode("monthly").SetMonthlyFeeCents(300000).SetContractStart(start).
		SetClosingDay(closing).SetPaymentDays(10).SetEstimatedMonthlyHours(hours).SaveX(context.Background())
}

func rowOf(o *overview.SoloOverview, name string) *overview.SoloCustomer {
	for i := range o.Customers {
		if o.Customers[i].Customer.Name == name {
			return &o.Customers[i]
		}
	}
	return nil
}

func TestSolo_DedicationFollowsTheContractPeriod(t *testing.T) {
	f := newSolo(t, dedicationNow, "America/Sao_Paulo")
	aurora := f.monthlyClosing("Aurora", 31, "2026-01-01", 40)
	pixel := f.monthlyClosing("Pixel", 20, "2026-01-01", 10)
	// O contrato começa no dia 12 e fecha no dia 10: o único período dele vai de 11/11 a 10/12, termina em dezembro e não
	// tem horas, recebimento nem valor projetado em novembro. A linha fica pela dedicação.
	f.monthlyClosing("Novo", 10, "2026-11-12", 20)
	alfa := f.hourly("Alfa")
	f.session(f.task(aurora, "Site"), "2026-11-03T13:00:00Z", "2026-11-03T17:00:00Z", nil) // 4h em novembro
	f.session(f.task(aurora, "App"), "2026-10-10T13:00:00Z", "2026-10-10T15:00:00Z", nil)  // 2h em outubro
	f.session(f.task(pixel, "Loja"), "2026-10-25T13:00:00Z", "2026-10-25T15:00:00Z", nil)  // 2h, no período 21/10 a 20/11
	f.session(f.task(alfa, "Alfa"), "2026-11-04T13:00:00Z", "2026-11-04T14:00:00Z", cents(10000))

	month := f.solo(overview.WindowMonth)
	checkSums(t, month)
	if d := rowOf(month, "Aurora").Dedication; d == nil || d.PeriodStart != "2026-11-01" || d.PeriodEnd != "2026-11-30" ||
		roundHours(d.Seconds) != 4 || d.Percent != 10 || len(d.Projects) != 2 {
		t.Errorf("Aurora this month = %+v, want November with 4h of 40h and both projects", d)
	}
	if d := rowOf(month, "Pixel").Dedication; d == nil || d.PeriodStart != "2026-10-21" || d.PeriodEnd != "2026-11-20" ||
		roundHours(d.Seconds) != 2 || d.Percent != 20 {
		t.Errorf("Pixel this month = %+v, want 21/10 to 20/11 with 2h of 10h", d)
	}
	if row := rowOf(month, "Novo"); row == nil || row.Dedication == nil || row.Dedication.Seconds != 0 || row.ProjectedCents != 0 {
		t.Errorf("Novo this month = %+v, want the row kept by its dedication, with no hours", row)
	}
	if d := rowOf(month, "Alfa").Dedication; d != nil {
		t.Errorf("an hourly customer has a dedication: %+v", d)
	}
	// A coluna Horas segue a janela de calendário: o Pixel tem 2h no período do contrato e nenhuma em novembro.
	if row := rowOf(month, "Pixel"); row.Seconds != 0 {
		t.Errorf("Pixel hours in November = %.0fs, want 0", row.Seconds)
	}

	last := f.solo(overview.WindowLastMonth)
	checkSums(t, last)
	if d := rowOf(last, "Aurora").Dedication; d == nil || d.PeriodStart != "2026-10-01" || roundHours(d.Seconds) != 2 {
		t.Errorf("Aurora last month = %+v, want October with 2h", d)
	}
	if d := rowOf(last, "Pixel").Dedication; d == nil || d.PeriodStart != "2026-09-21" || d.PeriodEnd != "2026-10-20" || d.Seconds != 0 {
		t.Errorf("Pixel last month = %+v, want 21/9 to 20/10 with no hours", d)
	}
	if row := rowOf(last, "Novo"); row != nil {
		t.Errorf("Novo last month = %+v, want no row: the contract had not started", row)
	}

	for _, row := range f.solo(overview.WindowYear).Customers {
		if row.Dedication != nil {
			t.Errorf("year window: %s has a dedication", row.Customer.Name)
		}
	}

	list, err := f.svc.SoloDedications(f.orgID.String())
	if err != nil || len(list) != 3 || list[0].Customer.Name != "Aurora" || list[1].Customer.Name != "Novo" || list[2].Customer.Name != "Pixel" {
		t.Errorf("SoloDedications = %+v, %v; want Aurora, Novo and Pixel, by name", list, err)
	}
}

// projectOverview monta a visão geral do projeto da tarefa com todos os domínios, sobre o relógio e os recebíveis do
// cenário solo.
func (f *soloFixture) projectOverview(taskID uuid.UUID) *overview.Overview {
	f.t.Helper()
	taskStore := task.NewStore(testClient)
	allocationStore := allocation.NewStore(testClient)
	svc := overview.NewService(overview.Deps{
		Projects:      project.NewService(project.NewStore(testClient)),
		Collaborators: collaborator.NewService(collaborator.NewStore(testClient)),
		Teams:         team.NewService(team.NewStore(testClient)),
		Sessions:      work_session.NewService(work_session.NewStore(testClient), taskStore, allocationStore),
		Integrations:  integration.NewService(integration.NewStore(testClient), "test-key"),
		Tasks:         taskStore,
		People:        person.NewService(person.NewStore(testClient)),
		Orgs:          organization.NewService(organization.NewStore(testClient)),
		Customers:     customer.NewService(customer.NewStore(testClient)),
		Receivables:   f.rec,
		Now:           func() time.Time { return f.now },
	})
	projectID := testClient.Task.GetX(context.Background(), taskID).ProjectID
	o, err := svc.Get(projectID.String())
	if err != nil {
		f.t.Fatalf("project overview: %v", err)
	}
	return o
}

func TestProjectOverview_ShowsTheCustomerDedication(t *testing.T) {
	f := newSolo(t, dedicationNow, "America/Sao_Paulo")
	aurora := f.monthlyClosing("Aurora", 31, "2026-01-01", 40)
	site := f.task(aurora, "Site")
	app := f.task(aurora, "App")
	f.session(site, "2026-11-03T13:00:00Z", "2026-11-03T17:00:00Z", nil)
	f.session(app, "2026-11-04T13:00:00Z", "2026-11-04T15:00:00Z", nil)

	o := f.projectOverview(site)
	if o.Dedication == nil || roundHours(o.Dedication.Seconds) != 6 || roundHours(o.Dedication.ProjectSeconds) != 4 ||
		o.Dedication.PeriodStart != "2026-11-01" || o.Dedication.Percent != 15 {
		t.Fatalf("Site dedication = %+v, want 6h of 40h for the customer and 4h for this project", o.Dedication)
	}
	o.HideBilling()
	if o.Dedication != nil {
		t.Error("HideBilling kept the dedication")
	}

	// O projeto do cliente por hora, o do mensal sem estimativa e o interno não têm dedicação.
	for name, taskID := range map[string]uuid.UUID{
		"hourly":      f.task(f.hourly("Alfa"), "Alfa"),
		"no estimate": f.task(f.monthly("Beta", 300000, "2026-01-01"), "Beta"),
		"internal":    f.task(nil, "Interno"),
	} {
		if o := f.projectOverview(taskID); o.Dedication != nil {
			t.Errorf("%s project has a dedication: %+v", name, o.Dedication)
		}
	}
}
