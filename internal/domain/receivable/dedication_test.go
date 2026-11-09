package receivable_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/receivable"
)

// A dedicação aos contratos mensais com estimativa de horas (Decisão 26): as horas de um período do contrato, em todos
// os projetos do cliente, contra a estimativa. O relógio é o do cenário (sexta 9/10/2026, meio-dia em São Paulo).

// estimate dá ao cliente a estimativa de horas por mês.
func (f *fixture) estimate(c *ent.Customer, hours int) *ent.Customer {
	f.t.Helper()
	return c.Update().SetEstimatedMonthlyHours(hours).SaveX(context.Background())
}

func (f *fixture) dedications(previous bool) map[string]receivable.Dedication {
	f.t.Helper()
	list, err := f.svc.Dedications(f.orgID.String(), previous)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string]receivable.Dedication{}
	for _, d := range list {
		out[d.Customer.Name] = d
	}
	return out
}

// day é um dia opcional, para o fim do contrato.
func day(v string) *string { return &v }

func hoursOf(seconds float64) float64 { return math.Round(seconds/36) / 100 }

func TestDedications_CurrentAndPreviousPeriod(t *testing.T) {
	f := setup(t)
	aurora := f.estimate(f.monthly("Aurora", "2026-01-01", nil), 40)
	site := f.task(aurora, "Site")
	app := f.task(aurora, "App")
	// Outubro: 4h no Site, 2h no App, a hora de outubro da sessão que atravessa a meia-noite de 30/9 e a sessão aberta
	// desde as 11h (uma hora até o meio-dia). Setembro: 3h e a hora de setembro da sessão da virada.
	f.session(site, "2026-10-01T13:00:00Z", "2026-10-01T17:00:00Z", nil)
	f.session(app, "2026-10-05T13:00:00Z", "2026-10-05T15:00:00Z", nil)
	f.session(site, "2026-10-01T02:00:00Z", "2026-10-01T04:00:00Z", nil)
	f.session(app, "2026-10-09T14:00:00Z", "", nil)
	f.session(site, "2026-09-10T13:00:00Z", "2026-09-10T16:00:00Z", nil)

	// Sem estimativa, ou por hora, não há o que acompanhar.
	plain := f.monthly("Sem estimativa", "2026-01-01", nil)
	f.session(f.task(plain, "P"), "2026-10-02T13:00:00Z", "2026-10-02T14:00:00Z", nil)
	f.session(f.task(f.hourly("Hora"), "H"), "2026-10-02T13:00:00Z", "2026-10-02T14:00:00Z", rate(15000))

	cur := f.dedications(false)
	if len(cur) != 1 {
		t.Fatalf("current dedications = %v, want only Aurora", cur)
	}
	d := cur["Aurora"]
	if d.PeriodStart != "2026-10-01" || d.PeriodEnd != "2026-10-31" || !d.Open || d.EstimatedHours != 40 ||
		hoursOf(d.Seconds) != 8 || d.Percent != 20 {
		t.Errorf("current = %+v, want October, open, 8h of 40h (20%%)", d)
	}
	if len(d.Projects) != 2 || d.Projects[0].Name != "App" || d.Projects[1].Name != "Site" ||
		hoursOf(d.Projects[0].Seconds) != 3 || hoursOf(d.Projects[1].Seconds) != 5 {
		t.Errorf("projects = %+v, want App 3h and Site 5h, by name", d.Projects)
	}

	prev := f.dedications(true)["Aurora"]
	if prev.PeriodStart != "2026-09-01" || prev.PeriodEnd != "2026-09-30" || prev.Open || hoursOf(prev.Seconds) != 4 || prev.Percent != 10 {
		t.Errorf("previous = %+v, want September, closed, 4h of 40h (10%%)", prev)
	}

	// Passar da estimativa passa de 100%.
	f.session(site, "2026-10-06T10:00:00Z", "2026-10-07T20:00:00Z", nil) // 34h
	if over := f.dedications(false)["Aurora"]; over.Percent != 105 {
		t.Errorf("over the estimate: percent = %d, want 105 (42h of 40h)", over.Percent)
	}
}

// O período é o do contrato: o cliente que fecha no dia 25 está, em 9/10, no período de 26/9 a 25/10.
func TestDedications_FollowTheClosingDay(t *testing.T) {
	f := setup(t)
	c := testClient.Customer.Create().SetOrganizationID(f.orgID).SetName("Pixel").
		SetBillingMode("monthly").SetMonthlyFeeCents(300000).SetContractStart("2026-01-01").SetClosingDay(25).
		SetEstimatedMonthlyHours(20).SaveX(context.Background())
	task := f.task(c, "Site")
	f.session(task, "2026-09-27T13:00:00Z", "2026-09-27T15:00:00Z", nil) // 2h, dentro
	f.session(task, "2026-09-25T13:00:00Z", "2026-09-25T15:00:00Z", nil) // 2h, no período anterior

	d := f.dedications(false)["Pixel"]
	if d.PeriodStart != "2026-09-26" || d.PeriodEnd != "2026-10-25" || hoursOf(d.Seconds) != 2 || d.Percent != 10 {
		t.Errorf("current = %+v, want 26/9 to 25/10 with 2h (10%%)", d)
	}
	if prev := f.dedications(true)["Pixel"]; prev.PeriodStart != "2026-08-26" || prev.PeriodEnd != "2026-09-25" || hoursOf(prev.Seconds) != 2 {
		t.Errorf("previous = %+v, want 26/8 to 25/9 with 2h", prev)
	}
}

// Só os períodos do contrato têm dedicação: nada antes do início nem depois do fim, e o período que contém o fim ainda
// conta.
func TestDedications_OnlyWithinTheContract(t *testing.T) {
	f := setup(t)
	f.estimate(f.monthly("Futuro", "2026-11-01", nil), 40)
	f.estimate(f.monthly("Fim em outubro", "2026-01-01", day("2026-10-05")), 40)
	f.estimate(f.monthly("Fim em setembro", "2026-01-01", day("2026-09-15")), 40)
	f.estimate(f.monthly("Começa hoje", "2026-10-09", nil), 40)

	cur := f.dedications(false)
	if _, ok := cur["Futuro"]; ok {
		t.Error("a contract that starts next month has a dedication now")
	}
	if _, ok := cur["Fim em outubro"]; !ok {
		t.Error("the period that holds the contract end has no dedication")
	}
	if _, ok := cur["Fim em setembro"]; ok {
		t.Error("a contract that ended last month has a dedication now")
	}
	if _, ok := cur["Começa hoje"]; !ok {
		t.Error("a contract that starts in the current period has no dedication")
	}
	prev := f.dedications(true)
	if _, ok := prev["Fim em setembro"]; !ok {
		t.Error("the last period of an ended contract has no dedication in the previous window")
	}
	if _, ok := prev["Começa hoje"]; ok {
		t.Error("the period before the contract start has a dedication")
	}
}

func TestDedication_OneCustomer(t *testing.T) {
	f := setup(t)
	aurora := f.estimate(f.monthly("Aurora", "2026-01-01", nil), 40)
	f.session(f.task(aurora, "Site"), "2026-10-01T13:00:00Z", "2026-10-01T15:00:00Z", nil)
	d, err := f.svc.Dedication(f.orgID.String(), aurora.ID.String())
	if err != nil || d == nil || hoursOf(d.Seconds) != 2 || d.PeriodStart != "2026-10-01" {
		t.Fatalf("Dedication = %+v, %v; want October with 2h", d, err)
	}
	if d, err := f.svc.Dedication(f.orgID.String(), f.hourly("Hora").ID.String()); err != nil || d != nil {
		t.Errorf("hourly customer = %+v, %v; want nil", d, err)
	}
	if d, err := f.svc.Dedication(f.orgID.String(), f.monthly("Sem", "2026-01-01", nil).ID.String()); err != nil || d != nil {
		t.Errorf("monthly without an estimate = %+v, %v; want nil", d, err)
	}

	otherOrg, _ := f.newOrg("Outra", true)
	other := testClient.Customer.Create().SetOrganizationID(otherOrg).SetName("Alheio").SetBillingMode("monthly").
		SetMonthlyFeeCents(100000).SetContractStart("2026-01-01").SetEstimatedMonthlyHours(10).SaveX(context.Background())
	if _, err := f.svc.Dedication(f.orgID.String(), other.ID.String()); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("another org's customer: err = %v, want not found", err)
	}
	if _, err := f.svc.Dedication(f.orgID.String(), "nope"); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("malformed id: err = %v, want not found", err)
	}
}

// Na lista Em andamento, a conta mensal traz a dedicação com as horas de agora, também quando o mês foi ajustado
// adiantado (o recebível gravado guarda as horas da gravação). A do por hora e as de A receber não trazem.
func TestSummary_InProgressMonthlyCarriesTheLiveDedication(t *testing.T) {
	f := setup(t)
	aurora := f.estimate(f.monthly("Aurora", "2026-09-01", nil), 40)
	task := f.task(aurora, "Site")
	f.session(task, "2026-10-01T13:00:00Z", "2026-10-01T15:00:00Z", nil)
	f.session(task, "2026-09-10T13:00:00Z", "2026-09-10T15:00:00Z", nil)
	f.session(f.task(f.hourly("Hora"), "H"), "2026-10-02T13:00:00Z", "2026-10-02T14:00:00Z", rate(15000))

	// O mês corrente ajustado adiantado; depois dele, mais 3h.
	f.confirm(aurora, "2026-10-01", "2026-10-31", 450000)
	f.session(task, "2026-10-06T13:00:00Z", "2026-10-06T16:00:00Z", nil)

	s, err := f.svc.Summary(f.orgID.String())
	if err != nil {
		t.Fatal(err)
	}
	var monthly, hourly *receivable.Due
	for i := range s.InProgress {
		switch s.InProgress[i].Customer.Name {
		case "Aurora":
			monthly = &s.InProgress[i]
		case "Hora":
			hourly = &s.InProgress[i]
		}
	}
	if monthly == nil || !monthly.Adjusted || monthly.Dedication == nil {
		t.Fatalf("Aurora in progress = %+v, want the adjusted month with a dedication", monthly)
	}
	if hoursOf(monthly.Seconds) != 2 || hoursOf(monthly.Dedication.Seconds) != 5 || monthly.Dedication.Percent != 13 {
		t.Errorf("adjusted month: seconds %.2fh, dedication %.2fh (%d%%); want 2h stored and 5h live (13%%)",
			hoursOf(monthly.Seconds), hoursOf(monthly.Dedication.Seconds), monthly.Dedication.Percent)
	}
	if hourly == nil || hourly.Dedication != nil {
		t.Errorf("hourly in progress = %+v, want no dedication", hourly)
	}
	for _, d := range s.ToReceive {
		if d.Dedication != nil {
			t.Errorf("to receive %s %s: has a dedication", d.Customer.Name, d.PeriodStart)
		}
	}
}
