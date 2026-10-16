package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/testutil"
)

// O cenário: hoje é sexta 9/10/2026, meio-dia em São Paulo (UTC-3 o ano todo). Com a regra mensal do dia 5, o
// período corrente vai de 6/10 a 5/11 e o fechado anterior, de 6/9 a 5/10 (a fronteira é 6/10 à meia-noite de lá,
// 03:00 UTC).
type fixture struct {
	t        *testing.T
	svc      *payment.Service
	people   *person.Service
	projects *project.Service
	orgID    string
	prj      uuid.UUID
	task     uuid.UUID
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func setup(t *testing.T) *fixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	now := at(t, "2026-10-09T15:00:00Z")
	orgs := organization.NewService(organization.NewStore(testClient))
	people := person.NewService(person.NewStore(testClient))
	projects := project.NewService(project.NewStore(testClient))
	taskStore := task.NewStore(testClient)
	allocStore := allocation.NewStore(testClient)
	org, err := orgs.Create("Org")
	if err != nil {
		t.Fatal(err)
	}
	prj, err := projects.Create(org.ID.String(), "Projeto X", "", 0, project.Routine{})
	if err != nil {
		t.Fatal(err)
	}
	tk := testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(context.Background())
	return &fixture{
		t: t, people: people, projects: projects, orgID: org.ID.String(), prj: prj.ID, task: tk.ID,
		svc: payment.NewService(payment.Deps{
			Orgs: orgs, People: people, Projects: projects,
			Sessions: work_session.NewService(work_session.NewStore(testClient), taskStore, allocStore),
			Payouts:  payment.NewStore(testClient),
			Now:      func() time.Time { return now },
		}),
	}
}

func (f *fixture) person(name string, rule person.PaymentRule) uuid.UUID {
	f.t.Helper()
	p, err := f.people.Create(f.orgID, name, name+"@test.com")
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.people.SetPayment(p.ID.String(), rule); err != nil {
		f.t.Fatal(err)
	}
	return p.ID
}

func (f *fixture) session(who uuid.UUID, start, end string, rate int) {
	f.t.Helper()
	var e *time.Time
	if end != "" {
		v := at(f.t, end)
		e = &v
	}
	testutil.Session(f.t, testClient, f.task, who, at(f.t, start), e, &rate, nil)
}

// project cria outro projeto na organização e devolve uma tarefa dele, para as sessões que não são do Projeto X.
func (f *fixture) project(name string) uuid.UUID {
	f.t.Helper()
	prj, err := f.projects.Create(f.orgID, name, "", 0, project.Routine{})
	if err != nil {
		f.t.Fatal(err)
	}
	return testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(context.Background()).ID
}

// sessionIn é session numa tarefa de outro projeto.
func (f *fixture) sessionIn(task, who uuid.UUID, start, end string, rate int) {
	f.t.Helper()
	e := at(f.t, end)
	testutil.Session(f.t, testClient, task, who, at(f.t, start), &e, &rate, nil)
}

func monthly(day int) person.PaymentRule {
	return person.PaymentRule{Frequency: person.PaymentMonthly, Day: day}
}

func (f *fixture) summary(who uuid.UUID) *payment.Summary {
	f.t.Helper()
	s, err := f.svc.Person(f.orgID, who.String(), 0, false)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestPerson_PeriodsAndAmounts(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", monthly(5))
	bruno := f.person("bruno", monthly(5))
	f.session(ana, "2026-10-06T01:00:00Z", "2026-10-06T05:00:00Z", 6000) // atravessa a fronteira: 2h de cada lado
	f.session(ana, "2026-10-08T12:00:00Z", "2026-10-08T13:00:00Z", 6000) // 1h no corrente
	f.session(ana, "2026-10-09T14:00:00Z", "", 6000)                     // aberta: 1h até agora
	f.session(ana, "2026-09-20T10:00:00Z", "2026-09-20T10:30:00Z", 6000) // 30min no fechado
	f.session(ana, "2026-09-25T10:00:00Z", "2026-09-25T10:00:20Z", 1000) // 20s a 10,00/h = 5,55c -> 6c
	f.session(bruno, "2026-10-08T12:00:00Z", "2026-10-08T20:00:00Z", 9999)

	s := f.summary(ana)
	if s.Timezone != "America/Sao_Paulo" || s.Owner || s.Rule == nil {
		t.Fatalf("summary header = %+v", s)
	}
	cur := s.Current
	if cur == nil || cur.Start != "2026-10-06" || cur.PayDate != "2026-11-05" {
		t.Fatalf("current = %+v", cur)
	}
	if cur.Seconds != 4*3600 || cur.AmountCents == nil || *cur.AmountCents != 24000 {
		t.Errorf("current = %vs / %v cents, want 14400s / 24000 (the open session counts up to now, the crossing one only its 2h)", cur.Seconds, cur.AmountCents)
	}
	if cur.DaysLeft == nil || *cur.DaysLeft != 27 {
		t.Errorf("days left = %v, want 27", cur.DaysLeft)
	}
	if len(cur.Projects) != 1 || cur.Projects[0].Project.Name != "Projeto X" || cur.Projects[0].Seconds != 4*3600 {
		t.Errorf("projects = %+v", cur.Projects)
	}
	if len(s.History) != 12 || s.History[0].Start != "2026-09-06" || s.History[0].PayDate != "2026-10-05" {
		t.Fatalf("history[0] = %+v (len %d)", s.History[0], len(s.History))
	}
	h := s.History[0]
	if h.Seconds != 7200+1800+20 || h.AmountCents == nil || *h.AmountCents != 12000+3000+6 {
		t.Errorf("history[0] = %vs / %v cents, want 9020s / 15006 (rounded per piece)", h.Seconds, h.AmountCents)
	}
	if s.Totals.AmountCents == nil || *s.Totals.AmountCents != 15006 || s.Totals.Seconds != 9020 {
		t.Errorf("totals = %+v", s.Totals)
	}
	if len(s.Upcoming) != 3 || s.Upcoming[0].PayDate != "2026-11-05" || s.Upcoming[2].PayDate != "2027-01-05" {
		t.Errorf("upcoming = %+v", s.Upcoming)
	}

	// Trocar a regra recalcula onde os períodos caem; as sessões e os valores são os mesmos.
	if _, err := f.people.SetPayment(ana.String(), person.PaymentRule{Frequency: person.PaymentBiweekly, Start: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	s = f.summary(ana)
	if s.Current.Start != "2026-10-01" || s.Current.PayDate != "2026-10-15" || s.Current.Seconds != 6*3600 || *s.Current.AmountCents != 36000 {
		t.Errorf("after the rule change current = %+v", s.Current)
	}
	if len(s.History) != 0 {
		t.Errorf("a biweekly rule starting 1/10 has no closed period, got %+v", s.History)
	}
}

func TestPerson_OwnerHasHoursAndNoAmount(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", monthly(5))
	testClient.Person.UpdateOneID(ana).SetIsOwner(true).ExecX(context.Background())
	f.session(ana, "2026-10-08T12:00:00Z", "2026-10-08T14:00:00Z", 6000)
	s := f.summary(ana)
	if !s.Owner || s.Current.Seconds != 7200 || s.Current.AmountCents != nil || s.Totals.AmountCents != nil {
		t.Errorf("owner summary = %+v current %+v", s, s.Current)
	}
}

func TestPerson_NoRuleAndGoal(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", person.PaymentRule{})
	if s := f.summary(ana); s.Rule != nil || s.Current != nil || len(s.History) != 0 || len(s.Upcoming) != 0 {
		t.Errorf("without a rule the summary should be empty: %+v", s)
	}
	if _, err := f.people.SetPayment(ana.String(), monthly(5)); err != nil {
		t.Fatal(err)
	}
	forty := 40
	testClient.Person.UpdateOneID(ana).SetWeeklyHours(forty).ExecX(context.Background())
	// 6/10 a 5/11 são 31 dias: 40h × 31 / 7.
	if g := f.summary(ana).Current.GoalSeconds; g == nil || *g != 40*3600*31/7.0 {
		t.Errorf("goal = %v", g)
	}
}

// O "desde o início" soma cada sessão inteira, com ou sem regra de pagamento: a que atravessa dois períodos entra uma
// vez (arredondada uma vez só), a anterior à janela do histórico também conta, e o detalhe por projeto bate com o total.
func TestPerson_Lifetime(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", monthly(5))
	other := f.project("Projeto Y")
	f.session(ana, "2026-10-06T01:00:00Z", "2026-10-06T05:00:00Z", 6000)          // 4h, atravessa a fronteira: 24000
	f.session(ana, "2026-10-09T14:00:00Z", "", 6000)                              // aberta: 1h até agora, 6000
	f.session(ana, "2024-01-10T12:00:00Z", "2024-01-10T12:00:20Z", 1000)          // muito antes do histórico: 20s, 6c
	f.sessionIn(other, ana, "2026-09-20T10:00:00Z", "2026-09-20T10:30:00Z", 8000) // 30min no outro projeto: 4000

	s := f.summary(ana)
	life := s.Lifetime
	if life.Seconds != 4*3600+3600+20+1800 || life.AmountCents == nil || *life.AmountCents != 24000+6000+6+4000 || life.FirstDay != "2024-01-10" {
		t.Errorf("lifetime = %vs / %v cents since %q, want 19820s / 34006 since 2024-01-10", life.Seconds, life.AmountCents, life.FirstDay)
	}
	// Do projeto com mais tempo para o com menos, e a soma das linhas é o total.
	if len(life.Projects) != 2 || life.Projects[0].Project.Name != "Projeto X" || life.Projects[1].Project.Name != "Projeto Y" {
		t.Fatalf("lifetime projects = %+v", life.Projects)
	}
	if x, y := life.Projects[0], life.Projects[1]; x.Seconds != 18020 || *x.AmountCents != 30006 || y.Seconds != 1800 || *y.AmountCents != 4000 {
		t.Errorf("lifetime by project = %vs/%v and %vs/%v, want 18020/30006 and 1800/4000", x.Seconds, *x.AmountCents, y.Seconds, *y.AmountCents)
	}
	// O histórico continua sendo só o dos períodos: a sessão de 2024 fica fora dos 12 últimos.
	if s.Totals.Seconds != 7200+1800 {
		t.Errorf("closed periods total = %vs, want 9000 (2h of the crossing session and the 30min of September)", s.Totals.Seconds)
	}
	if len(s.Current.Projects) != 1 || s.Current.Projects[0].Project.Name != "Projeto X" || s.Current.Projects[0].Seconds != 2*3600+3600 {
		t.Errorf("current by project = %+v, want only Projeto X with 3h", s.Current.Projects)
	}

	// Sem regra não há período, mas o que a pessoa fez continua aparecendo. O dia da primeira sessão é o do fuso da
	// organização: 01:00 UTC ainda é a véspera em São Paulo.
	bia := f.person("bia", person.PaymentRule{})
	f.session(bia, "2026-10-08T01:00:00Z", "2026-10-08T03:00:00Z", 5000)
	s = f.summary(bia)
	if s.Rule != nil || s.Current != nil || len(s.History) != 0 {
		t.Errorf("without a rule there are no periods: %+v", s)
	}
	if l := s.Lifetime; l.Seconds != 7200 || l.AmountCents == nil || *l.AmountCents != 10000 || l.FirstDay != "2026-10-07" || len(l.Projects) != 1 {
		t.Errorf("lifetime without a rule = %+v", l)
	}

	// O dono tem as horas e nenhum valor, também por projeto.
	testClient.Person.UpdateOneID(bia).SetIsOwner(true).ExecX(context.Background())
	if l := f.summary(bia).Lifetime; l.Seconds != 7200 || l.AmountCents != nil || len(l.Projects) != 1 || l.Projects[0].AmountCents != nil {
		t.Errorf("the owner's lifetime = %+v, want the hours and no amount", l)
	}

	// Quem nunca bateu ponto: zero, sem dia de início e com a lista vazia (nunca nula).
	caio := f.person("caio", person.PaymentRule{})
	if l := f.summary(caio).Lifetime; l.Seconds != 0 || l.AmountCents == nil || *l.AmountCents != 0 || l.FirstDay != "" || l.Projects == nil || len(l.Projects) != 0 {
		t.Errorf("lifetime of who never clocked in = %+v", l)
	}
}

func TestPerson_OtherOrganizationIsNotFound(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", monthly(5))
	other, _ := organization.NewService(organization.NewStore(testClient)).Create("Outra")
	if _, err := f.svc.Person(other.ID.String(), ana.String(), 0, false); err == nil {
		t.Error("a person from another organization should not be found")
	}
}

func TestTeam_TotalsAndOrder(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", monthly(5)) // o dono, com regra: só as horas contam
	testClient.Person.UpdateOneID(ana).SetIsOwner(true).ExecX(context.Background())
	zeca := f.person("zeca", monthly(10)) // paga em 10/10: fecha em 1 dia
	bia := f.person("bia", monthly(10))
	f.person("livia", person.PaymentRule{})
	f.session(zeca, "2026-10-08T12:00:00Z", "2026-10-08T14:00:00Z", 5000) // 2h = 10000 (período 11/9 a 10/10)
	f.session(bia, "2026-10-08T12:00:00Z", "2026-10-08T20:00:00Z", 5000)  // 8h = 40000
	f.session(ana, "2026-10-08T12:00:00Z", "2026-10-08T15:00:00Z", 5000)  // 3h do dono, sem valor

	team, err := f.svc.Team(f.orgID)
	if err != nil {
		t.Fatal(err)
	}
	// Por data de pagamento e, no empate, pelo nome: bia (8h) e zeca (2h) empatam em 10/10. O dono não é pago e fica
	// fora das linhas.
	if len(team.Due) != 2 || team.Due[0].Person.Name != "bia" || team.Due[1].Person.Name != "zeca" {
		t.Fatalf("due = %+v, want bia and zeca", team.Due)
	}
	if z := team.Due[1]; !z.Running || z.Overdue || z.DaysLeft != 1 || z.AmountCents != 10000 || z.Seconds != 7200 || z.PayDate != "2026-10-10" {
		t.Errorf("zeca's row = %+v", z)
	}
	tot := team.Totals
	if tot.ToPayCents != 50000 || tot.Closing7Cents != 50000 || tot.Closing30Cents != 50000 || tot.OverdueCents != 0 {
		t.Errorf("to pay %d, closing 7d %d, 30d %d, overdue %d; want 50000, 50000, 50000, 0", tot.ToPayCents, tot.Closing7Cents, tot.Closing30Cents, tot.OverdueCents)
	}
	if tot.Seconds != 13*3600 || tot.OwnerSeconds != 3*3600 || tot.WithoutRule != 1 || len(team.WithoutRule) != 1 || team.WithoutRule[0].Name != "livia" {
		t.Errorf("totals = %+v, without rule %+v", tot, team.WithoutRule)
	}
	if len(team.Paid) != 0 {
		t.Errorf("paid = %+v, want none", team.Paid)
	}
}

// O dono nunca tem regra (ele recebe do cliente), e isso não é uma pendência: ele não entra em "sem regra".
func TestTeam_OwnerWithoutRuleIsNotMissing(t *testing.T) {
	f := setup(t)
	ana := f.person("ana", person.PaymentRule{})
	testClient.Person.UpdateOneID(ana).SetIsOwner(true).ExecX(context.Background())
	f.person("livia", person.PaymentRule{})
	f.person("zeca", monthly(10))

	team, err := f.svc.Team(f.orgID)
	if err != nil {
		t.Fatal(err)
	}
	if team.Totals.WithoutRule != 1 || len(team.WithoutRule) != 1 || team.WithoutRule[0].Name != "livia" {
		t.Errorf("without rule = %+v, want only livia", team.WithoutRule)
	}
	for _, d := range team.Due {
		if d.Person.IsOwner {
			t.Errorf("the owner has a row to pay: %+v", d)
		}
	}
}

// Um fechado sem baixa fica atrasado por até 60 dias; a baixa o tira da lista e o põe em Pagos recentemente, com o
// valor pago (que pode ser outro) e o calculado.
func TestTeam_OverdueAndPayouts(t *testing.T) {
	f := setup(t)
	zeca := f.person("zeca", monthly(5))                                  // corrente 6/10 a 5/11
	f.session(zeca, "2026-07-20T12:00:00Z", "2026-07-20T13:00:00Z", 5000) // paga em 5/8: 65 dias atrás, fora
	f.session(zeca, "2026-08-20T12:00:00Z", "2026-08-20T13:00:00Z", 5000) // paga em 5/9: atrasado, 5000
	f.session(zeca, "2026-09-20T12:00:00Z", "2026-09-20T14:00:00Z", 5000) // paga em 5/10: atrasado, 10000
	f.session(zeca, "2026-10-08T12:00:00Z", "2026-10-08T13:00:00Z", 5000) // corrente: 5000

	team, err := f.svc.Team(f.orgID)
	if err != nil {
		t.Fatal(err)
	}
	var dates []string
	for _, d := range team.Due {
		dates = append(dates, d.PayDate)
	}
	if len(team.Due) != 3 || dates[0] != "2026-09-05" || dates[1] != "2026-10-05" || dates[2] != "2026-11-05" {
		t.Fatalf("due pay dates = %v, want 2026-09-05, 2026-10-05, 2026-11-05", dates)
	}
	if d := team.Due[0]; !d.Overdue || d.Running || d.DaysLeft != -34 || d.AmountCents != 5000 {
		t.Errorf("the oldest overdue row = %+v", d)
	}
	if d := team.Due[2]; d.Overdue || !d.Running || d.LastClosed == nil || d.LastClosed.PayDate != "2026-10-05" || d.LastClosed.Paid != nil {
		t.Errorf("the running row = %+v", d)
	}
	if tot := team.Totals; tot.ToPayCents != 20000 || tot.OverdueCents != 15000 || tot.Closing7Cents != 15000 || tot.Closing30Cents != 20000 {
		t.Errorf("totals = %+v", tot)
	}

	// A baixa de 6/9 a 5/10 com outro valor e um bônus.
	paid, bonus := 12000, 3000
	po, err := f.svc.MarkPaid(f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-09-06", PayDate: "2026-10-05", AmountCents: &paid, BonusCents: &bonus})
	if err != nil {
		t.Fatal(err)
	}
	if po.Seconds != 7200 || po.CalculatedCents != 10000 || po.AmountCents != 12000 || po.BonusCents != 3000 {
		t.Errorf("payout = %+v, want 7200s, calculated 10000, paid 12000, bonus 3000", po)
	}
	if _, err := f.svc.MarkPaid(f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-09-06", PayDate: "2026-10-05", AmountCents: &paid}); !errors.Is(err, payment.ErrAlreadyPaid) {
		t.Errorf("a second payout of the same period: err = %v, want already_paid", err)
	}
	team, _ = f.svc.Team(f.orgID)
	if len(team.Due) != 2 || team.Due[1].LastClosed == nil || team.Due[1].LastClosed.Paid == nil || team.Due[1].LastClosed.Paid.AmountCents != 12000 {
		t.Errorf("after the payout due = %+v", team.Due)
	}
	if len(team.Paid) != 1 || team.Paid[0].Person.Name != "zeca" || team.Paid[0].AmountCents != 12000 || team.Paid[0].BonusCents != 3000 || team.Paid[0].LaterSeconds != 0 {
		t.Errorf("paid = %+v", team.Paid)
	}
	if s := f.summary(zeca); s.History[0].Paid == nil || s.History[0].Paid.AmountCents != 12000 || s.History[0].Paid.BonusCents != 3000 || s.History[1].Paid != nil {
		t.Errorf("the profile history should show the payout: %+v", s.History[:2])
	}

	// A baixa do corrente antes da data; o que ele registra depois aparece como "depois da baixa".
	five := 5000
	if early, err := f.svc.MarkPaid(f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &five}); err != nil {
		t.Fatal(err)
	} else if early.BonusCents != 0 {
		t.Errorf("a payout without a bonus has bonus %d, want 0", early.BonusCents)
	}
	f.session(zeca, "2026-10-09T13:00:00Z", "2026-10-09T14:00:00Z", 5000)
	team, _ = f.svc.Team(f.orgID)
	if len(team.Due) != 1 || team.Due[0].PayDate != "2026-09-05" {
		t.Errorf("only the oldest overdue should be left: %+v", team.Due)
	}
	var early *payment.PaidRow
	for i := range team.Paid {
		if team.Paid[i].PayDate == "2026-11-05" {
			early = &team.Paid[i]
		}
	}
	if early == nil || early.LaterSeconds != 3600 || early.LaterCents != 5000 {
		t.Errorf("the early payout = %+v, want 1h / 5000 after it", early)
	}
	if s := f.summary(zeca); s.Current.Paid == nil || s.Current.Paid.AmountCents != 5000 {
		t.Errorf("the current period should show the payout: %+v", s.Current.Paid)
	}

	// Desfazer devolve o período para a lista.
	if err := f.svc.Unmark(f.orgID, po.ID.String()); err != nil {
		t.Fatal(err)
	}
	if team, _ = f.svc.Team(f.orgID); len(team.Due) != 2 || team.Due[1].PayDate != "2026-10-05" {
		t.Errorf("after undo due = %+v", team.Due)
	}
	if err := f.svc.Unmark(f.orgID, po.ID.String()); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("undo twice: err = %v, want not found", err)
	}
	other, _ := organization.NewService(organization.NewStore(testClient)).Create("Outra")
	if err := f.svc.Unmark(other.ID.String(), team.Paid[0].ID.String()); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("undo from another organization: err = %v, want not found", err)
	}
}

func TestMarkPaid_Refusals(t *testing.T) {
	f := setup(t)
	zeca := f.person("zeca", monthly(5))
	ana := f.person("ana", monthly(5))
	testClient.Person.UpdateOneID(ana).SetIsOwner(true).ExecX(context.Background())
	livia := f.person("livia", person.PaymentRule{})
	amount, negative := 1000, -1
	other, _ := organization.NewService(organization.NewStore(testClient)).Create("Outra")

	cases := []struct {
		name string
		org  string
		in   payment.MarkInput
		want error
	}{
		{"not a period of the rule", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-07", PayDate: "2026-11-05", AmountCents: &amount}, payment.ErrNotAPeriod},
		{"a malformed date", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "06/10/2026", PayDate: "2026-11-05", AmountCents: &amount}, payment.ErrNotAPeriod},
		{"a period that has not started", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-11-06", PayDate: "2026-12-05", AmountCents: &amount}, payment.ErrPeriodNotStarted},
		{"no amount", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05"}, payment.ErrInvalidAmount},
		{"a negative amount", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &negative}, payment.ErrInvalidAmount},
		{"a negative bonus", f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &amount, BonusCents: &negative}, payment.ErrInvalidBonus},
		{"the owner", f.orgID, payment.MarkInput{PersonID: ana.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &amount}, payment.ErrOwnerNotPaid},
		{"no rule", f.orgID, payment.MarkInput{PersonID: livia.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &amount}, payment.ErrNoRule},
		{"someone from another organization", other.ID.String(), payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &amount}, database.ErrNotFound},
	}
	for _, c := range cases {
		if _, err := f.svc.MarkPaid(c.org, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	// Zero é um valor pago válido: o período sem horas.
	zero := 0
	if _, err := f.svc.MarkPaid(f.orgID, payment.MarkInput{PersonID: zeca.String(), Start: "2026-10-06", PayDate: "2026-11-05", AmountCents: &zero}); err != nil {
		t.Errorf("a zero payout: %v", err)
	}
}
