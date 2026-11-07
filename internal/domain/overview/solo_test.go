package overview_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/allocation"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/organization"
	"working-time-tracker/internal/domain/overview"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/domain/receivable"
	"working-time-tracker/internal/domain/task"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/testutil"
)

// soloFixture é uma organização no modo solo com um relógio fixo, os serviços sobre o banco de teste e os atalhos para
// criar clientes, projetos, horas e recebíveis. O relógio é o mesmo da tela e dos recebíveis: os dois leem o "agora".
type soloFixture struct {
	t     *testing.T
	svc   *overview.Service
	rec   *receivable.Service
	orgID uuid.UUID
	who   uuid.UUID // o dono, que bate o ponto
	now   time.Time
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// newSolo cria a organização solo no fuso tz, e o instante de hoje é nowText.
func newSolo(t *testing.T, nowText, tz string) *soloFixture {
	t.Helper()
	testutil.Truncate(t, testDB)
	f := &soloFixture{t: t, now: parseTime(t, nowText)}
	clock := func() time.Time { return f.now }
	f.rec = receivable.NewService(receivable.Deps{Store: receivable.NewStore(testClient), Now: clock})
	f.svc = overview.NewService(overview.Deps{
		Projects:    project.NewService(project.NewStore(testClient)),
		Sessions:    work_session.NewService(work_session.NewStore(testClient), task.NewStore(testClient), allocation.NewStore(testClient)),
		Orgs:        organization.NewService(organization.NewStore(testClient)),
		Customers:   customer.NewService(customer.NewStore(testClient)),
		Receivables: f.rec,
		Now:         clock,
	})
	f.orgID, f.who = f.newOrg("Solo", tz)
	return f
}

// newOrg cria uma organização solo com o dono dela.
func (f *soloFixture) newOrg(name, tz string) (uuid.UUID, uuid.UUID) {
	f.t.Helper()
	org := testClient.Organization.Create().SetName(name).SetSolo(true).SetTimezone(tz).SaveX(context.Background())
	p, err := person.NewService(person.NewStore(testClient)).Create(org.ID.String(), "Ana "+name, strings.ToLower(name)+"@solo.com")
	if err != nil {
		f.t.Fatal(err)
	}
	return org.ID, p.ID
}

// hourly cria um cliente por hora que fecha o período no último dia do mês e dá 5 dias de prazo.
func (f *soloFixture) hourly(name string) *ent.Customer {
	f.t.Helper()
	return testClient.Customer.Create().SetOrganizationID(f.orgID).SetName(name).
		SetBillingMode("hourly").SetClosingDay(31).SetPaymentDays(5).SaveX(context.Background())
}

// monthly cria um cliente mensal que fecha no último dia do mês, com o contrato começando em start.
func (f *soloFixture) monthly(name string, feeCents int, start string) *ent.Customer {
	f.t.Helper()
	return testClient.Customer.Create().SetOrganizationID(f.orgID).SetName(name).
		SetBillingMode("monthly").SetMonthlyFeeCents(feeCents).SetContractStart(start).
		SetClosingDay(31).SetPaymentDays(10).SaveX(context.Background())
}

// task cria um projeto (do cliente, ou interno se c é nulo) e devolve uma tarefa dele.
func (f *soloFixture) task(c *ent.Customer, name string) uuid.UUID {
	f.t.Helper()
	ctx := context.Background()
	create := testClient.Project.Create().SetOrganizationID(f.orgID).SetName(name)
	if c != nil {
		create = create.SetCustomerID(c.ID)
	}
	prj := create.SaveX(ctx)
	return testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(ctx).ID
}

// session grava uma sessão fechada (ou aberta, com end vazio) com o valor cobrado dado, nulo no projeto de um cliente mensal.
func (f *soloFixture) session(taskID uuid.UUID, start, end string, bill *int) {
	f.t.Helper()
	var e *time.Time
	if end != "" {
		v := parseTime(f.t, end)
		e = &v
	}
	pay := 0
	testutil.Session(f.t, testClient, taskID, f.who, parseTime(f.t, start), e, &pay, bill)
}

func (f *soloFixture) confirm(c *ent.Customer, start, end string, amount int) *receivable.Receivable {
	f.t.Helper()
	r, err := f.rec.Confirm(f.orgID.String(), receivable.ConfirmInput{
		CustomerID: c.ID.String(), PeriodStart: start, PeriodEnd: end, AmountCents: &amount,
	})
	if err != nil {
		f.t.Fatalf("confirm %s %s..%s: %v", c.Name, start, end, err)
	}
	return r
}

// receive registra que o recebível entrou em on; cents nulo vale o confirmado (o por hora), e fee, os juros e a multa do mensal.
func (f *soloFixture) receive(r *receivable.Receivable, on string, cents, fee *int) {
	f.t.Helper()
	if _, err := f.rec.Receive(f.orgID.String(), r.ID.String(), receivable.ReceiveInput{ReceivedOn: on, ReceivedCents: cents, LateFeeCents: fee}); err != nil {
		f.t.Fatalf("receive %s: %v", r.PeriodEnd, err)
	}
}

func (f *soloFixture) solo(window string) *overview.SoloOverview {
	f.t.Helper()
	o, err := f.svc.Solo(f.orgID.String(), window)
	if err != nil {
		f.t.Fatalf("solo overview (%q): %v", window, err)
	}
	return o
}

func rateText(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(*v)
}

// totalsLine resume os cartões de uma janela.
func totalsLine(o *overview.SoloOverview) string {
	t := o.Totals
	return fmt.Sprintf("%s..%s: %.0fs, received %d, projected %d, rate %s below=%v, internal %.0fs",
		o.From, o.To, t.Seconds, t.ReceivedCents, t.ProjectedCents, rateText(t.EffectiveRateCents), t.BelowMinRate, o.InternalSeconds)
}

// rowLine resume uma linha da tabela por cliente.
func rowLine(c overview.SoloCustomer) string {
	return fmt.Sprintf("%s [%s]: %.0fs, received %d, projected %d, rate %s below=%v",
		c.Customer.Name, c.Kind, c.Seconds, c.ReceivedCents, c.ProjectedCents, rateText(c.EffectiveRateCents), c.BelowMinRate)
}

func rowLines(o *overview.SoloOverview) []string {
	out := make([]string, len(o.Customers))
	for i, c := range o.Customers {
		out[i] = rowLine(c)
	}
	return out
}

// checkSums confere que as linhas da tabela, com as horas internas, somam o que os cartões dizem.
func checkSums(t *testing.T, o *overview.SoloOverview) {
	t.Helper()
	seconds, received, projected := o.InternalSeconds, 0, 0
	for _, c := range o.Customers {
		seconds += c.Seconds
		received += c.ReceivedCents
		projected += c.ProjectedCents
	}
	if seconds != o.Totals.Seconds || received != o.Totals.ReceivedCents || projected != o.Totals.ProjectedCents {
		t.Errorf("%s: the rows add up to %.0fs, %d and %d; the cards say %.0fs, %d and %d",
			o.Window, seconds, received, projected, o.Totals.Seconds, o.Totals.ReceivedCents, o.Totals.ProjectedCents)
	}
}

// O cenário, hoje quarta 18/11/2026 ao meio-dia em São Paulo (UTC-3), com os dois clientes fechando no último dia do mês:
//
//   - Alfa, por hora a 100,00: 6h entre dezembro e janeiro (a sessão atravessa o ano), 1h em janeiro, 2h em agosto, 3h
//     em setembro, 4h em outubro (2h delas atravessam a virada de outubro para novembro) e 6h30 em novembro (uma sessão
//     aberta de 2h). Os recebíveis: dezembro de 2025 (400,00) recebido em 10/1; setembro, que o dono confirmou em
//     280,00 (o calculado era 300,00), recebido em 5/10; outubro, 400,00, recebido em 5/11. Janeiro e agosto ficaram só
//     como proposta.
//   - Beta, mensal a 5.000,00 desde 1/8, com 3h em outubro e 4h em novembro. Agosto confirmado e recebido em 12/10 com
//     100,00 de multa (5.100,00); outubro confirmado em 4.500,00 e ainda não recebido; setembro e novembro (o período em
//     andamento) só como proposta; dezembro, o seguinte, ainda não é proposto.
//   - Delta, por hora, sem nada. Um projeto interno com 1h em outubro e 1h30 em novembro. E outra organização solo, com
//     horas e dinheiro, que não pode entrar em conta nenhuma.
func richSolo(t *testing.T) *soloFixture {
	t.Helper()
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	alfa, beta := f.hourly("Alfa"), f.monthly("Beta", 500000, "2026-08-01")
	f.hourly("Delta")
	rate := cents(10000)

	a := f.task(alfa, "Site Alfa")
	f.session(a, "2025-12-31T23:00:00Z", "2026-01-01T05:00:00Z", rate) // 4h em dezembro e 2h em janeiro, no fuso da organização
	f.session(a, "2026-01-15T13:00:00Z", "2026-01-15T14:00:00Z", rate)
	f.session(a, "2026-08-12T13:00:00Z", "2026-08-12T15:00:00Z", rate)
	f.session(a, "2026-09-10T13:00:00Z", "2026-09-10T16:00:00Z", rate)
	f.session(a, "2026-10-15T13:00:00Z", "2026-10-15T15:00:00Z", rate)
	f.session(a, "2026-11-01T01:00:00Z", "2026-11-01T05:00:00Z", rate) // 2h em outubro e 2h em novembro
	f.session(a, "2026-11-03T13:00:00Z", "2026-11-03T15:30:00Z", rate)
	f.session(a, "2026-11-18T13:00:00Z", "", rate) // aberta: 2h até agora

	b := f.task(beta, "Site Beta")
	f.session(b, "2026-10-20T13:00:00Z", "2026-10-20T16:00:00Z", nil)
	f.session(b, "2026-11-05T13:00:00Z", "2026-11-05T17:00:00Z", nil)

	internal := f.task(nil, "Interno")
	f.session(internal, "2026-10-25T13:00:00Z", "2026-10-25T14:00:00Z", nil)
	f.session(internal, "2026-11-10T13:00:00Z", "2026-11-10T14:30:00Z", nil)

	f.receive(f.confirm(alfa, "2025-12-01", "2025-12-31", 40000), "2026-01-10", nil, nil)
	f.receive(f.confirm(alfa, "2026-09-01", "2026-09-30", 28000), "2026-10-05", nil, nil)
	f.receive(f.confirm(alfa, "2026-10-01", "2026-10-31", 40000), "2026-11-05", nil, nil)
	f.receive(f.confirm(beta, "2026-08-01", "2026-08-31", 500000), "2026-10-12", cents(510000), cents(10000))
	f.confirm(beta, "2026-10-01", "2026-10-31", 450000)

	// Outra organização solo: ela não pode entrar em nenhuma conta.
	other, otherWho := f.newOrg("Outra", "America/Sao_Paulo")
	otherCustomer := testClient.Customer.Create().SetOrganizationID(other).SetName("Alheio").SetBillingMode("hourly").SetClosingDay(31).SaveX(context.Background())
	prj := testClient.Project.Create().SetOrganizationID(other).SetName("Alheio").SetCustomerID(otherCustomer.ID).SaveX(context.Background())
	tsk := testClient.Task.Create().SetProjectID(prj.ID).SetName("T").SaveX(context.Background())
	pay, bill, end := 0, 99900, parseTime(t, "2026-11-04T15:00:00Z")
	testutil.Session(t, testClient, tsk.ID, otherWho, parseTime(t, "2026-11-04T10:00:00Z"), &end, &pay, &bill)
	return f
}

// As três janelas do mesmo cenário. Cada número está feito à mão no comentário de richSolo.
func TestSolo_Windows(t *testing.T) {
	f := richSolo(t)

	for _, c := range []struct {
		window string
		totals string
		rows   []string
	}{
		{
			// Novembro: o período em andamento do Alfa (6h30 a 100,00) e o da mensalidade do Beta contam como projetados;
			// a Receita é só o que entrou em novembro, o recebimento de outubro do Alfa.
			window: overview.WindowMonth,
			totals: "2026-11-01..2026-11-30: 43200s, received 40000, projected 565000, rate 47083 below=false, internal 5400s",
			rows: []string{
				"Alfa [hourly]: 23400s, received 40000, projected 65000, rate 10000 below=false",
				"Beta [monthly]: 14400s, received 0, projected 500000, rate 125000 below=false",
			},
		},
		{
			// Outubro: as horas do Alfa e do Beta (e a 1h interna) cortadas no mês, o que entrou em outubro (setembro do
			// Alfa e agosto do Beta, com a multa) e o que se espera dos períodos de outubro, já confirmados: 400,00 e 4.500,00.
			window: overview.WindowLastMonth,
			totals: "2026-10-01..2026-10-31: 28800s, received 538000, projected 490000, rate 61250 below=false, internal 3600s",
			rows: []string{
				"Alfa [hourly]: 14400s, received 28000, projected 40000, rate 10000 below=false",
				"Beta [monthly]: 10800s, received 510000, projected 450000, rate 150000 below=false",
			},
		},
		{
			// O ano vai de 1/1 ao fim de novembro. Projetado do Alfa: janeiro e agosto (propostas, 300,00 e 200,00),
			// setembro (confirmado em 280,00), outubro e novembro (650,00); do Beta: agosto, setembro, outubro e novembro.
			// O dezembro de 2025 do Alfa, recebido em janeiro, entra na Receita do ano e não na projetada.
			window: overview.WindowYear,
			totals: "2026-01-01..2026-11-30: 100800s, received 618000, projected 2133000, rate 76179 below=false, internal 9000s",
			rows: []string{
				"Alfa [hourly]: 66600s, received 108000, projected 183000, rate 9892 below=false",
				"Beta [monthly]: 25200s, received 510000, projected 1950000, rate 278571 below=false",
			},
		},
	} {
		got := f.solo(c.window)
		if got.Window != c.window || got.Timezone != "America/Sao_Paulo" || got.Currency != "BRL" || !got.GeneratedAt.Equal(f.now) {
			t.Errorf("%s: window %q, timezone %q, currency %q, generated at %s", c.window, got.Window, got.Timezone, got.Currency, got.GeneratedAt)
		}
		if line := totalsLine(got); line != c.totals {
			t.Errorf("%s totals\n got  %s\n want %s", c.window, line, c.totals)
		}
		if rows := rowLines(got); !slices.Equal(rows, c.rows) {
			t.Errorf("%s rows\n got  %q\n want %q", c.window, rows, c.rows)
		}
		checkSums(t, got)
	}

	// Sem janela, vale o mês.
	if got, want := totalsLine(f.solo("")), "2026-11-01..2026-11-30"; !strings.HasPrefix(got, want) {
		t.Errorf("the default window = %s, want %s", got, want)
	}
}

// A Receita é a data do recebimento, e a projetada, o fim do período: uma cobrança de janeiro recebida em 5 de fevereiro
// é Receita de fevereiro e projetada de janeiro, e nenhuma das duas contas conta o mesmo recebível duas vezes.
func TestSolo_RevenueFollowsTheReceiptDateAndProjectedTheEndOfThePeriod(t *testing.T) {
	f := newSolo(t, "2026-02-20T15:00:00Z", "America/Sao_Paulo")
	alfa := f.hourly("Alfa")
	task := f.task(alfa, "Site")
	f.session(task, "2026-01-10T13:00:00Z", "2026-01-10T15:00:00Z", cents(10000)) // 2h em janeiro: 200,00
	f.receive(f.confirm(alfa, "2026-01-01", "2026-01-31", 20000), "2026-02-05", nil, nil)

	jan := f.solo(overview.WindowLastMonth)
	feb := f.solo(overview.WindowMonth)
	if got, want := totalsLine(jan), "2026-01-01..2026-01-31: 7200s, received 0, projected 20000, rate 10000 below=false, internal 0s"; got != want {
		t.Errorf("January\n got  %s\n want %s", got, want)
	}
	if got, want := totalsLine(feb), "2026-02-01..2026-02-28: 0s, received 20000, projected 0, rate - below=false, internal 0s"; got != want {
		t.Errorf("February\n got  %s\n want %s", got, want)
	}
	// Em fevereiro o Alfa entrou só pelo que recebeu: sem horas, o valor por hora é um travessão (nulo) e não um aviso.
	if rows := rowLines(feb); !slices.Equal(rows, []string{"Alfa [hourly]: 0s, received 20000, projected 0, rate - below=false"}) {
		t.Errorf("February rows = %q", rows)
	}

	// Desfazer o recebimento tira a Receita de fevereiro, e o projetado de janeiro continua o mesmo (o recebível segue confirmado).
	list, err := f.rec.Receivables(f.orgID.String())
	if err != nil || len(list) != 1 {
		t.Fatalf("receivables = %v, %v", list, err)
	}
	if err := f.rec.Unreceive(f.orgID.String(), list[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	if got, want := totalsLine(f.solo(overview.WindowMonth)), "2026-02-01..2026-02-28: 0s, received 0, projected 0, rate - below=false, internal 0s"; got != want {
		t.Errorf("February after undoing the receipt\n got  %s\n want %s", got, want)
	}
	if got := f.solo(overview.WindowLastMonth).Totals.ProjectedCents; got != 20000 {
		t.Errorf("January projected after undoing the receipt = %d, want the confirmed 20000", got)
	}
	// Desfazer a confirmação devolve o período a proposta: o projetado volta a ser o calculado das horas.
	if err := f.rec.Unconfirm(f.orgID.String(), list[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	if got := f.solo(overview.WindowLastMonth).Totals.ProjectedCents; got != 20000 {
		t.Errorf("January projected after undoing the confirmation = %d, want the proposal 20000", got)
	}
}

// O valor confirmado vale mais que a proposta: se o dono ajustou o valor, é o ajustado que se espera, e as horas
// registradas depois da confirmação não o mexem.
func TestSolo_ProjectedUsesTheConfirmedAmountOverTheProposal(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	alfa := f.hourly("Alfa")
	task := f.task(alfa, "Site")
	f.session(task, "2026-10-10T13:00:00Z", "2026-10-10T15:00:00Z", cents(10000)) // 200,00
	f.confirm(alfa, "2026-10-01", "2026-10-31", 150000)                           // o dono confirmou 1.500,00
	f.session(task, "2026-10-12T13:00:00Z", "2026-10-12T15:00:00Z", cents(10000)) // depois da confirmação

	got := f.solo(overview.WindowLastMonth)
	if line := rowLines(got); !slices.Equal(line, []string{"Alfa [hourly]: 14400s, received 0, projected 150000, rate 37500 below=false"}) {
		t.Errorf("rows = %q, want the confirmed 150000 and not the 400,00 of the hours", line)
	}
}

// O fuso da organização decide onde o mês e o ano começam: a mesma sessão, que atravessa a meia-noite de 1º de novembro
// em São Paulo, cai inteira em novembro em UTC. Hoje é 30/11 22:00 em São Paulo e já 1º/12 em UTC.
func TestSolo_TheWindowFollowsTheTimezoneOfTheOrganization(t *testing.T) {
	for _, c := range []struct {
		tz, month, lastMonth string
	}{
		{
			tz:        "America/Sao_Paulo",
			month:     "2026-11-01..2026-11-30: 7200s, received 0, projected 20000, rate 10000 below=false, internal 0s",
			lastMonth: "2026-10-01..2026-10-31: 7200s, received 0, projected 20000, rate 10000 below=false, internal 0s",
		},
		{
			// Em UTC hoje já é dezembro: "este mês" é dezembro (sem nada), "mês passado" é novembro, com a sessão inteira.
			tz:        "UTC",
			month:     "2026-12-01..2026-12-31: 0s, received 0, projected 0, rate - below=false, internal 0s",
			lastMonth: "2026-11-01..2026-11-30: 14400s, received 0, projected 40000, rate 10000 below=false, internal 0s",
		},
	} {
		f := newSolo(t, "2026-12-01T01:00:00Z", c.tz)
		alfa := f.hourly("Alfa")
		task := f.task(alfa, "Site")
		f.session(task, "2026-11-01T01:00:00Z", "2026-11-01T05:00:00Z", cents(10000)) // 02:00..05:00 UTC; 22:00..02:00 em São Paulo
		if got := totalsLine(f.solo(overview.WindowMonth)); got != c.month {
			t.Errorf("%s this month\n got  %s\n want %s", c.tz, got, c.month)
		}
		// Em São Paulo a sessão corta em 1/11 03:00 UTC: 2h em outubro (as da noite de 31/10) e 2h em novembro. Em UTC
		// hoje já é dezembro, e o mês passado é novembro, com a sessão inteira.
		if got := totalsLine(f.solo(overview.WindowLastMonth)); got != c.lastMonth {
			t.Errorf("%s last month\n got  %s\n want %s", c.tz, got, c.lastMonth)
		}
	}
}

// O dia de hoje no fuso da organização decide a janela: perto da meia-noite de 31/12 em São Paulo, o ano de Greenwich já virou.
func TestSolo_TheYearFollowsTheTimezoneOfTheOrganization(t *testing.T) {
	// 2027-01-01 01:00 UTC é 31/12/2026 22:00 em São Paulo.
	f := newSolo(t, "2027-01-01T01:00:00Z", "America/Sao_Paulo")
	if got := f.solo(overview.WindowYear); got.From != "2026-01-01" || got.To != "2026-12-31" {
		t.Errorf("the year in São Paulo = %s..%s, want 2026-01-01..2026-12-31", got.From, got.To)
	}
	if got := f.solo(overview.WindowMonth); got.From != "2026-12-01" || got.To != "2026-12-31" {
		t.Errorf("the month in São Paulo = %s..%s, want December", got.From, got.To)
	}
	// Em dezembro, o mês passado cruza a virada do ano de volta: janeiro de 2027 vê dezembro de 2026.
	g := newSolo(t, "2027-01-20T15:00:00Z", "America/Sao_Paulo")
	if got := g.solo(overview.WindowLastMonth); got.From != "2026-12-01" || got.To != "2026-12-31" {
		t.Errorf("last month in January = %s..%s, want December of the year before", got.From, got.To)
	}
	if got := g.solo(overview.WindowYear); got.From != "2027-01-01" || got.To != "2027-01-31" {
		t.Errorf("the year in January = %s..%s, want 2027-01-01..2027-01-31", got.From, got.To)
	}
	// Fevereiro tem 28 dias em 2027 e 29 em 2028.
	for _, c := range []struct{ now, to string }{{"2027-02-10T15:00:00Z", "2027-02-28"}, {"2028-02-10T15:00:00Z", "2028-02-29"}} {
		if got := newSolo(t, c.now, "America/Sao_Paulo").solo(overview.WindowMonth); got.To != c.to {
			t.Errorf("the month of %s ends %s, want %s", c.now, got.To, c.to)
		}
	}
}

// As propostas do domínio dos recebíveis têm tetos (24 períodos fechados por cliente por hora, 60 por mensal), mas uma
// janela tem no máximo 12 períodos de cada cliente: um histórico longo e nunca confirmado não muda a conta do ano.
func TestSolo_AnOldUnconfirmedHistoryDoesNotChangeTheYear(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	alfa := f.hourly("Alfa")
	task := f.task(alfa, "Site")
	// 1h por mês em 40 meses seguidos, de agosto de 2023 a novembro de 2026, sem confirmar nenhum.
	for m := 0; m < 40; m++ {
		at := time.Date(2023, time.August+time.Month(m), 15, 13, 0, 0, 0, time.UTC)
		f.session(task, at.Format(time.RFC3339), at.Add(time.Hour).Format(time.RFC3339), cents(10000))
	}
	// O mensal tem o contrato começando em 2019: 90 períodos para trás, mais que os 60 do teto.
	f.monthly("Beta", 100000, "2019-01-01")

	got := f.solo(overview.WindowYear)
	want := []string{
		"Alfa [hourly]: 39600s, received 0, projected 110000, rate 10000 below=false", // 11 meses de 1h (jan a nov)
		"Beta [monthly]: 0s, received 0, projected 1100000, rate - below=false",       // 11 mensalidades de 1.000,00
	}
	if rows := rowLines(got); !slices.Equal(rows, want) {
		t.Errorf("rows\n got  %q\n want %q", rows, want)
	}
}

// O cliente sem horas aparece quando tem dinheiro na janela (a mensalidade), o sem nada na janela não aparece, e as
// horas dos projetos sem cliente ficam numa linha só delas. A organização sem nada devolve a tabela vazia, não nula.
func TestSolo_CustomersWithoutHoursAndInternalProjects(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	empty := f.solo(overview.WindowMonth)
	if empty.Customers == nil || len(empty.Customers) != 0 || empty.InternalSeconds != 0 || empty.Totals.EffectiveRateCents != nil || empty.Totals.BelowMinRate {
		t.Errorf("an empty organization = %+v, want no rows, no hours and no rate", empty)
	}

	f.hourly("Sem nada")
	old := f.task(f.hourly("Antigo"), "Antigo")
	f.session(old, "2026-03-10T13:00:00Z", "2026-03-10T14:00:00Z", cents(10000)) // março: fora do mês
	f.monthly("Mensal sem horas", 300000, "2026-11-01")
	f.monthly("Mensal futuro", 300000, "2027-06-01") // o contrato só começa depois do período seguinte
	f.session(f.task(nil, "Interno"), "2026-11-02T13:00:00Z", "2026-11-02T15:00:00Z", nil)

	got := f.solo(overview.WindowMonth)
	if rows, want := rowLines(got), []string{"Mensal sem horas [monthly]: 0s, received 0, projected 300000, rate - below=false"}; !slices.Equal(rows, want) {
		t.Errorf("rows\n got  %q\n want %q", rows, want)
	}
	if got.InternalSeconds != 7200 || got.Totals.Seconds != 7200 {
		t.Errorf("internal %.0fs of %.0fs, want the 2 internal hours in both", got.InternalSeconds, got.Totals.Seconds)
	}
	// Só horas internas e uma mensalidade sem horas do cliente: o valor por hora total divide a projetada por todas as horas.
	if rate := got.Totals.EffectiveRateCents; rate == nil || *rate != 150000 {
		t.Errorf("effective rate = %v, want 3.000,00 over 2 internal hours = 1.500,00", rate)
	}
	checkSums(t, got)

	// No ano, o cliente que só trabalhou em março aparece com as horas e o valor da proposta de março.
	year := f.solo(overview.WindowYear)
	if !slices.Contains(rowLines(year), "Antigo [hourly]: 3600s, received 0, projected 10000, rate 10000 below=false") {
		t.Errorf("the year rows = %q, want the March hours of Antigo with their 100,00", rowLines(year))
	}
}

// A ordem é a do nome, sem caixa nem acento, e nunca a das horas nem do valor: quem mais trabalhou e quem mais rende não
// vão para o topo.
func TestSolo_CustomersAreOrderedByName(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	for i, name := range []string{"Zeta", "Álvaro", "beta", "Carla"} {
		// O Zeta é o que mais trabalha e o que mais rende; a Carla é a que menos.
		hours := time.Duration(4-i) * time.Hour
		start := parseTime(t, "2026-11-02T13:00:00Z")
		task := f.task(f.hourly(name), name)
		f.session(task, start.Format(time.RFC3339), start.Add(hours).Format(time.RFC3339), cents(10000*(4-i)))
	}
	var names []string
	for _, c := range f.solo(overview.WindowMonth).Customers {
		names = append(names, c.Customer.Name)
	}
	if want := []string{"Álvaro", "beta", "Carla", "Zeta"}; !slices.Equal(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
}

// O valor por hora efetivo abaixo do piso de 10,00 é um aviso (BelowMinRate), nunca um erro; exatamente no piso não avisa.
func TestSolo_EffectiveRateBelowTheMinimumIsAWarning(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	floor := f.monthly("No piso", 10000, "2026-11-01")                                     // 100,00 por mês
	above := f.monthly("Acima", 10000, "2026-11-01")                                       // 100,00 por mês
	f.session(f.task(floor, "Piso"), "2026-11-02T13:00:00Z", "2026-11-02T23:00:00Z", nil)  // 10h: 10,00 por hora
	f.session(f.task(above, "Acima"), "2026-11-03T13:00:00Z", "2026-11-03T22:00:00Z", nil) // 9h: 11,11 por hora

	want := []string{
		"Acima [monthly]: 32400s, received 0, projected 10000, rate 1111 below=false",
		"No piso [monthly]: 36000s, received 0, projected 10000, rate 1000 below=false",
	}
	if rows := rowLines(f.solo(overview.WindowMonth)); !slices.Equal(rows, want) {
		t.Errorf("at the minimum\n got  %q\n want %q", rows, want)
	}

	// Mais 2h na mesma mensalidade: 11h rendem 9,09 por hora, abaixo do piso. O cartão do total (200,00 em 21h = 9,52) avisa também.
	f.session(f.task(above, "Mais horas"), "2026-11-04T13:00:00Z", "2026-11-04T15:00:00Z", nil)
	got := f.solo(overview.WindowMonth)
	if row := rowLine(got.Customers[0]); row != "Acima [monthly]: 39600s, received 0, projected 10000, rate 909 below=true" {
		t.Errorf("below the minimum: %s", row)
	}
	if rate := got.Totals.EffectiveRateCents; rate == nil || *rate != 952 || !got.Totals.BelowMinRate {
		t.Errorf("total rate = %s below=%v, want 952 and the warning", rateText(got.Totals.EffectiveRateCents), got.Totals.BelowMinRate)
	}
}

// A janela inválida é um erro com código; um id malformado ou de uma organização que não existe vale como não encontrado.
func TestSolo_InvalidWindowAndUnknownOrganization(t *testing.T) {
	f := newSolo(t, "2026-11-18T15:00:00Z", "America/Sao_Paulo")
	for _, w := range []string{"week", "MONTH", "all_time", "year ", "2026"} {
		_, err := f.svc.Solo(f.orgID.String(), w)
		var coded *apperr.Error
		if !errors.Is(err, overview.ErrInvalidWindow) || !errors.As(err, &coded) || coded.Params["field"] != "window" {
			t.Errorf("window %q: error = %v, want overview.invalid_window with field window", w, err)
		}
	}
	for _, id := range []string{"nope", "", uuid.NewString()} {
		if _, err := f.svc.Solo(id, overview.WindowMonth); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("organization %q: error = %v, want not found", id, err)
		}
	}
}
