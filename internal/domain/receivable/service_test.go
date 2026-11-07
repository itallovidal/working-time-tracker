package receivable_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/receivable"
	"working-time-tracker/testutil"
)

// O cenário: hoje é sexta 9/10/2026, meio-dia em São Paulo (UTC-3 o ano todo). Os clientes fecham o período no último dia
// do mês (closing_day 31), então o período corrente é outubro e o último fechado é setembro (1/9 a 30/9, que termina
// às 03:00 UTC de 1/10).
const nowText = "2026-10-09T15:00:00Z"

type fixture struct {
	t      *testing.T
	svc    *receivable.Service
	orgID  uuid.UUID
	who    uuid.UUID // a pessoa que bate o ponto
	people *person.Service
	now    time.Time
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// newOrg cria uma organização solo, com a pessoa dela.
func (f *fixture) newOrg(name string, solo bool) (uuid.UUID, uuid.UUID) {
	f.t.Helper()
	ctx := context.Background()
	org := testClient.Organization.Create().SetName(name).SetSolo(solo).SaveX(ctx)
	p, err := f.people.Create(org.ID.String(), "Ana "+name, name+"@test.com")
	if err != nil {
		f.t.Fatal(err)
	}
	return org.ID, p.ID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	f := &fixture{t: t, people: person.NewService(person.NewStore(testClient)), now: at(t, nowText)}
	f.orgID, f.who = f.newOrg("Org", true)
	f.svc = receivable.NewService(receivable.Deps{Store: receivable.NewStore(testClient), Now: func() time.Time { return f.now }})
	return f
}

// hourly cria um cliente por hora que fecha no último dia do mês e dá 5 dias de prazo.
func (f *fixture) hourly(name string) *ent.Customer {
	f.t.Helper()
	return testClient.Customer.Create().SetOrganizationID(f.orgID).SetName(name).
		SetBillingMode("hourly").SetClosingDay(31).SetPaymentDays(5).SaveX(context.Background())
}

// monthly cria um cliente mensal de 5.000,00 que fecha no último dia do mês, dá 10 dias de prazo e cobra 2% de multa e 1% ao
// mês de juros, com o contrato começando em start.
func (f *fixture) monthly(name, start string, end *string) *ent.Customer {
	f.t.Helper()
	return testClient.Customer.Create().SetOrganizationID(f.orgID).SetName(name).
		SetBillingMode("monthly").SetMonthlyFeeCents(500000).SetContractStart(start).SetNillableContractEnd(end).
		SetClosingDay(31).SetPaymentDays(10).SetLateFeeBp(200).SetLateInterestBpMonth(100).SetGraceDays(0).
		SaveX(context.Background())
}

// task cria um projeto do cliente (ou interno, com cliente nulo) e devolve uma tarefa dele.
func (f *fixture) task(c *ent.Customer, name string) uuid.UUID {
	f.t.Helper()
	ctx := context.Background()
	create := testClient.Project.Create().SetOrganizationID(f.orgID).SetName(name)
	if c != nil {
		create = create.SetCustomerID(c.ID)
	}
	prj := create.SaveX(ctx)
	return testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(ctx).ID
}

// session grava uma sessão da pessoa na tarefa com o valor cobrado dado (nulo, para o projeto de um cliente mensal).
func (f *fixture) session(task uuid.UUID, start, end string, bill *int) {
	f.t.Helper()
	var e *time.Time
	if end != "" {
		v := at(f.t, end)
		e = &v
	}
	pay := 0
	testutil.Session(f.t, testClient, task, f.who, at(f.t, start), e, &pay, bill)
}

func rate(cents int) *int { return &cents }

func (f *fixture) proposals(includeOpen bool) []receivable.Proposal {
	f.t.Helper()
	out, err := f.svc.Proposals(f.orgID.String(), includeOpen)
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *fixture) confirm(c *ent.Customer, start, end string, amount int) *receivable.Receivable {
	f.t.Helper()
	r, err := f.svc.Confirm(f.orgID.String(), receivable.ConfirmInput{
		CustomerID: c.ID.String(), PeriodStart: start, PeriodEnd: end, AmountCents: &amount,
	})
	if err != nil {
		f.t.Fatalf("confirm %s..%s: %v", start, end, err)
	}
	return r
}

func wantCode(t *testing.T, err error, sentinel *apperr.Error, field string) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %s", err, sentinel.Code)
	}
	if field == "" {
		return
	}
	var coded *apperr.Error
	if !errors.As(err, &coded) || coded.Params["field"] != field {
		t.Errorf("error %v has params %v, want field %q", err, coded.Params, field)
	}
}

// As propostas do cliente por hora: os períodos fechados com tempo, em todos os projetos do cliente, cortados nas
// fronteiras no fuso da organização, arredondados por sessão ao centavo pelo valor cobrado congelado nela.
func TestProposals_Hourly(t *testing.T) {
	f := setup(t)
	acme, other := f.hourly("Acme"), f.hourly("Outro")
	p1, p2 := f.task(acme, "Site"), f.task(acme, "App")
	otherTask, internal := f.task(other, "Dele"), f.task(nil, "Interno")

	// Setembro: 2h a 100,00 e 30min a 120,00, em dois projetos do mesmo cliente.
	f.session(p1, "2026-09-10T12:00:00Z", "2026-09-10T14:00:00Z", rate(10000))
	f.session(p2, "2026-09-11T12:00:00Z", "2026-09-11T12:30:00Z", rate(12000))
	// Três sessões de 20s a 10,00/h: 5,55c cada, arredondada ao centavo por sessão, 6c, e não 17c da soma.
	for _, day := range []string{"12", "13", "14"} {
		f.session(p1, "2026-09-"+day+"T12:00:00Z", "2026-09-"+day+"T12:00:20Z", rate(1000))
	}
	// A sessão que atravessa a fronteira (31/8 23:00 a 1/9 01:00 em São Paulo): 1h em cada mês, a 60,00.
	f.session(p1, "2026-09-01T02:00:00Z", "2026-09-01T04:00:00Z", rate(6000))
	// O outro cliente e o projeto interno não entram na conta do Acme.
	f.session(otherTask, "2026-09-10T12:00:00Z", "2026-09-10T20:00:00Z", rate(99999))
	f.session(internal, "2026-09-10T12:00:00Z", "2026-09-10T20:00:00Z", nil)
	// Outubro, o período corrente: uma sessão fechada de 1h e uma aberta que começou há 1h.
	f.session(p1, "2026-10-05T12:00:00Z", "2026-10-05T13:00:00Z", rate(10000))
	f.session(p2, "2026-10-09T14:00:00Z", "", rate(10000))

	var acmeProposals []receivable.Proposal
	for _, p := range f.proposals(false) {
		if p.Customer.ID == acme.ID {
			acmeProposals = append(acmeProposals, p)
		}
	}
	if len(acmeProposals) != 2 {
		t.Fatalf("acme proposals = %+v, want August and September only (the open October is not proposed)", acmeProposals)
	}

	aug, sep := acmeProposals[0], acmeProposals[1]
	if aug.PeriodStart != "2026-08-01" || aug.PeriodEnd != "2026-08-31" || aug.Seconds != 3600 || aug.CalculatedCents != 6000 {
		t.Errorf("august = %+v, want 1h and 60,00 (the half of the session that crosses the boundary)", aug)
	}
	// Setembro: 2h + 30min + 3×20s + 1h = 3h30min + 60s.
	if sep.PeriodStart != "2026-09-01" || sep.PeriodEnd != "2026-09-30" || sep.Seconds != 7200+1800+60+3600 {
		t.Errorf("september = %+v", sep)
	}
	if want := 20000 + 6000 + 3*6 + 6000; sep.CalculatedCents != want {
		t.Errorf("september amount = %d, want %d (each session rounded to the cent on its own)", sep.CalculatedCents, want)
	}
	if sep.Kind != "hourly" || sep.Open || sep.DueDate != "2026-10-05" || aug.DueDate != "2026-09-05" {
		t.Errorf("september kind/open/due = %s/%v/%s, august due = %s, want hourly/false/2026-10-05 and 2026-09-05 (the end plus 5 days)", sep.Kind, sep.Open, sep.DueDate, aug.DueDate)
	}
	if sep.EffectiveRateCents == nil || sep.BelowMinRate {
		t.Errorf("september effective rate = %v below=%v", sep.EffectiveRateCents, sep.BelowMinRate)
	}

	// Com o período aberto: outubro entra com as horas até agora (1h fechada + 1h da sessão aberta), marcado Open.
	var october *receivable.Proposal
	for _, p := range f.proposals(true) {
		if p.Customer.ID == acme.ID && p.PeriodStart == "2026-10-01" {
			october = &p
		}
	}
	if october == nil || !october.Open || october.PeriodEnd != "2026-10-31" || october.Seconds != 7200 || october.CalculatedCents != 20000 {
		t.Errorf("open october = %+v, want open, 2h and 200,00 (the open session counts until now)", october)
	}
}

// Só os períodos com tempo viram proposta, e só o cliente por hora: um mês sem sessão, um cliente sem projeto e o cliente
// mensal sem contrato não propõem nada por hora.
func TestProposals_HourlySkipsEmptyPeriods(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	f.hourly("Sem projeto")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-06-10T12:00:00Z", "2026-06-10T13:00:00Z", rate(10000))
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T13:00:00Z", rate(10000))

	got := f.proposals(false)
	if len(got) != 2 || got[0].PeriodStart != "2026-06-01" || got[1].PeriodStart != "2026-09-01" {
		t.Fatalf("proposals = %+v, want June and September only (July and August have no time)", got)
	}
}

// No máximo 24 propostas por cliente, as dos períodos mais recentes.
func TestProposals_HourlyAtMost24(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	// Trinta meses, de abril de 2024 a setembro de 2026, com uma hora em cada um.
	for i := 0; i < 30; i++ {
		month := time.Date(2024, time.April+time.Month(i), 10, 12, 0, 0, 0, time.UTC)
		f.session(tk, month.Format(time.RFC3339), month.Add(time.Hour).Format(time.RFC3339), rate(10000))
	}
	got := f.proposals(false)
	if len(got) != receivable.MaxHourlyProposals {
		t.Fatalf("got %d proposals, want %d", len(got), receivable.MaxHourlyProposals)
	}
	if got[0].PeriodStart != "2024-10-01" || got[len(got)-1].PeriodStart != "2026-09-01" {
		t.Errorf("proposals run from %s to %s, want October 2024 to September 2026 (the most recent 24)", got[0].PeriodStart, got[len(got)-1].PeriodStart)
	}
}

// Um valor por hora efetivo abaixo do piso de 10,00 é um aviso na proposta, não um erro.
func TestProposals_BelowTheMinimumRateIsAWarning(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T13:00:00Z", rate(500)) // 5,00/h

	got := f.proposals(false)
	if len(got) != 1 || !got[0].BelowMinRate || got[0].EffectiveRateCents == nil || *got[0].EffectiveRateCents != 500 {
		t.Fatalf("proposals = %+v, want one with the warning and a 5,00 effective rate", got)
	}
}

// O contrato mensal propõe do período do início do contrato até o corrente e o seguinte, parando no fim do contrato.
func TestProposals_Monthly(t *testing.T) {
	f := setup(t)
	periods := func(c *ent.Customer) (starts []string) {
		for _, p := range f.proposals(false) {
			if p.Customer.ID == c.ID {
				starts = append(starts, p.PeriodStart)
			}
		}
		return starts
	}
	equal := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	open := f.monthly("Sem fim", "2026-08-15", nil)
	if got, want := periods(open), []string{"2026-08-01", "2026-09-01", "2026-10-01", "2026-11-01"}; !equal(got, want) {
		t.Errorf("open-ended contract = %v, want %v (the start's period through the current and the next)", got, want)
	}
	end := "2026-09-20"
	ended := f.monthly("Com fim", "2026-08-15", &end)
	if got, want := periods(ended), []string{"2026-08-01", "2026-09-01"}; !equal(got, want) {
		t.Errorf("contract ending on 2026-09-20 = %v, want %v", got, want)
	}
	endOct := "2026-10-01"
	endedOct := f.monthly("Fim no dia 1", "2026-10-01", &endOct)
	if got, want := periods(endedOct), []string{"2026-10-01"}; !equal(got, want) {
		t.Errorf("contract ending on the first day of October = %v, want %v (the period that holds the end is the last)", got, want)
	}
	future := f.monthly("Futuro", "2026-12-01", nil)
	if got := periods(future); len(got) != 0 {
		t.Errorf("a contract that starts after the next period = %v, want none", got)
	}
	noFee := testClient.Customer.Create().SetOrganizationID(f.orgID).SetName("Sem valor").SetBillingMode("monthly").
		SetContractStart("2026-08-01").SaveX(context.Background())
	if got := periods(noFee); len(got) != 0 {
		t.Errorf("a monthly customer without a fee = %v, want none", got)
	}

	for _, p := range f.proposals(false) {
		if p.Customer.ID != open.ID {
			continue
		}
		if p.CalculatedCents != 500000 || p.Kind != "monthly" {
			t.Errorf("proposal %s = %d / %s, want the monthly fee 5.000,00", p.PeriodStart, p.CalculatedCents, p.Kind)
		}
		wantOpen := p.PeriodStart >= "2026-10-01"
		if p.Open != wantOpen {
			t.Errorf("proposal %s open = %v, want %v", p.PeriodStart, p.Open, wantOpen)
		}
		if p.PeriodStart == "2026-09-01" && p.DueDate != "2026-10-10" {
			t.Errorf("september due date = %s, want 2026-10-10 (the end plus 10 days)", p.DueDate)
		}
	}
}

// As horas do contrato mensal contam (sem valor cobrado, as sessões trazem só o tempo), e dão o valor por hora efetivo.
func TestProposals_MonthlyHoursGiveTheEffectiveRate(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	tk := f.task(acme, "Retainer")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T22:00:00Z", nil) // 10h

	var sep *receivable.Proposal
	for _, p := range f.proposals(false) {
		if p.PeriodStart == "2026-09-01" {
			sep = &p
		}
	}
	if sep == nil || sep.Seconds != 36000 || sep.CalculatedCents != 500000 || sep.EffectiveRateCents == nil || *sep.EffectiveRateCents != 50000 {
		t.Errorf("september = %+v, want 10h, 5.000,00 and 500,00 per hour", sep)
	}
}

func TestConfirm_Hourly(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T14:00:00Z", rate(10000))
	org := f.orgID.String()

	// O período aberto é recusado: só se cobra o que já terminou.
	_, err := f.svc.Confirm(org, receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31", AmountCents: rate(1000)})
	wantCode(t, err, receivable.ErrPeriodOpen, "")

	// O dono ajusta o valor, o vencimento, a nota e a observação.
	amount := 19500
	r, err := f.svc.Confirm(org, receivable.ConfirmInput{
		CustomerID: acme.ID.String(), PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", AmountCents: &amount,
		DueDate: "2026-10-20", InvoiceNumber: " NF-1234 ", Note: " desconto combinado ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Kind != "hourly" || r.Seconds != 7200 || r.CalculatedCents != 20000 || r.AmountCents != 19500 || r.Currency != "BRL" ||
		r.DueDate != "2026-10-20" || r.InvoiceNumber != "NF-1234" || r.Note != "desconto combinado" || r.Customer.Name != "Acme" ||
		r.ReceivedOn != nil || r.Late {
		t.Errorf("confirmed = %+v", r)
	}
	if r.EffectiveRateCents == nil || *r.EffectiveRateCents != 9750 {
		t.Errorf("effective rate = %v, want 97,50 (the confirmed amount over the hours)", r.EffectiveRateCents)
	}

	// O mesmo período não confirma duas vezes.
	_, err = f.svc.Confirm(org, receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", AmountCents: &amount})
	wantCode(t, err, receivable.ErrAlreadyConfirmed, "")

	// Confirmado, o período sai das propostas; desfazer a confirmação o traz de volta.
	if got := f.proposals(false); len(got) != 0 {
		t.Errorf("proposals after the confirmation = %+v, want none", got)
	}
	if err := f.svc.Unconfirm(org, r.ID.String()); err != nil {
		t.Fatal(err)
	}
	if got := f.proposals(false); len(got) != 1 || got[0].PeriodStart != "2026-09-01" {
		t.Errorf("proposals after undoing the confirmation = %+v, want September back", got)
	}
}

// A confirmação grava o que o sistema calcula agora (as horas e o valor), o vencimento padrão e a moeda da organização,
// e o que a hora registrada depois não reescreve.
func TestConfirm_DefaultsAndSnapshot(t *testing.T) {
	f := setup(t)
	testClient.Organization.UpdateOneID(f.orgID).SetCurrency("USD").ExecX(context.Background())
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T13:00:00Z", rate(10000))

	r := f.confirm(acme, "2026-09-01", "2026-09-30", 10000)
	if r.DueDate != "2026-10-05" || r.Currency != "USD" || r.Seconds != 3600 || r.CalculatedCents != 10000 {
		t.Errorf("confirmed = %+v, want due 2026-10-05 (end + 5 days), USD, 1h and 100,00", r)
	}
	// Uma hora lançada depois, no mesmo período, não muda o que foi cobrado.
	f.session(tk, "2026-09-20T12:00:00Z", "2026-09-20T13:00:00Z", rate(10000))
	list, err := f.svc.Receivables(f.orgID.String())
	if err != nil || len(list) != 1 || list[0].Seconds != 3600 || list[0].AmountCents != 10000 {
		t.Errorf("receivables = %+v (%v), want the confirmation untouched", list, err)
	}
}

func TestConfirm_Validation(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	org := f.orgID.String()
	base := receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", AmountCents: rate(10000)}
	with := func(edit func(*receivable.ConfirmInput)) receivable.ConfirmInput {
		in := base
		edit(&in)
		return in
	}
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'a'
	}

	for _, c := range []struct {
		name  string
		in    receivable.ConfirmInput
		code  *apperr.Error
		field string
	}{
		{"no amount", with(func(in *receivable.ConfirmInput) { in.AmountCents = nil }), receivable.ErrInvalidAmount, "amount_cents"},
		{"zero amount", with(func(in *receivable.ConfirmInput) { in.AmountCents = rate(0) }), receivable.ErrInvalidAmount, "amount_cents"},
		{"negative amount", with(func(in *receivable.ConfirmInput) { in.AmountCents = rate(-5) }), receivable.ErrInvalidAmount, "amount_cents"},
		{"amount over the ceiling", with(func(in *receivable.ConfirmInput) { in.AmountCents = rate(100_000_001) }), receivable.ErrInvalidAmount, "amount_cents"},
		{"no period start", with(func(in *receivable.ConfirmInput) { in.PeriodStart = "" }), apperr.ErrFieldRequired, "period_start"},
		{"bad period end", with(func(in *receivable.ConfirmInput) { in.PeriodEnd = "30/09/2026" }), apperr.ErrFieldInvalid, "period_end"},
		{"not a period of the cycle", with(func(in *receivable.ConfirmInput) { in.PeriodStart = "2026-09-02" }), receivable.ErrNotAPeriod, "period_start"},
		{"two periods", with(func(in *receivable.ConfirmInput) { in.PeriodEnd = "2026-10-31" }), receivable.ErrNotAPeriod, "period_start"},
		{"bad due date", with(func(in *receivable.ConfirmInput) { in.DueDate = "amanhã" }), apperr.ErrFieldInvalid, "due_date"},
		{"invoice too long", with(func(in *receivable.ConfirmInput) { in.InvoiceNumber = string(long[:61]) }), apperr.ErrFieldTooLong, "invoice_number"},
		{"note too long", with(func(in *receivable.ConfirmInput) { in.Note = string(long) }), apperr.ErrFieldTooLong, "note"},
		{"malformed customer", with(func(in *receivable.ConfirmInput) { in.CustomerID = "nope" }), customer.ErrNotFound, ""},
		{"unknown customer", with(func(in *receivable.ConfirmInput) { in.CustomerID = uuid.NewString() }), customer.ErrNotFound, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.svc.Confirm(org, c.in)
			wantCode(t, err, c.code, c.field)
		})
	}
	if list, _ := f.svc.Receivables(org); len(list) != 0 {
		t.Errorf("a refused confirmation left %d receivables", len(list))
	}
}

// O mensal confirma a qualquer momento, inclusive adiantado, mas só dentro do contrato.
func TestConfirm_Monthly(t *testing.T) {
	f := setup(t)
	end := "2026-11-15"
	acme := f.monthly("Acme", "2026-08-15", &end)
	org := f.orgID.String()
	try := func(start, end string) error {
		_, err := f.svc.Confirm(org, receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: start, PeriodEnd: end, AmountCents: rate(500000)})
		return err
	}

	// O primeiro mês é parcial: o dono corrige o valor ao confirmar.
	partial := f.confirm(acme, "2026-08-01", "2026-08-31", 260000)
	if partial.Kind != "monthly" || partial.CalculatedCents != 500000 || partial.AmountCents != 260000 || partial.DueDate != "2026-09-10" {
		t.Errorf("partial first month = %+v", partial)
	}
	// O período corrente (aberto) e o seguinte (adiantado) se confirmam.
	if err := try("2026-10-01", "2026-10-31"); err != nil {
		t.Errorf("the current period: %v", err)
	}
	if err := try("2026-11-01", "2026-11-30"); err != nil {
		t.Errorf("the next period, in advance: %v", err)
	}
	// Fora do contrato: antes do início, depois do seguinte e depois do fim.
	wantCode(t, try("2026-07-01", "2026-07-31"), receivable.ErrNotAPeriod, "period_start")
	wantCode(t, try("2026-12-01", "2026-12-31"), receivable.ErrNotAPeriod, "period_start")
	wantCode(t, try("2026-08-01", "2026-08-31"), receivable.ErrAlreadyConfirmed, "")
}

// Um contrato que acabou antes do período seguinte não confirma o que vem depois do fim.
func TestConfirm_MonthlyStopsAtTheContractEnd(t *testing.T) {
	f := setup(t)
	end := "2026-09-20"
	acme := f.monthly("Acme", "2026-08-15", &end)
	_, err := f.svc.Confirm(f.orgID.String(), receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31", AmountCents: rate(500000)})
	wantCode(t, err, receivable.ErrNotAPeriod, "period_start")
}

func TestEdit(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)

	amount, due, invoice, note := 480000, "2026-10-15", "NF-7", "negociado"
	got, err := f.svc.Update(org, r.ID.String(), receivable.UpdateInput{AmountCents: &amount, DueDate: &due, InvoiceNumber: &invoice, Note: &note})
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountCents != 480000 || got.DueDate != "2026-10-15" || got.InvoiceNumber != "NF-7" || got.Note != "negociado" || got.CalculatedCents != 500000 {
		t.Errorf("edited = %+v, want the new fields and the calculated value untouched", got)
	}
	// Campo ausente mantém; texto vazio apaga.
	empty := ""
	got, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{InvoiceNumber: &empty})
	if err != nil || got.InvoiceNumber != "" || got.Note != "negociado" || got.AmountCents != 480000 {
		t.Errorf("clearing the invoice = %+v (%v)", got, err)
	}
	// Sem nenhum campo, nada muda.
	if got, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{}); err != nil || got.AmountCents != 480000 {
		t.Errorf("an empty edit = %+v (%v)", got, err)
	}

	zero, bad, blank := 0, "ontem", " "
	_, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{AmountCents: &zero})
	wantCode(t, err, receivable.ErrInvalidAmount, "amount_cents")
	_, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{DueDate: &bad})
	wantCode(t, err, apperr.ErrFieldInvalid, "due_date")
	_, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{DueDate: &blank})
	wantCode(t, err, apperr.ErrFieldRequired, "due_date")

	// Recebido, não se edita mais.
	if _, err := f.svc.Receive(org, r.ID.String(), receivable.ReceiveInput{}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.Update(org, r.ID.String(), receivable.UpdateInput{AmountCents: &amount})
	wantCode(t, err, receivable.ErrAlreadyReceived, "")
}

// No cliente por hora o valor recebido é o confirmado, e multa e juros são recusados.
func TestReceive_Hourly(t *testing.T) {
	f := setup(t)
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T14:00:00Z", rate(10000))
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 19500)
	id := r.ID.String()

	for _, c := range []struct {
		name  string
		in    receivable.ReceiveInput
		code  *apperr.Error
		field string
	}{
		{"another amount", receivable.ReceiveInput{ReceivedCents: rate(20000)}, receivable.ErrReceivedMustMatch, "received_cents"},
		{"a late fee", receivable.ReceiveInput{LateFeeCents: rate(100)}, receivable.ErrChargesNotAllowed, "late_fee_cents"},
		{"interest", receivable.ReceiveInput{InterestCents: rate(100)}, receivable.ErrChargesNotAllowed, "interest_cents"},
		{"a bad date", receivable.ReceiveInput{ReceivedOn: "hoje"}, apperr.ErrFieldInvalid, "received_on"},
		{"a date in the future", receivable.ReceiveInput{ReceivedOn: "2026-10-10"}, receivable.ErrReceivedInFuture, "received_on"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.svc.Receive(org, id, c.in)
			wantCode(t, err, c.code, c.field)
		})
	}

	// Sem nada no corpo vale hoje e o valor confirmado; zero em multa e juros não é uma cobrança.
	zero := 0
	got, err := f.svc.Receive(org, id, receivable.ReceiveInput{LateFeeCents: &zero, InterestCents: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if got.ReceivedOn == nil || *got.ReceivedOn != "2026-10-09" || got.ReceivedCents == nil || *got.ReceivedCents != 19500 || got.LateFeeCents != 0 || got.InterestCents != 0 {
		t.Errorf("received = %+v, want today and the confirmed 195,00", got)
	}
	_, err = f.svc.Receive(org, id, receivable.ReceiveInput{})
	wantCode(t, err, receivable.ErrAlreadyReceived, "")
}

// No mensal o valor recebido é livre, e a multa e os juros são partes dele.
func TestReceive_Monthly(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-08-01", nil)
	org := f.orgID.String()

	// O exemplo da especificação: 6.300,00 pagos 10 dias depois do vencimento, com a multa e os juros sugeridos.
	r := f.confirm(acme, "2026-08-01", "2026-08-31", 630000) // vence em 2026-09-10
	got, err := f.svc.Receive(org, r.ID.String(), receivable.ReceiveInput{
		ReceivedOn: "2026-09-20", ReceivedCents: rate(644700), LateFeeCents: rate(12600), InterestCents: rate(2100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if *got.ReceivedOn != "2026-09-20" || *got.ReceivedCents != 644700 || got.LateFeeCents != 12600 || got.InterestCents != 2100 || got.AmountCents != 630000 {
		t.Errorf("received = %+v", got)
	}

	// O valor recebido é livre: menos que o confirmado, e sem informar multa nem juros.
	r2 := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)
	got, err = f.svc.Receive(org, r2.ID.String(), receivable.ReceiveInput{ReceivedCents: rate(300000)})
	if err != nil || *got.ReceivedCents != 300000 || got.LateFeeCents != 0 || got.InterestCents != 0 {
		t.Errorf("a partial payment = %+v (%v)", got, err)
	}

	// Sem o valor recebido, vale o confirmado mais a multa e os juros.
	r3 := f.confirm(acme, "2026-10-01", "2026-10-31", 500000)
	got, err = f.svc.Receive(org, r3.ID.String(), receivable.ReceiveInput{LateFeeCents: rate(1000), InterestCents: rate(500)})
	if err != nil || *got.ReceivedCents != 501500 {
		t.Errorf("default received = %+v (%v), want 5.015,00", got, err)
	}
}

func TestReceive_MonthlyValidation(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)
	id := r.ID.String()

	for _, c := range []struct {
		name  string
		in    receivable.ReceiveInput
		code  *apperr.Error
		field string
	}{
		{"zero received", receivable.ReceiveInput{ReceivedCents: rate(0)}, receivable.ErrInvalidAmount, "received_cents"},
		{"negative received", receivable.ReceiveInput{ReceivedCents: rate(-1)}, receivable.ErrInvalidAmount, "received_cents"},
		{"received over the ceiling", receivable.ReceiveInput{ReceivedCents: rate(100_000_001)}, receivable.ErrInvalidAmount, "received_cents"},
		{"negative late fee", receivable.ReceiveInput{LateFeeCents: rate(-1)}, receivable.ErrInvalidCharge, "late_fee_cents"},
		{"negative interest", receivable.ReceiveInput{InterestCents: rate(-1)}, receivable.ErrInvalidCharge, "interest_cents"},
		{"charges over the received amount", receivable.ReceiveInput{ReceivedCents: rate(1000), LateFeeCents: rate(600), InterestCents: rate(600)}, receivable.ErrChargesExceedReceived, "late_fee_cents"},
		{"a date in the future", receivable.ReceiveInput{ReceivedOn: "2026-10-10"}, receivable.ErrReceivedInFuture, "received_on"},
		{"a day that does not exist", receivable.ReceiveInput{ReceivedOn: "2026-02-30"}, apperr.ErrFieldInvalid, "received_on"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := f.svc.Receive(org, id, c.in)
			wantCode(t, err, c.code, c.field)
		})
	}
	// Hoje vale, e é o limite.
	if _, err := f.svc.Receive(org, id, receivable.ReceiveInput{ReceivedOn: "2026-10-09"}); err != nil {
		t.Errorf("receiving today: %v", err)
	}
}

// Desfazer o recebimento leva a multa e os juros com ele; desfazer a confirmação só vale sem recebimento.
func TestUndo(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)
	id := r.ID.String()

	wantCode(t, f.svc.Unreceive(org, id), receivable.ErrNotReceived, "")
	if _, err := f.svc.Receive(org, id, receivable.ReceiveInput{LateFeeCents: rate(100), InterestCents: rate(50)}); err != nil {
		t.Fatal(err)
	}
	// Recebido, a confirmação não se desfaz: desfaça o recebimento antes.
	wantCode(t, f.svc.Unconfirm(org, id), receivable.ErrAlreadyReceived, "")
	if list, _ := f.svc.Receivables(org); len(list) != 1 {
		t.Fatalf("a refused undo removed the receivable: %d left", len(list))
	}

	if err := f.svc.Unreceive(org, id); err != nil {
		t.Fatal(err)
	}
	list, _ := f.svc.Receivables(org)
	if len(list) != 1 || list[0].ReceivedOn != nil || list[0].ReceivedCents != nil || list[0].LateFeeCents != 0 || list[0].InterestCents != 0 {
		t.Fatalf("after undoing the receipt = %+v, want it open again with no charges", list)
	}
	if err := f.svc.Unconfirm(org, id); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.svc.Receivables(org); len(list) != 0 {
		t.Errorf("after undoing the confirmation: %+v", list)
	}
	if err := f.svc.Unconfirm(org, id); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("undoing a confirmation that is gone = %v, want not found", err)
	}
}

// A sugestão: o exemplo da especificação, a tolerância e o recebível por hora.
func TestLateCharges(t *testing.T) {
	f := setup(t)
	org := f.orgID.String()
	acme := testClient.Customer.Create().SetOrganizationID(f.orgID).SetName("Acme").SetBillingMode("monthly").
		SetMonthlyFeeCents(630000).SetContractStart("2026-08-01").SetClosingDay(31).SetPaymentDays(5).
		SetLateFeeBp(200).SetLateInterestBpMonth(100).SetGraceDays(3).SaveX(context.Background())
	r := f.confirm(acme, "2026-08-01", "2026-08-31", 630000) // vence em 2026-09-05

	got, err := f.svc.LateCharges(org, r.ID.String(), "2026-09-15")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Charged || got.DaysLate != 10 || got.LateFeeCents != 12600 || got.InterestCents != 2100 || got.TotalCents != 644700 || got.GraceDays != 3 {
		t.Errorf("10 days late = %+v, want fee 126,00, interest 21,00, total 6.447,00", got)
	}
	// Dentro da tolerância de 3 dias, nada.
	if got, _ = f.svc.LateCharges(org, r.ID.String(), "2026-09-08"); got.Charged || got.LateFeeCents != 0 || got.InterestCents != 0 || got.TotalCents != 630000 {
		t.Errorf("inside the grace = %+v, want no charge", got)
	}
	if got, _ = f.svc.LateCharges(org, r.ID.String(), "2026-09-09"); !got.Charged || got.DaysLate != 4 {
		t.Errorf("one day past the grace = %+v, want charged for 4 days", got)
	}
	// Sem ?on= vale hoje (9/10, 34 dias depois do vencimento).
	if got, _ = f.svc.LateCharges(org, r.ID.String(), ""); got.On != "2026-10-09" || got.DaysLate != 34 {
		t.Errorf("default date = %+v", got)
	}
	_, err = f.svc.LateCharges(org, r.ID.String(), "amanhã")
	wantCode(t, err, apperr.ErrFieldInvalid, "on")

	// O recebível por hora não tem multa nem juros.
	hourly := f.hourly("Por hora")
	tk := f.task(hourly, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T13:00:00Z", rate(10000))
	h := f.confirm(hourly, "2026-09-01", "2026-09-30", 10000)
	if got, _ = f.svc.LateCharges(org, h.ID.String(), "2026-12-01"); got.Charged || got.LateFeeCents != 0 || got.InterestCents != 0 || got.TotalCents != 10000 {
		t.Errorf("an hourly receivable = %+v, want no charges", got)
	}
}

// "Atrasado" é derivado: confirmado, sem recebimento e com hoje depois do vencimento. Recebido, ou no dia do vencimento,
// não está atrasado.
func TestLate_IsDerived(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-07-01", nil)
	org := f.orgID.String()
	f.confirm(acme, "2026-07-01", "2026-07-31", 500000) // vence 10/8: atrasado
	paid := f.confirm(acme, "2026-08-01", "2026-08-31", 500000)
	if _, err := f.svc.Receive(org, paid.ID.String(), receivable.ReceiveInput{}); err != nil {
		t.Fatal(err)
	}
	f.confirm(acme, "2026-09-01", "2026-09-30", 500000) // vence 10/10: amanhã
	list, err := f.svc.Receivables(org)
	if err != nil || len(list) != 3 {
		t.Fatalf("receivables = %+v (%v)", list, err)
	}
	byStart := map[string]receivable.Receivable{}
	for _, r := range list {
		byStart[r.PeriodStart] = r
	}
	if r := byStart["2026-07-01"]; !r.Late || r.DaysLate != 60 {
		t.Errorf("july (due 2026-08-10) = late %v for %d days, want late for 60", r.Late, r.DaysLate)
	}
	if r := byStart["2026-08-01"]; r.Late {
		t.Error("a received receivable is not late")
	}
	if r := byStart["2026-09-01"]; r.Late || r.DaysLate != 0 {
		t.Errorf("september (due 2026-10-10) is late = %v, want not yet", r.Late)
	}

	// No dia do vencimento ainda não está atrasado; no dia seguinte está.
	f.now = at(t, "2026-10-10T20:00:00Z")
	list, _ = f.svc.Receivables(org)
	for _, r := range list {
		if r.PeriodStart == "2026-09-01" && r.Late {
			t.Error("on the due date the receivable is not late yet")
		}
	}
	f.now = at(t, "2026-10-11T20:00:00Z")
	list, _ = f.svc.Receivables(org)
	for _, r := range list {
		if r.PeriodStart == "2026-09-01" && (!r.Late || r.DaysLate != 1) {
			t.Errorf("the day after the due date: late %v for %d days, want late for 1", r.Late, r.DaysLate)
		}
	}
}

// Os cartões e as três listas da página.
func TestSummary(t *testing.T) {
	f := setup(t)
	org := f.orgID.String()
	hourly := f.hourly("Por hora")
	tk := f.task(hourly, "Site")
	f.session(tk, "2026-09-10T12:00:00Z", "2026-09-10T14:00:00Z", rate(10000)) // setembro: 200,00 a confirmar
	monthly := f.monthly("Mensal", "2026-07-01", nil)
	f.confirm(monthly, "2026-07-01", "2026-07-31", 500000)        // vence 10/8: atrasado
	jul := f.confirm(monthly, "2026-08-01", "2026-08-31", 500000) // vence 10/9: atrasado
	f.confirm(monthly, "2026-09-01", "2026-09-30", 500000)        // vence 10/10: em aberto
	if _, err := f.svc.Receive(org, jul.ID.String(), receivable.ReceiveInput{ReceivedOn: "2026-10-02", ReceivedCents: rate(510000)}); err != nil {
		t.Fatal(err)
	}
	old := f.confirm(hourly, "2026-09-01", "2026-09-30", 20000)
	if _, err := f.svc.Receive(org, old.ID.String(), receivable.ReceiveInput{ReceivedOn: "2026-09-30"}); err != nil {
		t.Fatal(err)
	}

	s, err := f.svc.Summary(org)
	if err != nil {
		t.Fatal(err)
	}
	if s.Today != "2026-10-09" || s.Timezone != "America/Sao_Paulo" || s.Currency != "BRL" {
		t.Errorf("header = %s / %s / %s", s.Today, s.Timezone, s.Currency)
	}
	// A confirmar: as mensalidades de outubro (corrente) e de novembro (seguinte); setembro por hora já foi confirmado.
	if len(s.ToConfirm) != 2 || s.ToConfirm[0].PeriodStart != "2026-10-01" || s.ToConfirm[1].PeriodStart != "2026-11-01" {
		t.Fatalf("to confirm = %+v", s.ToConfirm)
	}
	want := receivable.Totals{
		ToConfirmCount: 2, ToConfirmCents: 1000000,
		OpenCount: 2, OpenCents: 1000000, LateCount: 1, LateCents: 500000,
		ReceivedMonthCount: 1, ReceivedMonthCents: 510000,
	}
	if s.Totals != want {
		t.Errorf("totals = %+v, want %+v", s.Totals, want)
	}
	if len(s.Open) != 2 || s.Open[0].PeriodStart != "2026-07-01" || !s.Open[0].Late || s.Open[1].PeriodStart != "2026-09-01" || s.Open[1].Late {
		t.Errorf("open (by due date) = %+v", s.Open)
	}
	if len(s.Received) != 2 || *s.Received[0].ReceivedOn != "2026-10-02" || *s.Received[1].ReceivedOn != "2026-09-30" {
		t.Errorf("received (newest first) = %+v", s.Received)
	}
}

// O recebido no mês conta o calendário da organização: o que entrou no fim do mês passado não entra.
func TestSummary_ReceivedMonthUsesTheOrganizationCalendar(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-08-01", nil)
	org := f.orgID.String()
	a := f.confirm(acme, "2026-08-01", "2026-08-31", 100000)
	b := f.confirm(acme, "2026-09-01", "2026-09-30", 200000)
	for id, on := range map[string]string{a.ID.String(): "2026-09-30", b.ID.String(): "2026-10-01"} {
		if _, err := f.svc.Receive(org, id, receivable.ReceiveInput{ReceivedOn: on}); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := f.svc.Summary(org)
	if s.Totals.ReceivedMonthCents != 200000 || s.Totals.ReceivedMonthCount != 1 {
		t.Errorf("received this month = %d (%d), want only the one received on 2026-10-01", s.Totals.ReceivedMonthCents, s.Totals.ReceivedMonthCount)
	}
}

// O fuso da organização decide quando o período fecha: em Tóquio, setembro já fechou às 15:00 UTC de 30/9.
func TestConfirm_PeriodClosesInTheOrganizationZone(t *testing.T) {
	f := setup(t)
	testClient.Organization.UpdateOneID(f.orgID).SetTimezone("Asia/Tokyo").ExecX(context.Background())
	acme := f.hourly("Acme")
	tk := f.task(acme, "Site")
	f.session(tk, "2026-09-10T01:00:00Z", "2026-09-10T02:00:00Z", rate(10000))
	in := receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", AmountCents: rate(10000)}

	f.now = at(t, "2026-09-30T14:59:59Z") // 23:59:59 do dia 30 em Tóquio
	_, err := f.svc.Confirm(f.orgID.String(), in)
	wantCode(t, err, receivable.ErrPeriodOpen, "")
	f.now = at(t, "2026-09-30T15:00:00Z") // meia-noite em Tóquio
	if _, err := f.svc.Confirm(f.orgID.String(), in); err != nil {
		t.Errorf("at midnight in Tokyo the period is closed: %v", err)
	}
}

// Nada vaza entre organizações: o cliente e o recebível de uma não valem na outra, e a lista de uma não traz o da outra.
func TestIsolationBetweenOrganizations(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	mine := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)

	otherOrg, _ := f.newOrg("Outra", true)
	other := otherOrg.String()
	id := mine.ID.String()

	_, err := f.svc.Confirm(other, receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31", AmountCents: rate(1000)})
	wantCode(t, err, customer.ErrNotFound, "")
	if _, err := f.svc.Receive(other, id, receivable.ReceiveInput{}); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("receive from another organization = %v, want not found", err)
	}
	amount := 1000
	if _, err := f.svc.Update(other, id, receivable.UpdateInput{AmountCents: &amount}); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("update from another organization = %v, want not found", err)
	}
	if err := f.svc.Unconfirm(other, id); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("unconfirm from another organization = %v, want not found", err)
	}
	if err := f.svc.Unreceive(other, id); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("unreceive from another organization = %v, want not found", err)
	}
	if _, err := f.svc.LateCharges(other, id, ""); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("late charges from another organization = %v, want not found", err)
	}
	s, err := f.svc.Summary(other)
	if err != nil || len(s.Open) != 0 || len(s.ToConfirm) != 0 || len(s.Received) != 0 {
		t.Errorf("the other organization's summary = %+v (%v), want it empty", s, err)
	}
	// O recebível continua intacto na organização dele.
	if list, _ := f.svc.Receivables(f.orgID.String()); len(list) != 1 || list[0].AmountCents != 500000 || list[0].ReceivedOn != nil {
		t.Errorf("the receivable changed: %+v", list)
	}
}

// Só as organizações do modo solo têm as rotas: o serviço diz quais são.
func TestSolo(t *testing.T) {
	f := setup(t)
	notSolo, _ := f.newOrg("Equipe", false)
	if !f.svc.Solo(f.orgID.String()) || f.svc.Solo(notSolo.String()) || f.svc.Solo("nope") || f.svc.Solo(uuid.NewString()) {
		t.Error("Solo answers wrongly")
	}
}

// Duas confirmações do mesmo período ao mesmo tempo não cobram o período duas vezes: a que perde esbarra no índice único
// e recebe o mesmo erro de quem chegou depois.
func TestConfirm_ConcurrentConfirmationsMakeOne(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	amount := 500000

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := f.svc.Confirm(org, receivable.ConfirmInput{CustomerID: acme.ID.String(), PeriodStart: "2026-09-01", PeriodEnd: "2026-09-30", AmountCents: &amount})
			errs <- err
		}()
	}
	ok := 0
	for i := 0; i < n; i++ {
		switch err := <-errs; {
		case err == nil:
			ok++
		case !errors.Is(err, receivable.ErrAlreadyConfirmed):
			t.Errorf("a losing confirmation returned %v, want receivable.already_confirmed", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d confirmations succeeded, want exactly 1", ok)
	}
	if list, _ := f.svc.Receivables(org); len(list) != 1 {
		t.Errorf("%d receivables for the period, want 1", len(list))
	}
}

// Uma edição do valor que chega depois de o recebimento ler o recebível, e antes de ele gravar, não deixa o recebimento
// valer o valor velho: a gravação confere o valor que foi lido e, se mudou, recusa (ErrChanged) sem gravar nada. O que
// já foi recebido continua sendo ErrAlreadyReceived, e o que não existe, ErrNotFound.
func TestReceive_RefusesAnAmountEditedAfterTheRead(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)
	id := r.ID.String()
	store := receivable.NewStore(testClient)
	ctx := context.Background()

	// O recebimento leu 500.000 e a edição para 520.000 entrou antes da gravação.
	edited := 520000
	if _, err := f.svc.Update(org, id, receivable.UpdateInput{AmountCents: &edited}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	err := store.Receive(f.orgID, r.ID, 500000, "2026-10-09", 500000, 0, 0)
	wantCode(t, err, receivable.ErrChanged, "")
	if row := testClient.Receivable.GetX(ctx, r.ID); row.ReceivedOn != nil || row.ReceivedCents != nil || row.AmountCents != 520000 {
		t.Errorf("row after a refused receipt = received_on %v, received_cents %v, amount %d; want untouched at 520000", row.ReceivedOn, row.ReceivedCents, row.AmountCents)
	}

	// Lido de novo, o recebimento segue o valor editado.
	got, err := f.svc.Receive(org, id, receivable.ReceiveInput{})
	if err != nil {
		t.Fatalf("receive after the edit: %v", err)
	}
	if got.ReceivedCents == nil || *got.ReceivedCents != 520000 {
		t.Errorf("received = %v, want the edited 520000", got.ReceivedCents)
	}

	// Recebido, o valor velho é recusado como já recebido, não como mudado; e um recebível que não existe é ErrNotFound.
	err = store.Receive(f.orgID, r.ID, 500000, "2026-10-09", 500000, 0, 0)
	wantCode(t, err, receivable.ErrAlreadyReceived, "")
	err = store.Receive(f.orgID, uuid.New(), 500000, "2026-10-09", 500000, 0, 0)
	if !errors.Is(err, database.ErrNotFound) {
		t.Errorf("receive of a missing receivable: err = %v, want ErrNotFound", err)
	}
}

// Dois recebimentos ao mesmo tempo não se sobrescrevem: um grava, e o outro é recusado como já recebido.
func TestReceive_ConcurrentReceiptsMakeOne(t *testing.T) {
	f := setup(t)
	acme := f.monthly("Acme", "2026-09-01", nil)
	org := f.orgID.String()
	r := f.confirm(acme, "2026-09-01", "2026-09-30", 500000)

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		cents := 100000 * (i + 1)
		go func() {
			_, err := f.svc.Receive(org, r.ID.String(), receivable.ReceiveInput{ReceivedCents: &cents})
			errs <- err
		}()
	}
	ok := 0
	for i := 0; i < n; i++ {
		switch err := <-errs; {
		case err == nil:
			ok++
		case !errors.Is(err, receivable.ErrAlreadyReceived):
			t.Errorf("a losing receipt returned %v, want receivable.already_received", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d receipts succeeded, want exactly 1", ok)
	}
}
