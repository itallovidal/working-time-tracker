// Package receivable guarda as contas a receber do modo solo: o dono confirma o que vai cobrar de cada cliente por
// período, registra quando o dinheiro entrou e desfaz o que errou. O período e a proposta são calculados na hora, do
// modo de cobrança do cliente (por hora, das sessões de todos os projetos dele; ou mensal, do contrato); o que fica
// gravado é só a confirmação e o recebimento, como a baixa de payment (Decisão 18).
package receivable

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/apperr"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/validate"
)

// Deps são o que o serviço usa.
type Deps struct {
	Store *Store
	// Now é o relógio da conta; sem ele vale time.Now. Os testes o fixam.
	Now func() time.Time
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now()
	}
	return time.Now()
}

// Solo diz se a organização trabalha no modo solo. As rotas deste domínio só valem para ela; uma organização que não
// existe vale como não solo.
func (s *Service) Solo(orgID string) bool {
	org, err := s.deps.Store.Org(orgID)
	return err == nil && org.Solo
}

// env é o que toda conta precisa de uma organização e do instante em que se pergunta.
type env struct {
	id    uuid.UUID
	org   Org
	loc   *time.Location
	now   time.Time
	today string // o dia de hoje, YYYY-MM-DD, no fuso da organização
}

func (s *Service) env(orgID string) (env, error) {
	id, err := uuid.Parse(orgID)
	if err != nil {
		return env{}, database.ErrNotFound
	}
	org, err := s.deps.Store.Org(orgID)
	if err != nil {
		return env{}, err
	}
	loc := location(org.Timezone)
	now := s.now()
	return env{id: id, org: org, loc: loc, now: now, today: now.In(loc).Format(time.DateOnly)}, nil
}

// Proposals devolve o que se propõe cobrar dos clientes da organização, do período que fecha primeiro para o que
// fecha depois: no cliente por hora, os períodos fechados com tempo ainda sem recebível (os mais recentes, até
// MaxHourlyProposals por cliente no ciclo mensal e MaxBiweeklyProposals no de 14 dias); no mensal, os períodos do
// contrato ainda sem recebível. Com includeOpen entra também
// o período corrente dos clientes por hora, com as horas até agora (Open verdadeiro). Nada é gravado.
func (s *Service) Proposals(orgID string, includeOpen bool) ([]Proposal, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.deps.Store.Customers(e.id)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Store.List(e.id)
	if err != nil {
		return nil, err
	}
	return s.proposalsOf(e, customers, doneOf(rows), includeOpen)
}

// proposalsOf propõe os períodos dos clientes, que o chamador já leu, descontando os confirmados (done).
func (s *Service) proposalsOf(e env, customers []*ent.Customer, done map[string]bool, includeOpen bool) ([]Proposal, error) {
	var billable []uuid.UUID
	for _, c := range customers {
		if c.BillingMode == KindHourly || c.BillingMode == KindMonthly {
			billable = append(billable, c.ID)
		}
	}
	sessions, err := s.deps.Store.SessionsByCustomer(e.id, billable)
	if err != nil {
		return nil, err
	}
	out := []Proposal{}
	for _, c := range customers {
		out = append(out, termsOf(c, e.loc).proposals(sessions[c.ID], done, e.now, includeOpen)...)
	}
	sortProposals(out)
	return out, nil
}

// doneOf junta os períodos que já têm recebível, por cliente.
func doneOf(rows []*ent.Receivable) map[string]bool {
	done := make(map[string]bool, len(rows))
	for _, r := range rows {
		done[key(r.CustomerID, r.PeriodStart, r.PeriodEnd)] = true
	}
	return done
}

// Receivables devolve os recebíveis da organização, todos, com o atraso e o valor por hora efetivo já calculados para
// hoje, em ordem de vencimento.
func (s *Service) Receivables(orgID string) ([]Receivable, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.deps.Store.Customers(e.id)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Store.List(e.id)
	if err != nil {
		return nil, err
	}
	return viewsOf(e, rows, namesOf(customers)), nil
}

func namesOf(customers []*ent.Customer) map[uuid.UUID]string {
	names := make(map[uuid.UUID]string, len(customers))
	for _, c := range customers {
		names[c.ID] = c.Name
	}
	return names
}

func viewsOf(e env, rows []*ent.Receivable, names map[uuid.UUID]string) []Receivable {
	out := make([]Receivable, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(e, r, names[r.CustomerID]))
	}
	slices.SortFunc(out, func(a, b Receivable) int {
		if c := strings.Compare(a.DueDate, b.DueDate); c != 0 {
			return c
		}
		if c := strings.Compare(a.Customer.Name, b.Customer.Name); c != 0 {
			return c
		}
		return strings.Compare(a.PeriodEnd, b.PeriodEnd)
	})
	return out
}

// view monta o recebível para a tela. O atraso é derivado: confirmado, sem recebimento e com hoje depois do vencimento.
func view(e env, r *ent.Receivable, customerName string) Receivable {
	v := Receivable{
		ID: r.ID, Customer: Ref{ID: r.CustomerID, Name: customerName}, Kind: r.Kind,
		PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd,
		Seconds: r.Seconds, CalculatedCents: r.CalculatedCents, AmountCents: r.AmountCents,
		Currency: r.Currency, DueDate: r.DueDate, InvoiceNumber: r.InvoiceNumber, Note: r.Note,
		ConfirmedAt: r.ConfirmedAt, ReceivedOn: r.ReceivedOn, ReceivedCents: r.ReceivedCents,
		LateFeeCents: r.LateFeeCents, InterestCents: r.InterestCents,
	}
	if r.ReceivedOn == nil {
		v.Late, v.DaysLate = lateOn(e, r.DueDate)
	}
	v.EffectiveRateCents, v.BelowMinRate = effectiveRate(r.AmountCents, r.Seconds)
	return v
}

// lateOn diz se uma conta que vence em due está atrasada hoje (hoje depois do vencimento) e há quantos dias.
func lateOn(e env, due string) (bool, int) {
	if e.today > due {
		return true, daysBetween(due, e.today)
	}
	return false, 0
}

// dueOfProposal é o período calculado e já fechado na lista A receber: nada gravado, o valor é o calculado.
func dueOfProposal(e env, p Proposal) Due {
	d := Due{
		Customer: p.Customer, Kind: p.Kind, PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd,
		Seconds: p.Seconds, CalculatedCents: p.CalculatedCents, AmountCents: p.CalculatedCents, ExtraCents: p.ExtraCents,
		DueDate: p.DueDate, EffectiveRateCents: p.EffectiveRateCents, BelowMinRate: p.BelowMinRate,
	}
	d.Late, d.DaysLate = lateOn(e, p.DueDate)
	return d
}

// dueOfReceivable é o recebível gravado e não recebido na lista A receber, com o atraso que view já calculou.
func dueOfReceivable(r Receivable) Due {
	id := r.ID
	return Due{
		ID: &id, Adjusted: true, Customer: r.Customer, Kind: r.Kind, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd,
		Seconds: r.Seconds, CalculatedCents: r.CalculatedCents, AmountCents: r.AmountCents,
		DueDate: r.DueDate, InvoiceNumber: r.InvoiceNumber, Note: r.Note, Late: r.Late, DaysLate: r.DaysLate,
		EffectiveRateCents: r.EffectiveRateCents, BelowMinRate: r.BelowMinRate,
	}
}

// Summary monta a página A receber: os cartões e as três listas. Em andamento são os períodos ainda abertos (o mês
// corrente do contrato mensal e o período corrente do por hora, com as horas até agora); A receber, o que fechou e não
// entrou, calculado ou gravado; Recebidos, o histórico. O recebimento é do dono, e esta leitura também.
func (s *Service) Summary(orgID string) (*Summary, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.deps.Store.Customers(e.id)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.Store.List(e.id)
	if err != nil {
		return nil, err
	}
	proposals, err := s.proposalsOf(e, customers, doneOf(rows), true)
	if err != nil {
		return nil, err
	}
	out := &Summary{
		GeneratedAt: e.now, Timezone: e.org.Timezone, Currency: e.org.Currency, Today: e.today,
		InProgress: []Due{}, ToReceive: []Due{}, Received: []Receivable{},
	}
	// Um período está em andamento enquanto não fechou: hoje ainda é o último dia dele, ou antes. Vale para o calculado e
	// para o gravado (o mês ajustado adiantado), que só passa para A receber quando o período fecha.
	place := func(d Due) {
		if e.today <= d.PeriodEnd {
			out.InProgress = append(out.InProgress, d)
			out.Totals.InProgressCount++
			out.Totals.InProgressCents += d.AmountCents
			return
		}
		out.ToReceive = append(out.ToReceive, d)
	}
	for _, p := range proposals {
		place(dueOfProposal(e, p))
	}
	month := e.today[:len("2006-01")]
	for _, r := range viewsOf(e, rows, namesOf(customers)) {
		if r.Received() {
			out.Received = append(out.Received, r)
			if strings.HasPrefix(*r.ReceivedOn, month) && r.ReceivedCents != nil {
				out.Totals.ReceivedMonthCount++
				out.Totals.ReceivedMonthCents += *r.ReceivedCents
			}
			continue
		}
		place(dueOfReceivable(r))
	}
	// Em andamento, o que fecha primeiro antes.
	slices.SortStableFunc(out.InProgress, func(a, b Due) int {
		if c := strings.Compare(a.PeriodEnd, b.PeriodEnd); c != 0 {
			return c
		}
		return strings.Compare(a.Customer.Name, b.Customer.Name)
	})
	// A receber, o vencimento mais antigo primeiro: os atrasados (o mais atrasado no topo) e depois o que vence antes.
	slices.SortStableFunc(out.ToReceive, func(a, b Due) int {
		if c := strings.Compare(a.DueDate, b.DueDate); c != 0 {
			return c
		}
		if c := strings.Compare(a.Customer.Name, b.Customer.Name); c != 0 {
			return c
		}
		return strings.Compare(a.PeriodEnd, b.PeriodEnd)
	})
	for _, d := range out.ToReceive {
		out.Totals.ToReceiveCount++
		out.Totals.ToReceiveCents += d.AmountCents
		if d.Late {
			out.Totals.LateCount++
			out.Totals.LateCents += d.AmountCents
		}
	}
	// Os recebidos, o mais recente primeiro; só os últimos MaxReceived vão para a tela.
	slices.SortStableFunc(out.Received, func(a, b Receivable) int {
		if c := strings.Compare(*b.ReceivedOn, *a.ReceivedOn); c != 0 {
			return c
		}
		return b.ConfirmedAt.Compare(a.ConfirmedAt)
	})
	if len(out.Received) > MaxReceived {
		out.Received = out.Received[:MaxReceived]
	}
	return out, nil
}

// Confirm confirma o que se vai cobrar de um cliente por um período: grava o recebível com o tempo e o valor que o
// sistema calcula agora e o valor que o dono confirmou (que pode ser outro). O período tem de ser um do ciclo do
// cliente. No cliente por hora só fecha depois que o período terminou (ErrPeriodOpen); no mensal vale a qualquer
// momento, inclusive adiantado, dentro do contrato. Um ID malformado ou de outra organização vale como "cliente não
// encontrado".
func (s *Service) Confirm(orgID string, in ConfirmInput) (*Receivable, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	b, err := s.billableOf(e, in.CustomerID, in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if in.AmountCents == nil || *in.AmountCents <= 0 || *in.AmountCents > validate.MaxCents {
		return nil, ErrInvalidAmount.With("field", "amount_cents")
	}
	due := in.DueDate
	if due == "" {
		due = b.defaultDue()
	} else if !validDay(due) {
		return nil, apperr.ErrFieldInvalid.With("field", "due_date")
	}
	invoice, err := validate.Text("invoice_number", in.InvoiceNumber, validate.MaxInvoiceNumber)
	if err != nil {
		return nil, err
	}
	note, err := validate.Text("note", in.Note, validate.MaxNote)
	if err != nil {
		return nil, err
	}
	if err := s.notYetRecorded(b); err != nil {
		return nil, err
	}
	seconds, calculated, err := s.calculate(e, b)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Store.Create(e.id, &ent.Receivable{
		CustomerID: b.customer.ID, Kind: b.kind, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd,
		Seconds: seconds, CalculatedCents: calculated, AmountCents: *in.AmountCents, Currency: e.org.Currency,
		DueDate: due, InvoiceNumber: invoice, Note: note, ConfirmedAt: e.now,
	})
	if err != nil {
		return nil, err
	}
	v := view(e, created, b.customer.Name)
	return &v, nil
}

// ReceivePeriod recebe um período calculado num passo só: grava o recebível já recebido, com o valor que o sistema
// calcula agora e o vencimento padrão do cliente. As regras do período são as de Confirm (no mensal, o mês corrente é o
// recebimento adiantado; no por hora, só depois de fechar) e as do valor são as de Receive. Outro valor ou outro
// vencimento se ajustam antes, por Confirm.
func (s *Service) ReceivePeriod(orgID string, in ReceivePeriodInput) (*Receivable, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	b, err := s.billableOf(e, in.CustomerID, in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return nil, err
	}
	on, err := receivedOn(e, in.ReceiveInput)
	if err != nil {
		return nil, err
	}
	invoice := ""
	if in.InvoiceNumber != nil {
		if invoice, err = validate.Text("invoice_number", *in.InvoiceNumber, validate.MaxInvoiceNumber); err != nil {
			return nil, err
		}
	}
	note, err := validate.Text("note", in.Note, validate.MaxNote)
	if err != nil {
		return nil, err
	}
	if err := s.notYetRecorded(b); err != nil {
		return nil, err
	}
	seconds, calculated, err := s.calculate(e, b)
	if err != nil {
		return nil, err
	}
	// Um período sem valor (as sessões não têm valor cobrado) não se recebe assim: o dono ajusta o valor antes.
	if calculated <= 0 || calculated > validate.MaxCents {
		return nil, ErrInvalidAmount.With("field", "amount_cents")
	}
	cents, fee, interest, err := receiveAmounts(b.kind, calculated, in.ReceiveInput)
	if err != nil {
		return nil, err
	}
	created, err := s.deps.Store.Create(e.id, &ent.Receivable{
		CustomerID: b.customer.ID, Kind: b.kind, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd,
		Seconds: seconds, CalculatedCents: calculated, AmountCents: calculated, Currency: e.org.Currency,
		DueDate: b.defaultDue(), InvoiceNumber: invoice, Note: note, ConfirmedAt: e.now,
		ReceivedOn: &on, ReceivedCents: &cents, LateFeeCents: fee, InterestCents: interest,
	})
	if err != nil {
		return nil, err
	}
	v := view(e, created, b.customer.Name)
	return &v, nil
}

// billable é um período de um cliente que se pode cobrar agora, já conferido por billableOf.
type billable struct {
	customer *ent.Customer
	terms    terms
	kind     string
	period   payment.Period
}

// defaultDue é o vencimento padrão do período: o fim dele mais os payment_days do cliente.
func (b billable) defaultDue() string { return addDays(b.period.PayDay(), b.customer.PaymentDays) }

// billableOf confere o cliente e o período de uma cobrança: o período tem de ser um do ciclo do cliente; no por hora,
// já fechado (ErrPeriodOpen); no mensal, um dos do contrato até o corrente. Um ID malformado ou de outra organização
// vale como "cliente não encontrado".
func (s *Service) billableOf(e env, customerID, start, end string) (billable, error) {
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return billable{}, customer.ErrNotFound
	}
	c, err := s.deps.Store.Customer(e.id, cid)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return billable{}, customer.ErrNotFound
		}
		return billable{}, err
	}
	for field, v := range map[string]string{"period_start": start, "period_end": end} {
		if v == "" {
			return billable{}, apperr.ErrFieldRequired.With("field", field)
		}
		if !validDay(v) {
			return billable{}, apperr.ErrFieldInvalid.With("field", field)
		}
	}
	t := termsOf(c, e.loc)
	period, ok := t.cycle.exact(start, end)
	if !ok {
		return billable{}, ErrNotAPeriod.With("field", "period_start")
	}
	switch c.BillingMode {
	case KindHourly:
		if !closed(period, e.now) {
			return billable{}, ErrPeriodOpen
		}
	case KindMonthly:
		if !slices.ContainsFunc(t.contractPeriods(e.now), func(p payment.Period) bool { return p.StartDay() == start }) {
			return billable{}, ErrNotAPeriod.With("field", "period_start")
		}
	default:
		return billable{}, ErrNotAPeriod.With("field", "period_start")
	}
	return billable{customer: c, terms: t, kind: c.BillingMode, period: period}, nil
}

// notYetRecorded recusa o período que já tem recebível (ErrAlreadyConfirmed); o índice único garante o mesmo.
func (s *Service) notYetRecorded(b billable) error {
	exists, err := s.deps.Store.Exists(b.customer.ID, b.period.StartDay(), b.period.PayDay())
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyConfirmed
	}
	return nil
}

// calculate é o tempo e o valor que o sistema calcula agora para o período: as sessões dos projetos do cliente
// cortadas nele; no mensal, o valor é a mensalidade mais os acréscimos mensais dos projetos.
func (s *Service) calculate(e env, b billable) (float64, int, error) {
	sessions, err := s.deps.Store.SessionsByCustomer(e.id, []uuid.UUID{b.customer.ID})
	if err != nil {
		return 0, 0, err
	}
	seconds, cents := sumWithin(sessions[b.customer.ID], b.period.Start, b.period.End, e.now)
	if b.kind == KindMonthly {
		cents = b.terms.monthlyCents()
	}
	return seconds, cents, nil
}

// one lê um recebível da organização e o nome do cliente dele.
func (s *Service) one(e env, id string) (*ent.Receivable, string, error) {
	r, err := s.deps.Store.Get(e.id, id)
	if err != nil {
		return nil, "", err
	}
	c, err := s.deps.Store.Customer(e.id, r.CustomerID)
	if err != nil {
		return nil, "", err
	}
	return r, c.Name, nil
}

// Update edita o que o dono ajustou na confirmação: o valor, o vencimento, o número da nota e a observação. Vale
// enquanto o recebível não foi recebido (ErrAlreadyReceived).
func (s *Service) Update(orgID, id string, in UpdateInput) (*Receivable, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	r, name, err := s.one(e, id)
	if err != nil {
		return nil, err
	}
	if r.ReceivedOn != nil {
		return nil, ErrAlreadyReceived
	}
	var fields editFields
	if in.AmountCents != nil {
		if *in.AmountCents <= 0 || *in.AmountCents > validate.MaxCents {
			return nil, ErrInvalidAmount.With("field", "amount_cents")
		}
		fields.amount = in.AmountCents
	}
	if in.DueDate != nil {
		due := strings.TrimSpace(*in.DueDate)
		if due == "" {
			return nil, apperr.ErrFieldRequired.With("field", "due_date")
		}
		if !validDay(due) {
			return nil, apperr.ErrFieldInvalid.With("field", "due_date")
		}
		fields.dueDate = &due
	}
	if in.InvoiceNumber != nil {
		v, err := validate.Text("invoice_number", *in.InvoiceNumber, validate.MaxInvoiceNumber)
		if err != nil {
			return nil, err
		}
		fields.invoice = &v
	}
	if in.Note != nil {
		v, err := validate.Text("note", *in.Note, validate.MaxNote)
		if err != nil {
			return nil, err
		}
		fields.note = &v
	}
	if fields != (editFields{}) {
		if err := s.deps.Store.Update(e.id, r.ID, fields); err != nil {
			return nil, err
		}
		if r, err = s.deps.Store.Get(e.id, id); err != nil {
			return nil, err
		}
	}
	v := view(e, r, name)
	return &v, nil
}

// Receive registra que o dinheiro entrou. Sem received_on vale hoje, e uma data depois de hoje é recusada.
//
// No cliente por hora o valor recebido é o confirmado, e multa e juros são recusados. No mensal o valor recebido é
// livre (maior que zero, e sem ele vale o confirmado mais a multa e os juros), e a multa e os juros são livres (zero ou
// mais, partes do valor recebido): a sugestão está em LateCharges.
func (s *Service) Receive(orgID, id string, in ReceiveInput) (*Receivable, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	r, name, err := s.one(e, id)
	if err != nil {
		return nil, err
	}
	if r.ReceivedOn != nil {
		return nil, ErrAlreadyReceived
	}
	on, err := receivedOn(e, in)
	if err != nil {
		return nil, err
	}
	cents, fee, interest, err := receiveAmounts(r.Kind, r.AmountCents, in)
	if err != nil {
		return nil, err
	}
	var invoice *string
	if in.InvoiceNumber != nil {
		v, err := validate.Text("invoice_number", *in.InvoiceNumber, validate.MaxInvoiceNumber)
		if err != nil {
			return nil, err
		}
		invoice = &v
	}
	if err := s.deps.Store.Receive(e.id, r.ID, r.AmountCents, on, cents, fee, interest, invoice); err != nil {
		return nil, err
	}
	r, err = s.deps.Store.Get(e.id, id)
	if err != nil {
		return nil, err
	}
	v := view(e, r, name)
	return &v, nil
}

// receivedOn é o dia do recebimento: o informado, ou hoje sem ele. Uma data depois de hoje é recusada.
func receivedOn(e env, in ReceiveInput) (string, error) {
	if in.ReceivedOn == "" {
		return e.today, nil
	}
	if !validDay(in.ReceivedOn) {
		return "", apperr.ErrFieldInvalid.With("field", "received_on")
	}
	if in.ReceivedOn > e.today {
		return "", ErrReceivedInFuture.With("field", "received_on")
	}
	return in.ReceivedOn, nil
}

// receiveAmounts confere o que entrou de uma conta de valor amount e devolve o valor recebido, a multa e os juros. No
// por hora o valor recebido é o da conta e multa e juros são recusados; no mensal o valor é livre (maior que zero, e
// sem ele vale a conta mais a multa e os juros), e a multa e os juros valem zero ou mais, partes do valor recebido.
func receiveAmounts(kind string, amount int, in ReceiveInput) (cents, fee, interest int, err error) {
	if kind == KindHourly {
		if in.ReceivedCents != nil && *in.ReceivedCents != amount {
			return 0, 0, 0, ErrReceivedMustMatch.With("field", "received_cents")
		}
		if in.LateFeeCents != nil && *in.LateFeeCents != 0 {
			return 0, 0, 0, ErrChargesNotAllowed.With("field", "late_fee_cents")
		}
		if in.InterestCents != nil && *in.InterestCents != 0 {
			return 0, 0, 0, ErrChargesNotAllowed.With("field", "interest_cents")
		}
		return amount, 0, 0, nil
	}
	if in.ReceivedCents != nil && (*in.ReceivedCents <= 0 || *in.ReceivedCents > validate.MaxCents) {
		return 0, 0, 0, ErrInvalidAmount.With("field", "received_cents")
	}
	for _, c := range []struct {
		field string
		in    *int
		out   *int
	}{{"late_fee_cents", in.LateFeeCents, &fee}, {"interest_cents", in.InterestCents, &interest}} {
		if c.in == nil {
			continue
		}
		if *c.in < 0 || *c.in > validate.MaxCents {
			return 0, 0, 0, ErrInvalidCharge.With("field", c.field)
		}
		*c.out = *c.in
	}
	if in.ReceivedCents != nil {
		cents = *in.ReceivedCents
	} else if cents = amount + fee + interest; cents > validate.MaxCents {
		return 0, 0, 0, ErrInvalidAmount.With("field", "received_cents")
	}
	if fee+interest > cents {
		return 0, 0, 0, ErrChargesExceedReceived.With("field", "late_fee_cents")
	}
	return cents, fee, interest, nil
}

// Unreceive desfaz o recebimento: o recebível volta a estar em aberto, e a multa e os juros informados se vão com ele.
func (s *Service) Unreceive(orgID, id string) error {
	e, err := s.env(orgID)
	if err != nil {
		return err
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return database.ErrNotFound
	}
	return s.deps.Store.Unreceive(e.id, rid)
}

// Unconfirm desfaz a confirmação: o recebível some e o período volta a ser uma proposta. Com o recebimento registrado
// é recusado (ErrAlreadyReceived): desfaça o recebimento antes.
func (s *Service) Unconfirm(orgID, id string) error {
	e, err := s.env(orgID)
	if err != nil {
		return err
	}
	rid, err := uuid.Parse(id)
	if err != nil {
		return database.ErrNotFound
	}
	return s.deps.Store.Delete(e.id, rid)
}

// LateCharges sugere a multa e os juros de receber o recebível mensal em on (YYYY-MM-DD, hoje se vazio). É só uma
// sugestão: o recebimento grava o que o dono informar. No recebível por hora não há multa nem juros: tudo zero.
func (s *Service) LateCharges(orgID, id, on string) (*LateCharges, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	if on == "" {
		on = e.today
	} else if !validDay(on) {
		return nil, apperr.ErrFieldInvalid.With("field", "on")
	}
	r, _, err := s.one(e, id)
	if err != nil {
		return nil, err
	}
	if r.Kind != KindMonthly {
		out := lateCharges(r.AmountCents, 0, 0, 0, r.DueDate, on)
		out.Charged, out.TotalCents = false, r.AmountCents
		return &out, nil
	}
	c, err := s.deps.Store.Customer(e.id, r.CustomerID)
	if err != nil {
		return nil, err
	}
	out := lateCharges(r.AmountCents, c.LateFeeBp, c.LateInterestBpMonth, c.GraceDays, r.DueDate, on)
	return &out, nil
}

// PeriodLateCharges é a mesma sugestão para um período calculado, ainda sem recebível: o valor é o que o sistema
// calcula agora e o vencimento é o padrão do cliente. O período segue as regras de Confirm.
func (s *Service) PeriodLateCharges(orgID, customerID, start, end, on string) (*LateCharges, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	if on == "" {
		on = e.today
	} else if !validDay(on) {
		return nil, apperr.ErrFieldInvalid.With("field", "on")
	}
	b, err := s.billableOf(e, customerID, start, end)
	if err != nil {
		return nil, err
	}
	_, amount, err := s.calculate(e, b)
	if err != nil {
		return nil, err
	}
	if b.kind != KindMonthly {
		out := lateCharges(amount, 0, 0, 0, b.defaultDue(), on)
		out.Charged, out.TotalCents = false, amount
		return &out, nil
	}
	c := b.customer
	out := lateCharges(amount, c.LateFeeBp, c.LateInterestBpMonth, c.GraceDays, b.defaultDue(), on)
	return &out, nil
}
