package customer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

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

// Um cliente sem termos é por hora, fecha no último dia e vence no fechamento, e é assim que sai da lista e do Get. É o
// que todo cliente de antes dos termos é.
func TestBilling_DefaultsAreTheOldBehaviour(t *testing.T) {
	svc, _, orgID := setup(t)
	created, err := svc.Create(orgID, customer.Input{Name: ptr("Empresa A")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for name, c := range map[string]*customer.Customer{"created": created, "got": mustGet(t, svc, created.ID.String())} {
		if c.BillingMode != customer.BillingHourly || c.ClosingDay != 31 || c.PaymentDays != 0 ||
			c.DefaultRateCents != nil || c.MonthlyFeeCents != nil || c.ContractStart != nil || c.ContractEnd != nil ||
			c.LateFeeBP != 0 || c.LateInterestBPMonth != 0 || c.GraceDays != 0 {
			t.Errorf("%s = %+v, want hourly, closing day 31 and nothing else", name, c)
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

	// O valor por hora padrão tem o piso de 10,00 (e o teto) quando vem.
	refused(t, "rate under the floor", func() error {
		_, err := svc.Create(orgID, customer.Input{Name: ptr("X"), DefaultRateCents: opt(validate.MinRateCents - 1)})
		return err
	}(), customer.ErrInvalidDefaultRate, "default_rate_cents")
	refused(t, "rate zero", func() error {
		_, err := svc.Create(orgID, customer.Input{Name: ptr("X"), DefaultRateCents: opt(0)})
		return err
	}(), customer.ErrInvalidDefaultRate, "default_rate_cents")
	refused(t, "rate over the cap", func() error {
		_, err := svc.Create(orgID, customer.Input{Name: ptr("X"), DefaultRateCents: opt(validate.MaxCents + 1)})
		return err
	}(), customer.ErrInvalidDefaultRate, "default_rate_cents")

	c, err := svc.Create(orgID, customer.Input{
		Name: ptr("Hora"), DefaultRateCents: opt(validate.MinRateCents), ClosingDay: ptr(5), PaymentDays: ptr(15),
	})
	if err != nil {
		t.Fatalf("create at the floor: %v", err)
	}
	if c.DefaultRateCents == nil || *c.DefaultRateCents != 1000 || c.ClosingDay != 5 || c.PaymentDays != 15 {
		t.Errorf("created = %+v", c)
	}
	if got := mustGet(t, svc, c.ID.String()); got.DefaultRateCents == nil || *got.DefaultRateCents != 1000 || got.ClosingDay != 5 || got.PaymentDays != 15 {
		t.Errorf("stored = %+v", got)
	}

	// null apaga o valor padrão; ausente mantém.
	kept, err := svc.Update(c.ID.String(), customer.Input{Name: ptr("Hora 2")})
	if err != nil || kept.DefaultRateCents == nil || *kept.DefaultRateCents != 1000 {
		t.Errorf("absent keeps the rate: %+v, %v", kept, err)
	}
	cleared, err := svc.Update(c.ID.String(), customer.Input{DefaultRateCents: validate.Optional[int]{Set: true}})
	if err != nil || cleared.DefaultRateCents != nil {
		t.Errorf("null clears the rate: %+v, %v", cleared, err)
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
		_, err := svc.Create(orgID, c.in)
		refused(t, "create with "+c.field, err, customer.ErrFieldMonthlyOnly, c.field)
	}

	got, err := svc.Create(orgID, customer.Input{
		Name: ptr("Tudo vazio"), BillingMode: ptr("hourly"),
		MonthlyFeeCents: validate.Optional[int]{Set: true}, ContractStart: ptr(""), ContractEnd: ptr("  "),
		LateFeeBP: ptr(0), LateInterestBPMonth: ptr(0), GraceDays: ptr(0),
	})
	if err != nil || got.BillingMode != customer.BillingHourly || got.MonthlyFeeCents != nil || got.ContractStart != nil {
		t.Errorf("empty monthly fields on an hourly customer: %+v, %v", got, err)
	}
}

func TestBilling_BillingModeValue(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	for _, mode := range []string{"", "weekly", "MONTHLY", "hour"} {
		_, err := svc.Create(orgID, customer.Input{Name: ptr("X"), BillingMode: ptr(mode)})
		refused(t, "mode "+mode, err, customer.ErrInvalidBillingMode, "billing_mode")
	}
}

// O mensal só vale numa organização solo: na criação e na troca de modo.
func TestBilling_MonthlyNeedsSolo(t *testing.T) {
	svc, _, orgID := setup(t)
	monthly := customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
	}
	_, err := svc.Create(orgID, monthly)
	refused(t, "create monthly outside solo", err, customer.ErrMonthlyNeedsSolo, "billing_mode")

	hourly, err := svc.Create(orgID, customer.Input{Name: ptr("Hora")})
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
	created, err := svc.Create(orgID, monthly)
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
	_, err := svc.Create(orgID, noFee)
	refused(t, "no fee", err, apperr.ErrFieldRequired, "monthly_fee_cents")
	nullFee := base()
	nullFee.MonthlyFeeCents = validate.Optional[int]{Set: true}
	_, err = svc.Create(orgID, nullFee)
	refused(t, "null fee", err, apperr.ErrFieldRequired, "monthly_fee_cents")
	noStart := base()
	noStart.ContractStart = nil
	_, err = svc.Create(orgID, noStart)
	refused(t, "no start", err, apperr.ErrFieldRequired, "contract_start")
	blankStart := base()
	blankStart.ContractStart = ptr("  ")
	_, err = svc.Create(orgID, blankStart)
	refused(t, "blank start", err, apperr.ErrFieldRequired, "contract_start")

	// A mensalidade é maior que zero e cabe no teto.
	for _, cents := range []int{0, -1, validate.MaxCents + 1} {
		in := base()
		in.MonthlyFeeCents = opt(cents)
		_, err := svc.Create(orgID, in)
		refused(t, "fee out of range", err, customer.ErrInvalidMonthlyFee, "monthly_fee_cents")
	}
	one := base()
	one.MonthlyFeeCents = opt(1)
	if _, err := svc.Create(orgID, one); err != nil {
		t.Errorf("a fee of one cent: %v", err)
	}

	// As datas são dias de calendário que existem, e o fim vem depois do início.
	for _, bad := range []string{"2026-02-30", "2026-1-5", "01/11/2026", "amanhã", "2026-13-01"} {
		in := base()
		in.ContractStart = ptr(bad)
		_, err := svc.Create(orgID, in)
		refused(t, "start "+bad, err, apperr.ErrFieldInvalid, "contract_start")
		in = base()
		in.ContractEnd = ptr(bad)
		_, err = svc.Create(orgID, in)
		refused(t, "end "+bad, err, apperr.ErrFieldInvalid, "contract_end")
	}
	for _, end := range []string{"2026-11-01", "2026-10-31", "2025-01-01"} {
		in := base()
		in.ContractEnd = ptr(end)
		_, err := svc.Create(orgID, in)
		refused(t, "end "+end, err, customer.ErrInvalidContractEnd, "contract_end")
	}

	in := base()
	in.ContractEnd = ptr("2026-11-02")
	in.ClosingDay, in.PaymentDays = ptr(10), ptr(5)
	in.LateFeeBP, in.LateInterestBPMonth, in.GraceDays = ptr(200), ptr(100), ptr(3)
	c, err := svc.Create(orgID, in)
	if err != nil {
		t.Fatalf("create monthly: %v", err)
	}
	got := mustGet(t, svc, c.ID.String())
	if got.BillingMode != customer.BillingMonthly || got.MonthlyFeeCents == nil || *got.MonthlyFeeCents != 630000 ||
		got.ContractStart == nil || *got.ContractStart != "2026-11-01" || got.ContractEnd == nil || *got.ContractEnd != "2026-11-02" ||
		got.ClosingDay != 10 || got.PaymentDays != 5 || got.LateFeeBP != 200 || got.LateInterestBPMonth != 100 || got.GraceDays != 3 ||
		got.DefaultRateCents != nil {
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
	c, err := svc.Create(orgID, customer.Input{Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")})
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
	_, err := svc.Create(orgID, customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
		DefaultRateCents: opt(15000),
	})
	refused(t, "default rate on a monthly customer", err, customer.ErrFieldHourlyOnly, "default_rate_cents")

	// null é o padrão do campo, não um valor.
	if _, err := svc.Create(orgID, customer.Input{
		Name: ptr("Mensal"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01"),
		DefaultRateCents: validate.Optional[int]{Set: true},
	}); err != nil {
		t.Errorf("null default rate on a monthly customer: %v", err)
	}
}

// Trocar o modo de um cliente sem projetos apaga os termos do modo que saiu, e o cliente nunca guarda os dois.
func TestBilling_SwitchingTheModeClearsTheOtherTerms(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	hourly, err := svc.Create(orgID, customer.Input{Name: ptr("Troca"), DefaultRateCents: opt(15000), ClosingDay: ptr(10), PaymentDays: ptr(7)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := hourly.ID.String()

	// Mandar o valor por hora junto do modo mensal é recusado; sem ele, o valor padrão fica para trás.
	_, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01"), DefaultRateCents: opt(15000)})
	refused(t, "switch carrying the rate", err, customer.ErrFieldHourlyOnly, "default_rate_cents")
	monthly, err := svc.Update(id, customer.Input{
		BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01"), LateFeeBP: ptr(200),
	})
	if err != nil || monthly.BillingMode != customer.BillingMonthly || monthly.DefaultRateCents != nil ||
		monthly.MonthlyFeeCents == nil || monthly.ClosingDay != 10 || monthly.PaymentDays != 7 || monthly.LateFeeBP != 200 {
		t.Fatalf("switch to monthly = %+v, %v; want the rate gone and the closing terms kept", monthly, err)
	}
	if got := mustGet(t, svc, id); got.DefaultRateCents != nil {
		t.Errorf("stored default rate = %v, want none", *got.DefaultRateCents)
	}

	// De volta ao por hora: o contrato inteiro sai.
	back, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly"), DefaultRateCents: opt(20000)})
	if err != nil || back.BillingMode != customer.BillingHourly || back.MonthlyFeeCents != nil || back.ContractStart != nil ||
		back.ContractEnd != nil || back.LateFeeBP != 0 || back.LateInterestBPMonth != 0 || back.GraceDays != 0 ||
		back.DefaultRateCents == nil || *back.DefaultRateCents != 20000 {
		t.Errorf("switch back to hourly = %+v, %v", back, err)
	}
	// Mandar o mesmo modo que já tem não é troca.
	if same, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly")}); err != nil || same.DefaultRateCents == nil {
		t.Errorf("the same mode again: %+v, %v; want it unchanged", same, err)
	}
}

// Um cliente que já tem projetos não troca de modo (issue #25); o resto dele continua editável.
func TestBilling_ModeLockedWithProjects(t *testing.T) {
	svc, projSvc, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, customer.Input{Name: ptr("Com projeto"), DefaultRateCents: opt(15000)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := c.ID.String()
	proj, _ := projSvc.Create(orgID, "Projeto", "", 0, project.Routine{})
	if _, err := projSvc.SetBilling(proj.ID.String(), &id, ptr(15000)); err != nil {
		t.Fatalf("set billing: %v", err)
	}

	_, err = svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")})
	refused(t, "switch with projects", err, customer.ErrModeLocked, "billing_mode")
	if got := mustGet(t, svc, id); got.BillingMode != customer.BillingHourly || got.DefaultRateCents == nil {
		t.Errorf("a locked switch changed the customer: %+v", got)
	}

	// Mandar o modo que ele já tem, e mexer nos outros termos, continua valendo.
	edited, err := svc.Update(id, customer.Input{BillingMode: ptr("hourly"), DefaultRateCents: opt(18000), PaymentDays: ptr(10)})
	if err != nil || edited.DefaultRateCents == nil || *edited.DefaultRateCents != 18000 || edited.PaymentDays != 10 {
		t.Errorf("edit of a locked customer = %+v, %v", edited, err)
	}

	// Sem projetos o modo volta a trocar.
	if _, err := projSvc.SetBilling(proj.ID.String(), nil, nil); err != nil {
		t.Fatalf("clear billing: %v", err)
	}
	if got, err := svc.Update(id, customer.Input{BillingMode: ptr("monthly"), MonthlyFeeCents: opt(500000), ContractStart: ptr("2026-11-01")}); err != nil || got.BillingMode != customer.BillingMonthly {
		t.Errorf("switch without projects = %+v, %v", got, err)
	}
}

// Um cliente com recebíveis não é excluído; desfeitos os recebíveis, é.
func TestBilling_DeleteRefusedWithReceivables(t *testing.T) {
	svc, _, orgID := setup(t)
	setSolo(t, orgID, true)
	c, err := svc.Create(orgID, customer.Input{Name: ptr("Com recebível"), BillingMode: ptr("monthly"), MonthlyFeeCents: opt(630000), ContractStart: ptr("2026-11-01")})
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
