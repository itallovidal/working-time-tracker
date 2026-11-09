package customer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/project"
	"working-time-tracker/internal/validate"
)

// Os termos comerciais do cliente (modo solo): o modo de cobrança, o que é de cada modo, o que o servidor recusa e o
// que trava o modo.

// opt é um campo opcional de dinheiro enviado com valor; o null é validate.Optional[int]{Set: true}.
func opt(v int) validate.Optional[int] {
	return validate.Optional[int]{Set: true, Value: &v}
}

// withRate dá ao cliente por hora o valor por hora, que é obrigatório (Decisão 21), quando o teste não manda um (nem
// null): o cliente mensal e o que já diz o valor passam como vieram.
func withRate(in customer.Input) customer.Input {
	if !in.HourlyRateCents.Set && (in.BillingMode == nil || *in.BillingMode != customer.BillingMonthly) {
		in.HourlyRateCents = opt(15000)
	}
	return in
}

// setSolo liga ou desliga o modo solo da organização do teste, como o dono faz em Configurações.
func setSolo(t *testing.T, orgID string, solo bool) {
	t.Helper()
	uid := uuid.MustParse(orgID)
	if err := testClient.Organization.UpdateOneID(uid).SetSolo(solo).Exec(context.Background()); err != nil {
		t.Fatalf("set solo: %v", err)
	}
}

// refused confere que err é o erro want, com o field dado.
func refused(t *testing.T, name string, err error, want error, field string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: err = %v, want %v", name, err, want)
		return
	}
	if got := fieldOf(err); got != field {
		t.Errorf("%s: field = %q, want %q", name, got, field)
	}
}

// Um cliente só com o valor por hora é por hora, fecha no último dia e vence no fechamento, e é assim que sai da lista e
// do Get. Sem o valor por hora ele é recusado: a cobrança mora no cliente (Decisão 21).
func TestBilling_DefaultsAreTheOldBehaviour(t *testing.T) {
	svc, _, orgID := setup(t)
	_, err := svc.Create(orgID, customer.Input{Name: ptr("Sem valor")})
	refused(t, "hourly customer without a rate", err, apperr.ErrFieldRequired, "hourly_rate_cents")
	created, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Empresa A")}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for name, c := range map[string]*customer.Customer{"created": created, "got": mustGet(t, svc, created.ID.String())} {
		if c.BillingMode != customer.BillingHourly || c.BillingCycle != customer.CycleMonthly || c.CycleAnchor != nil ||
			c.ClosingDay != 31 || c.PaymentDays != 0 ||
			c.HourlyRateCents == nil || *c.HourlyRateCents != 15000 || c.MonthlyFeeCents != nil || c.ContractStart != nil || c.ContractEnd != nil ||
			c.LateFeeBP != 0 || c.LateInterestBPMonth != 0 || c.GraceDays != 0 {
			t.Errorf("%s = %+v, want hourly at 150.00, closing day 31 and nothing else", name, c)
		}
	}
	list, err := svc.ListByOrg(orgID)
	if err != nil || len(list) != 1 || list[0].BillingMode != customer.BillingHourly || list[0].ClosingDay != 31 {
		t.Errorf("list = %+v, %v", list, err)
	}

	// Editar só o nome não mexe em termo nenhum.
	updated, err := svc.Update(created.ID.String(), customer.Input{Name: ptr("Empresa Alfa")})
	if err != nil || updated.Name != "Empresa Alfa" || updated.BillingMode != customer.BillingHourly || updated.ClosingDay != 31 {
		t.Errorf("update name = %+v, %v", updated, err)
	}
}

func mustGet(t *testing.T, svc *customer.Service, id string) *customer.Customer {
	t.Helper()
	c, err := svc.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return c
}

func TestBilling_HourlyTerms(t *testing.T) {
	svc, _, orgID := setup(t)

	// O valor por hora tem o piso de 10,00 (e o teto).
	refused(t, "rate under the floor", func() error {
		_, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("X"), HourlyRateCents: opt(validate.MinRateCents - 1)}))
		return err
	}(), customer.ErrInvalidHourlyRate, "hourly_rate_cents")
	refused(t, "rate zero", func() error {
		_, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("X"), HourlyRateCents: opt(0)}))
		return err
	}(), customer.ErrInvalidHourlyRate, "hourly_rate_cents")
	refused(t, "rate over the cap", func() error {
		_, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("X"), HourlyRateCents: opt(validate.MaxCents + 1)}))
		return err
	}(), customer.ErrInvalidHourlyRate, "hourly_rate_cents")

	c, err := svc.Create(orgID, withRate(customer.Input{
		Name: ptr("Hora"), HourlyRateCents: opt(validate.MinRateCents), ClosingDay: ptr(5), PaymentDays: ptr(15),
	}))
	if err != nil {
		t.Fatalf("create at the floor: %v", err)
	}
	if c.HourlyRateCents == nil || *c.HourlyRateCents != 1000 || c.ClosingDay != 5 || c.PaymentDays != 15 {
		t.Errorf("created = %+v", c)
	}
	if got := mustGet(t, svc, c.ID.String()); got.HourlyRateCents == nil || *got.HourlyRateCents != 1000 || got.ClosingDay != 5 || got.PaymentDays != 15 {
		t.Errorf("stored = %+v", got)
	}

	// Ausente mantém o valor; null é recusado, porque o cliente por hora sempre tem valor.
	kept, err := svc.Update(c.ID.String(), customer.Input{Name: ptr("Hora 2")})
	if err != nil || kept.HourlyRateCents == nil || *kept.HourlyRateCents != 1000 {
		t.Errorf("absent keeps the rate: %+v, %v", kept, err)
	}
	_, err = svc.Update(c.ID.String(), customer.Input{HourlyRateCents: validate.Optional[int]{Set: true}})
	refused(t, "null rate", err, apperr.ErrFieldRequired, "hourly_rate_cents")
	if got := mustGet(t, svc, c.ID.String()); got.HourlyRateCents == nil || *got.HourlyRateCents != 1000 {
		t.Errorf("refused null kept = %+v, want the 10.00 rate", got)
	}

	// O dia de fechamento vai de 1 a 31 e o prazo, de 0 ao teto.
	for _, day := range []int{0, 32, -1} {
		_, err := svc.Update(c.ID.String(), customer.Input{ClosingDay: ptr(day)})
		refused(t, "closing day", err, customer.ErrInvalidClosingDay, "closing_day")
	}
	for _, day := range []int{1, 28, 31} {
		if got, err := svc.Update(c.ID.String(), customer.Input{ClosingDay: ptr(day)}); err != nil || got.ClosingDay != day {
			t.Errorf("closing day %d: %+v, %v", day, got, err)
		}
	}
	for _, days := range []int{-1, validate.MaxTermDays + 1} {
		_, err := svc.Update(c.ID.String(), customer.Input{PaymentDays: ptr(days)})
		refused(t, "payment days", err, customer.ErrInvalidPaymentDays, "payment_days")
	}
	for _, days := range []int{0, 30, validate.MaxTermDays} {
		if got, err := svc.Update(c.ID.String(), customer.Input{PaymentDays: ptr(days)}); err != nil || got.PaymentDays != days {
			t.Errorf("payment days %d: %+v, %v", days, got, err)
		}
	}
}

// Num cliente por hora, o que é do contrato mensal é recusado com o campo; o padrão do campo (zero, vazio, null) não é
// valor, e a tela que manda tudo sempre passa.
func TestBilling_HourlyRefusesMonthlyFields(t *testing.T) {
	svc, _, orgID := setup(t)
	cases := []struct {
		field string
		in    customer.Input
	}{
		{"monthly_fee_cents", customer.Input{MonthlyFeeCents: opt(100000)}},
		{"contract_start", customer.Input{ContractStart: ptr("2026-11-01")}},
		{"contract_end", customer.Input{ContractEnd: ptr("2027-11-01")}},
		{"late_fee_bp", customer.Input{LateFeeBP: ptr(200)}},
		{"late_interest_bp_month", customer.Input{LateInterestBPMonth: ptr(100)}},
		{"grace_days", customer.Input{GraceDays: ptr(3)}},
	}
	for _, c := range cases {
		c.in.Name = ptr("X")
		_, err := svc.Create(orgID, withRate(c.in))
		refused(t, "create with "+c.field, err, customer.ErrFieldMonthlyOnly, c.field)
	}

	got, err := svc.Create(orgID, withRate(customer.Input{
		Name: ptr("Tudo vazio"), BillingMode: ptr("hourly"),
		MonthlyFeeCents: validate.Optional[int]{Set: true}, ContractStart: ptr(""), ContractEnd: ptr("  "),
		LateFeeBP: ptr(0), LateInterestBPMonth: ptr(0), GraceDays: ptr(0),
	}))
	if err != nil || got.BillingMode != customer.BillingHourly || got.MonthlyFeeCents != nil || got.ContractStart != nil {
		t.Errorf("empty monthly fields on an hourly customer: %+v, %v", got, err)
	}
}

func TestBilling_BillingModeValue(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	for _, mode := range []string{"", "weekly", "MONTHLY", "hour"} {
		_, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("X"), BillingMode: ptr(mode)}))
		refused(t, "mode "+mode, err, customer.ErrInvalidBillingMode, "billing_mode")
	}
}

// O mensal só vale numa organização solo: na criação e na troca de modo.
func TestBilling_MonthlyNeedsSolo(t *testing.T) {
	svc, _, orgID := setup(t)
	monthly := customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
	}
	_, err := svc.Create(orgID, withRate(monthly))
	refused(t, "create monthly outside solo", err, customer.ErrMonthlyNeedsSolo, "billing_mode")

	hourly, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Hora")}))
	if err != nil {
		t.Fatalf("create hourly: %v", err)
	}
	_, err = svc.Update(hourly.ID.String(), customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")})
	refused(t, "switch to monthly outside solo", err, customer.ErrMonthlyNeedsSolo, "billing_mode")
	if got := mustGet(t, svc, hourly.ID.String()); got.BillingMode != customer.BillingHourly || got.MonthlyFeeCents != nil {
		t.Errorf("a refused switch changed the customer: %+v", got)
	}

	// Com o modo ligado, passa.
	setSolo(t, orgID, true)
	created, err := svc.Create(orgID, withRate(monthly))
	if err != nil || created.BillingMode != customer.BillingMonthly {
		t.Fatalf("create monthly in a solo organization: %+v, %v", created, err)
	}

	// Desligar o modo esconde, não apaga: o cliente mensal continua guardado e se edita sem tocar nos termos.
	setSolo(t, orgID, false)
	renamed, err := svc.Update(created.ID.String(), customer.Input{Name: ptr("Mensal 2"), ContactName: ptr("Carla")})
	if err != nil || renamed.Name != "Mensal 2" || renamed.BillingMode != customer.BillingMonthly ||
		renamed.MonthlyFeeCents == nil || *renamed.MonthlyFeeCents != 630000 || renamed.ContractStart == nil || *renamed.ContractStart != "2026-11-01" {
		t.Errorf("edit of a monthly customer outside solo = %+v, %v; want the terms kept", renamed, err)
	}
}

func TestBilling_MonthlyContract(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	base := func() customer.Input {
		return customer.Input{Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")}
	}

	// A mensalidade e o início são obrigatórios.
	noFee := base()
	noFee.MonthlyFeeCents = validate.Optional[int]{}
	_, err := svc.Create(orgID, withRate(noFee))
	refused(t, "no fee", err, apperr.ErrFieldRequired, "monthly_fee_cents")
	nullFee := base()
	nullFee.MonthlyFeeCents = validate.Optional[int]{Set: true}
	_, err = svc.Create(orgID, withRate(nullFee))
	refused(t, "null fee", err, apperr.ErrFieldRequired, "monthly_fee_cents")
	noStart := base()
	noStart.ContractStart = nil
	_, err = svc.Create(orgID, withRate(noStart))
	refused(t, "no start", err, apperr.ErrFieldRequired, "contract_start")
	blankStart := base()
	blankStart.ContractStart = ptr("  ")
	_, err = svc.Create(orgID, withRate(blankStart))
	refused(t, "blank start", err, apperr.ErrFieldRequired, "contract_start")

	// A mensalidade é maior que zero e cabe no teto.
	for _, cents := range []int{0, -1, validate.MaxCents + 1} {
		in := base()
		in.MonthlyFeeCents = opt(cents)
		_, err := svc.Create(orgID, withRate(in))
		refused(t, "fee out of range", err, customer.ErrInvalidMonthlyFee, "monthly_fee_cents")
	}
	one := base()
	one.MonthlyFeeCents = opt(1)
	if _, err := svc.Create(orgID, withRate(one)); err != nil {
		t.Errorf("a fee of one cent: %v", err)
	}

	// As datas são dias de calendário que existem, e o fim vem depois do início.
	for _, bad := range []string{"2026-02-30", "2026-1-5", "01/11/2026", "amanhã", "2026-13-01"} {
		in := base()
		in.ContractStart = ptr(bad)
		_, err := svc.Create(orgID, withRate(in))
		refused(t, "start "+bad, err, apperr.ErrFieldInvalid, "contract_start")
		in = base()
		in.ContractEnd = ptr(bad)
		_, err = svc.Create(orgID, withRate(in))
		refused(t, "end "+bad, err, apperr.ErrFieldInvalid, "contract_end")
	}
	for _, end := range []string{"2026-11-01", "2026-10-31", "2025-01-01"} {
		in := base()
		in.ContractEnd = ptr(end)
		_, err := svc.Create(orgID, withRate(in))
		refused(t, "end "+end, err, customer.ErrInvalidContractEnd, "contract_end")
	}

	in := base()
	in.ContractEnd = ptr("2026-11-02")
	in.ClosingDay, in.PaymentDays = ptr(10), ptr(5)
	in.LateFeeBP, in.LateInterestBPMonth, in.GraceDays = ptr(200), ptr(100), ptr(3)
	c, err := svc.Create(orgID, withRate(in))
	if err != nil {
		t.Fatalf("create monthly: %v", err)
	}
	got := mustGet(t, svc, c.ID.String())
	if got.BillingMode != customer.BillingMonthly || got.MonthlyFeeCents == nil || *got.MonthlyFeeCents != 630000 ||
		got.ContractStart == nil || *got.ContractStart != "2026-11-01" || got.ContractEnd == nil || *got.ContractEnd != "2026-11-02" ||
		got.ClosingDay != 10 || got.PaymentDays != 5 || got.LateFeeBP != 200 || got.LateInterestBPMonth != 100 || got.GraceDays != 3 ||
		got.HourlyRateCents != nil {
		t.Errorf("stored monthly customer = %+v", got)
	}

	// Mudar o início para depois do fim que já está lá é recusado no fim; texto vazio apaga o fim.
	_, err = svc.Update(c.ID.String(), customer.Input{ContractStart: ptr("2026-12-01")})
	refused(t, "start after the stored end", err, customer.ErrInvalidContractEnd, "contract_end")
	open, err := svc.Update(c.ID.String(), customer.Input{ContractEnd: ptr("")})
	if err != nil || open.ContractEnd != nil || open.ContractStart == nil {
		t.Errorf("clear the end: %+v, %v", open, err)
	}
	// Apagar o início ou a mensalidade de um mensal é recusado.
	_, err = svc.Update(c.ID.String(), customer.Input{ContractStart: ptr("")})
	refused(t, "clear the start", err, apperr.ErrFieldRequired, "contract_start")
	_, err = svc.Update(c.ID.String(), customer.Input{MonthlyFeeCents: validate.Optional[int]{Set: true}})
	refused(t, "clear the fee", err, apperr.ErrFieldRequired, "monthly_fee_cents")
}

func TestBilling_MonthlyLateTerms(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()
	for _, bp := range []int{-1, validate.MaxRateBP + 1} {
		_, err := svc.Update(id, customer.Input{LateFeeBP: ptr(bp)})
		refused(t, "late fee", err, customer.ErrInvalidLateFee, "late_fee_bp")
		_, err = svc.Update(id, customer.Input{LateInterestBPMonth: ptr(bp)})
		refused(t, "late interest", err, customer.ErrInvalidLateInterest, "late_interest_bp_month")
	}
	for _, days := range []int{-1, validate.MaxTermDays + 1} {
		_, err := svc.Update(id, customer.Input{GraceDays: ptr(days)})
		refused(t, "grace days", err, customer.ErrInvalidGraceDays, "grace_days")
	}
	got, err := svc.Update(id, customer.Input{LateFeeBP: ptr(validate.MaxRateBP), LateInterestBPMonth: ptr(0), GraceDays: ptr(validate.MaxTermDays)})
	if err != nil || got.LateFeeBP != validate.MaxRateBP || got.GraceDays != validate.MaxTermDays {
		t.Errorf("late terms at the cap: %+v, %v", got, err)
	}
}

// Num cliente mensal, o valor por hora padrão é recusado; o cliente mensal não tem valor por hora.
func TestBilling_MonthlyRefusesTheHourlyRate(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	_, err := svc.Create(orgID, withRate(customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
		HourlyRateCents: opt(15000),
	}))
	refused(t, "default rate on a monthly customer", err, customer.ErrFieldHourlyOnly, "hourly_rate_cents")

	// null é o padrão do campo, não um valor.
	if _, err := svc.Create(orgID, withRate(customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
		HourlyRateCents: validate.Optional[int]{Set: true},
	})); err != nil {
		t.Errorf("null default rate on a monthly customer: %v", err)
	}
}

// Trocar o modo de um cliente sem projetos apaga os termos do modo que saiu, e o cliente nunca guarda os dois.
func TestBilling_SwitchingTheModeClearsTheOtherTerms(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	hourly, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Troca"), HourlyRateCents: opt(15000), ClosingDay: ptr(10), PaymentDays: ptr(7)}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := hourly.ID.String()

	// Mandar o valor por hora junto do modo mensal é recusado; sem ele, o valor padrão fica para trás.
	_, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01"), HourlyRateCents: opt(15000)})
	refused(t, "switch carrying the rate", err, customer.ErrFieldHourlyOnly, "hourly_rate_cents")
	monthly, err := svc.Update(id, customer.Input{
		BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01"), LateFeeBP: ptr(200),
	})
	if err != nil || monthly.BillingMode != customer.BillingMonthly || monthly.HourlyRateCents != nil ||
		monthly.MonthlyFeeCents == nil || monthly.ClosingDay != 10 || monthly.PaymentDays != 7 || monthly.LateFeeBP != 200 {
		t.Fatalf("switch to monthly = %+v, %v; want the rate gone and the closing terms kept", monthly, err)
	}
	if got := mustGet(t, svc, id); got.HourlyRateCents != nil {
		t.Errorf("stored default rate = %v, want none", *got.HourlyRateCents)
	}

	// De volta ao por hora: o contrato inteiro sai.
	back, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly"), HourlyRateCents: opt(20000)})
	if err != nil || back.BillingMode != customer.BillingHourly || back.MonthlyFeeCents != nil || back.ContractStart != nil ||
		back.ContractEnd != nil || back.LateFeeBP != 0 || back.LateInterestBPMonth != 0 || back.GraceDays != 0 ||
		back.HourlyRateCents == nil || *back.HourlyRateCents != 20000 {
		t.Errorf("switch back to hourly = %+v, %v", back, err)
	}
	// Mandar o mesmo modo que já tem não é troca.
	if same, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly")}); err != nil || same.HourlyRateCents == nil {
		t.Errorf("the same mode again: %+v, %v; want it unchanged", same, err)
	}
}

// Um cliente que já tem projetos não troca de modo (issue #25); o resto dele continua editável.
func TestBilling_ModeLockedWithProjects(t *testing.T) {
	svc, projSvc, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Com projeto"), HourlyRateCents: opt(15000)}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()
	proj, _ := projSvc.Create(orgID, "Projeto", "", 0, project.Routine{})
	if _, err := projSvc.SetBilling(proj.ID.String(), &id, 0); err != nil {
		t.Fatalf("set billing: %v", err)
	}

	_, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")})
	refused(t, "switch with projects", err, customer.ErrModeLocked, "billing_mode")
	if got := mustGet(t, svc, id); got.BillingMode != customer.BillingHourly || got.HourlyRateCents == nil {
		t.Errorf("a locked switch changed the customer: %+v", got)
	}

	// Mandar o modo que ele já tem, e mexer nos outros termos, continua valendo.
	edited, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly"), HourlyRateCents: opt(18000), PaymentDays: ptr(10)})
	if err != nil || edited.HourlyRateCents == nil || *edited.HourlyRateCents != 18000 || edited.PaymentDays != 10 {
		t.Errorf("edit of a locked customer = %+v, %v", edited, err)
	}

	// Sem projetos o modo volta a trocar.
	if _, err := projSvc.SetBilling(proj.ID.String(), nil, 0); err != nil {
		t.Fatalf("clear billing: %v", err)
	}
	if got, err := svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")}); err != nil || got.BillingMode != customer.BillingMonthly {
		t.Errorf("switch without projects = %+v, %v", got, err)
	}
}

// receivableOf grava um recebível por hora do cliente, de setembro, como se o dono o tivesse confirmado.
func receivableOf(c *customer.Customer) *ent.Receivable {
	return testClient.Receivable.Create().
		SetOrganizationID(c.OrganizationID).SetCustomerID(c.ID).SetKind("hourly").
		SetPeriodStart("2026-09-01").SetPeriodEnd("2026-09-30").SetSeconds(0).
		SetCalculatedCents(10000).SetAmountCents(10000).SetCurrency("BRL").SetDueDate("2026-10-05").
		SaveX(context.Background())
}

// O dia de fechamento de um cliente com recebíveis não muda: os períodos novos cairiam por cima dos confirmados, e as
// mesmas horas poderiam ser cobradas duas vezes. Mandar o dia que ele já tem passa, o recebível de outro cliente não
// trava este, e desfeitos os recebíveis o dia muda.
func TestBilling_ClosingDayLockedWithReceivables(t *testing.T) {
	svc, _, orgID := setup(t)
	c, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Com recebível"), ClosingDay: ptr(20)}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()
	other, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Outro")}))
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	// Sem recebível o dia muda à vontade.
	if got, err := svc.Update(id, customer.Input{ClosingDay: ptr(25)}); err != nil || got.ClosingDay != 25 {
		t.Fatalf("closing day without receivables: %+v, %v", got, err)
	}

	rec := receivableOf(c)
	receivableOf(other) // o recebível do outro cliente não conta aqui

	_, err = svc.Update(id, customer.Input{ClosingDay: ptr(15)})
	refused(t, "another closing day", err, customer.ErrClosingDayLocked, "closing_day")
	if got := mustGet(t, svc, id); got.ClosingDay != 25 {
		t.Errorf("closing day = %d after a refused change, want 25", got.ClosingDay)
	}

	// O mesmo dia não muda nada e passa, também junto de outro campo.
	if got, err := svc.Update(id, customer.Input{ClosingDay: ptr(25), Name: ptr("Novo nome")}); err != nil || got.ClosingDay != 25 || got.Name != "Novo nome" {
		t.Errorf("same closing day with another field: %+v, %v", got, err)
	}
	// Um dia fora da faixa continua sendo o erro de faixa, não o da trava.
	_, err = svc.Update(id, customer.Input{ClosingDay: ptr(32)})
	refused(t, "closing day out of range", err, customer.ErrInvalidClosingDay, "closing_day")

	// Na criação o dia é livre: o cliente ainda não tem recebível.
	if created, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Novo"), ClosingDay: ptr(5)})); err != nil || created.ClosingDay != 5 {
		t.Errorf("create with a closing day: %+v, %v", created, err)
	}

	// O outro cliente tem o recebível dele e está travado do mesmo jeito.
	_, err = svc.Update(other.ID.String(), customer.Input{ClosingDay: ptr(10)})
	refused(t, "the other customer", err, customer.ErrClosingDayLocked, "closing_day")

	testClient.Receivable.DeleteOneID(rec.ID).ExecX(context.Background())
	if got, err := svc.Update(id, customer.Input{ClosingDay: ptr(15)}); err != nil || got.ClosingDay != 15 {
		t.Errorf("closing day after the receivable was undone: %+v, %v", got, err)
	}
}

// todayIn devolve hoje e amanhã, YYYY-MM-DD, no fuso da organização do teste: o "hoje" da âncora do ciclo de 14 dias.
func todayIn(t *testing.T, orgID string) (today, tomorrow string) {
	t.Helper()
	org := testClient.Organization.GetX(context.Background(), uuid.MustParse(orgID))
	loc, err := time.LoadLocation(org.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	return now.Format(time.DateOnly), now.AddDate(0, 0, 1).Format(time.DateOnly)
}

// biweekly é o corpo de um cliente por hora no ciclo de 14 dias, com a âncora dada.
func biweekly(name, anchor string) customer.Input {
	return withRate(customer.Input{Name: ptr(name), BillingCycle: ptr("biweekly"), CycleAnchor: ptr(anchor)})
}

// O ciclo de 14 dias do cliente por hora: guarda a âncora, sai assim do Get e da lista, não muda quando só outro campo
// vem, e voltar ao mensal (ou trocar o modo para o contrato mensal) apaga a âncora.
func TestBilling_BiweeklyCycle(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	in := biweekly("Acme Inc.", "2026-10-27")
	in.PaymentDays = ptr(3)
	created, err := svc.Create(orgID, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := created.ID.String()
	isBiweekly := func(c customer.Customer, anchor string) bool {
		return c.BillingCycle == customer.CycleBiweekly && c.CycleAnchor != nil && *c.CycleAnchor == anchor
	}
	if got := mustGet(t, svc, id); !isBiweekly(*got, "2026-10-27") || got.PaymentDays != 3 || got.ClosingDay != 31 {
		t.Errorf("got = %+v, want biweekly from 2026-10-27, due in 3 days", got)
	}
	if list, err := svc.ListByOrg(orgID); err != nil || len(list) != 1 || !isBiweekly(list[0], "2026-10-27") {
		t.Errorf("list = %+v, %v", list, err)
	}

	// Só o nome, ou o mesmo ciclo e a mesma âncora de novo, não mexem no ciclo.
	if got, err := svc.Update(id, customer.Input{Name: ptr("Acme")}); err != nil || !isBiweekly(*got, "2026-10-27") {
		t.Errorf("update name = %+v, %v", got, err)
	}
	if got, err := svc.Update(id, customer.Input{BillingCycle: ptr("biweekly"), CycleAnchor: ptr("2026-10-27")}); err != nil || !isBiweekly(*got, "2026-10-27") {
		t.Errorf("same cycle again = %+v, %v", got, err)
	}
	// Uma âncora já guardada não é conferida de novo: só a que muda não pode passar de hoje.
	_, tomorrow := todayIn(t, orgID)
	testClient.Customer.UpdateOneID(created.ID).SetCycleAnchor(tomorrow).ExecX(context.Background())
	if got, err := svc.Update(id, customer.Input{Name: ptr("Acme Inc."), BillingCycle: ptr("biweekly"), CycleAnchor: ptr(tomorrow)}); err != nil || !isBiweekly(*got, tomorrow) {
		t.Errorf("the stored anchor sent again = %+v, %v", got, err)
	}
	if got, err := svc.Update(id, customer.Input{CycleAnchor: ptr("2026-11-03")}); err != nil || !isBiweekly(*got, "2026-11-03") {
		t.Errorf("move the anchor = %+v, %v", got, err)
	}

	// De volta ao mensal, como a tela manda: o ciclo mensal e a âncora vazia.
	got, err := svc.Update(id, customer.Input{BillingCycle: ptr("monthly"), CycleAnchor: ptr("")})
	if err != nil || got.BillingCycle != customer.CycleMonthly || got.CycleAnchor != nil {
		t.Errorf("back to monthly = %+v, %v", got, err)
	}
	if got := mustGet(t, svc, id); got.BillingCycle != customer.CycleMonthly || got.CycleAnchor != nil {
		t.Errorf("stored after back to monthly = %+v", got)
	}

	// Trocar o modo para o contrato mensal leva o ciclo junto, mesmo sem dizer o ciclo.
	if _, err := svc.Update(id, customer.Input{BillingCycle: ptr("biweekly"), CycleAnchor: ptr("2026-10-27")}); err != nil {
		t.Fatalf("biweekly again: %v", err)
	}
	got, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")})
	if err != nil || got.BillingMode != customer.BillingMonthly || got.BillingCycle != customer.CycleMonthly || got.CycleAnchor != nil {
		t.Errorf("switch to the monthly contract = %+v, %v", got, err)
	}
}

// O que o ciclo recusa, cada recusa com o campo.
func TestBilling_BiweeklyRefusals(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	today, tomorrow := todayIn(t, orgID)
	create := func(in customer.Input) error {
		_, err := svc.Create(orgID, in)
		return err
	}
	monthly := func(in customer.Input) customer.Input {
		in.Name, in.BillingMode = ptr("Contrato"), ptr("monthly")
		in.MonthlyFeeCents, in.ContractStart = opt(500000), ptr("2026-11-01")
		return in
	}

	refused(t, "unknown cycle", create(withRate(customer.Input{Name: ptr("X"), BillingCycle: ptr("weekly")})), customer.ErrInvalidBillingCycle, "billing_cycle")
	refused(t, "empty cycle", create(withRate(customer.Input{Name: ptr("X"), BillingCycle: ptr("")})), customer.ErrInvalidBillingCycle, "billing_cycle")
	refused(t, "biweekly without an anchor", create(withRate(customer.Input{Name: ptr("X"), BillingCycle: ptr("biweekly")})), apperr.ErrFieldRequired, "cycle_anchor")
	refused(t, "biweekly with an empty anchor", create(biweekly("X", "")), apperr.ErrFieldRequired, "cycle_anchor")
	refused(t, "anchor in another format", create(biweekly("X", "27/10/2026")), apperr.ErrFieldInvalid, "cycle_anchor")
	refused(t, "anchor that does not exist", create(biweekly("X", "2026-02-30")), apperr.ErrFieldInvalid, "cycle_anchor")
	refused(t, "anchor tomorrow", create(biweekly("X", tomorrow)), customer.ErrInvalidCycleAnchor, "cycle_anchor")
	refused(t, "anchor on the monthly cycle", create(withRate(customer.Input{Name: ptr("X"), CycleAnchor: ptr("2026-10-27")})), customer.ErrFieldBiweeklyOnly, "cycle_anchor")
	refused(t, "biweekly on the monthly contract", create(monthly(customer.Input{BillingCycle: ptr("biweekly")})), customer.ErrFieldHourlyOnly, "billing_cycle")
	refused(t, "anchor on the monthly contract", create(monthly(customer.Input{CycleAnchor: ptr("2026-10-27")})), customer.ErrFieldBiweeklyOnly, "cycle_anchor")
	if list, _ := svc.ListByOrg(orgID); len(list) != 0 {
		t.Fatalf("a refused create left %d customers", len(list))
	}

	// O que passa: a âncora de hoje, e o contrato mensal com o ciclo mensal e a âncora vazia, como a tela manda.
	if c, err := svc.Create(orgID, biweekly("Hoje", today)); err != nil || c.CycleAnchor == nil || *c.CycleAnchor != today {
		t.Errorf("anchor today = %+v, %v", c, err)
	}
	if c, err := svc.Create(orgID, monthly(customer.Input{BillingCycle: ptr("monthly"), CycleAnchor: ptr("")})); err != nil || c.BillingCycle != customer.CycleMonthly {
		t.Errorf("monthly contract with the defaults spelled out = %+v, %v", c, err)
	}
}

// Com recebíveis no cliente, o ciclo e a âncora travam como o dia de fechamento: os períodos novos não podem cair por
// cima dos confirmados. Passar para o de 14 dias com a âncora no fim do último recebível, ou depois, é aceito.
func TestBilling_CycleLockedWithReceivables(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Com recebível")}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()
	free, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Sem recebível")}))
	if err != nil {
		t.Fatalf("create free: %v", err)
	}
	toBiweekly := func(id, anchor string) (*customer.Customer, error) {
		return svc.Update(id, customer.Input{BillingCycle: ptr("biweekly"), CycleAnchor: ptr(anchor)})
	}
	toMonthly := func(id string) (*customer.Customer, error) {
		return svc.Update(id, customer.Input{BillingCycle: ptr("monthly"), CycleAnchor: ptr("")})
	}

	// Sem recebível o ciclo muda à vontade.
	if _, err := toBiweekly(id, "2026-09-15"); err != nil {
		t.Fatalf("biweekly without receivables: %v", err)
	}
	if _, err := toMonthly(id); err != nil {
		t.Fatalf("monthly without receivables: %v", err)
	}

	rec := receivableOf(c) // 2026-09-01 a 2026-09-30

	_, err = toBiweekly(id, "2026-09-29")
	refused(t, "anchor before the last receivable", err, customer.ErrCycleLocked, "cycle_anchor")
	if got := mustGet(t, svc, id); got.BillingCycle != customer.CycleMonthly || got.CycleAnchor != nil {
		t.Errorf("stored after a refused change = %+v", got)
	}
	if got, err := toBiweekly(id, "2026-09-30"); err != nil || got.BillingCycle != customer.CycleBiweekly {
		t.Errorf("anchor on the end of the last receivable = %+v, %v", got, err)
	}
	_, err = toBiweekly(id, "2026-09-15")
	refused(t, "anchor moved back", err, customer.ErrCycleLocked, "cycle_anchor")
	if got, err := toBiweekly(id, "2026-10-10"); err != nil || *got.CycleAnchor != "2026-10-10" {
		t.Errorf("anchor moved forward = %+v, %v", got, err)
	}
	// O mesmo ciclo e a mesma âncora passam junto de outro campo.
	if got, err := svc.Update(id, customer.Input{Name: ptr("Novo nome"), BillingCycle: ptr("biweekly"), CycleAnchor: ptr("2026-10-10")}); err != nil || got.Name != "Novo nome" {
		t.Errorf("same cycle with another field = %+v, %v", got, err)
	}

	_, err = toMonthly(id)
	refused(t, "back to monthly", err, customer.ErrCycleLocked, "billing_cycle")
	_, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")})
	refused(t, "switch to the monthly contract", err, customer.ErrCycleLocked, "billing_cycle")
	if got := mustGet(t, svc, id); got.BillingMode != customer.BillingHourly || got.BillingCycle != customer.CycleBiweekly {
		t.Errorf("stored after the refused switches = %+v", got)
	}

	// O recebível deste cliente não trava o outro.
	if _, err := toBiweekly(free.ID.String(), "2026-09-01"); err != nil {
		t.Errorf("the other customer: %v", err)
	}

	testClient.Receivable.DeleteOneID(rec.ID).ExecX(context.Background())
	if got, err := toMonthly(id); err != nil || got.BillingCycle != customer.CycleMonthly {
		t.Errorf("back to monthly after the receivable was undone = %+v, %v", got, err)
	}
}

// Um cliente com recebíveis não é excluído; desfeitos os recebíveis, é.
func TestBilling_DeleteRefusedWithReceivables(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, withRate(customer.Input{Name: ptr("Com recebível"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ctx := context.Background()
	rec := testClient.Receivable.Create().
		SetOrganizationID(c.OrganizationID).SetCustomerID(c.ID).SetKind("monthly").
		SetPeriodStart("2026-11-01").SetPeriodEnd("2026-11-30").SetSeconds(0).
		SetCalculatedCents(630000).SetAmountCents(630000).SetCurrency("BRL").SetDueDate("2026-12-05").
		SaveX(ctx)

	err = svc.Delete(c.ID.String())
	if !errors.Is(err, customer.ErrHasReceivables) {
		t.Fatalf("delete with receivables: err = %v, want ErrHasReceivables", err)
	}
	if _, err := svc.Get(c.ID.String()); err != nil {
		t.Errorf("the customer is gone after a refused delete: %v", err)
	}

	testClient.Receivable.DeleteOneID(rec.ID).ExecX(ctx)
	if err := svc.Delete(c.ID.String()); err != nil {
		t.Errorf("delete after the receivable was undone: %v", err)
	}
}

// A estimativa de horas por mês é do cliente mensal, opcional, de 1 a validate.MaxMonthlyHours; null apaga, o por hora
// a recusa e a troca de modo a apaga (Decisão 26).
func TestBilling_EstimatedMonthlyHours(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	base := func() customer.Input {
		return customer.Input{Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")}
	}

	// Sem a estimativa o cliente mensal continua valendo: ela é opcional.
	plain, err := svc.Create(orgID, base())
	if err != nil || plain.EstimatedMonthlyHours != nil {
		t.Fatalf("monthly without an estimate = %+v, %v", plain, err)
	}

	for _, hours := range []int{0, -1, validate.MaxMonthlyHours + 1} {
		in := base()
		in.EstimatedMonthlyHours = opt(hours)
		_, err := svc.Create(orgID, in)
		refused(t, "estimate out of range", err, customer.ErrInvalidEstimatedHours, "estimated_monthly_hours")
	}
	for _, hours := range []int{1, validate.MaxMonthlyHours} {
		in := base()
		in.EstimatedMonthlyHours = opt(hours)
		c, err := svc.Create(orgID, in)
		if err != nil || c.EstimatedMonthlyHours == nil || *c.EstimatedMonthlyHours != hours {
			t.Errorf("estimate %d = %+v, %v", hours, c, err)
		}
	}

	in := base()
	in.EstimatedMonthlyHours = opt(40)
	c, err := svc.Create(orgID, in)
	if err != nil {
		t.Fatalf("create with an estimate: %v", err)
	}
	id := c.ID.String()
	if got := mustGet(t, svc, id); got.EstimatedMonthlyHours == nil || *got.EstimatedMonthlyHours != 40 {
		t.Fatalf("stored estimate = %v, want 40", got.EstimatedMonthlyHours)
	}
	// Ausente mantém; outro valor troca; null apaga.
	if kept, err := svc.Update(id, customer.Input{Name: ptr("Mensal 2")}); err != nil || kept.EstimatedMonthlyHours == nil || *kept.EstimatedMonthlyHours != 40 {
		t.Errorf("update without the estimate = %+v, %v; want it kept", kept, err)
	}
	if changed, err := svc.Update(id, customer.Input{EstimatedMonthlyHours: opt(55)}); err != nil || changed.EstimatedMonthlyHours == nil || *changed.EstimatedMonthlyHours != 55 {
		t.Errorf("new estimate = %+v, %v; want 55", changed, err)
	}
	if cleared, err := svc.Update(id, customer.Input{EstimatedMonthlyHours: validate.Optional[int]{Set: true}}); err != nil || cleared.EstimatedMonthlyHours != nil {
		t.Errorf("null estimate = %+v, %v; want it cleared", cleared, err)
	}
	if got := mustGet(t, svc, id); got.EstimatedMonthlyHours != nil {
		t.Errorf("stored estimate after null = %v, want none", *got.EstimatedMonthlyHours)
	}

	// O cliente por hora não tem estimativa: com valor é recusada; null passa, como manda a tela.
	_, err = svc.Create(orgID, customer.Input{Name: ptr("Hora"), HourlyRateCents: opt(15000), EstimatedMonthlyHours: opt(40)})
	refused(t, "hourly with an estimate", err, customer.ErrFieldMonthlyOnly, "estimated_monthly_hours")
	if _, err := svc.Create(orgID, customer.Input{Name: ptr("Hora"), HourlyRateCents: opt(15000), EstimatedMonthlyHours: validate.Optional[int]{Set: true}}); err != nil {
		t.Errorf("hourly with a null estimate: %v", err)
	}

	// Trocar o mensal (sem projetos) para o por hora apaga a estimativa com o resto do contrato.
	if _, err := svc.Update(id, customer.Input{EstimatedMonthlyHours: opt(30)}); err != nil {
		t.Fatalf("set the estimate again: %v", err)
	}
	back, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly"), HourlyRateCents: opt(20000)})
	if err != nil || back.EstimatedMonthlyHours != nil {
		t.Errorf("switch to hourly = %+v, %v; want the estimate gone", back, err)
	}
}
