package payment

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/database"
)

// MarkInput é o pedido de baixa: o período de uma pessoa, o valor pago pelas horas e um bônus. O valor vem da tela,
// preenchido com o calculado, e o dono pode mudá-lo (um desconto, um arredondamento); o bônus é opcional, e sem ele
// vale zero.
type MarkInput struct {
	PersonID    string `json:"person_id"`
	Start       string `json:"start"`
	PayDate     string `json:"pay_date"`
	AmountCents *int   `json:"amount_cents"`
	BonusCents  *int   `json:"bonus_cents"`
}

// MarkPaid dá baixa num período: grava que o dono pagou, com as horas e o valor que a baixa cobre e o valor pago. Dá
// baixa a qualquer momento depois que o período começou, inclusive no corrente, antes da data. O que a pessoa registrar
// depois de uma baixa é o restante do período: volta para os pagamentos a fazer e leva outra baixa, que cobre só esse
// restante; sem restante, o período já está pago (ErrAlreadyPaid). Um ID malformado ou uma pessoa de outra organização
// vale como "não encontrada".
func (s *Service) MarkPaid(orgID string, in MarkInput) (*Payout, error) {
	if s.deps.Payouts == nil {
		return nil, database.ErrNotFound
	}
	org, err := s.deps.Orgs.Get(orgID)
	if err != nil {
		return nil, err
	}
	// O ID vem do corpo, não da rota: nenhum middleware o conferiu.
	if _, err := uuid.Parse(in.PersonID); err != nil {
		return nil, database.ErrNotFound
	}
	p, err := s.deps.People.Get(in.PersonID)
	if err != nil {
		return nil, err
	}
	if p.OrganizationID.String() != orgID {
		return nil, database.ErrNotFound
	}
	if p.IsOwner {
		return nil, ErrOwnerNotPaid
	}
	if p.Payment == nil {
		return nil, ErrNoRule
	}
	if in.AmountCents == nil || *in.AmountCents < 0 {
		return nil, ErrInvalidAmount.With("field", "amount_cents")
	}
	bonus := 0
	if in.BonusCents != nil {
		if *in.BonusCents < 0 {
			return nil, ErrInvalidBonus.With("field", "bonus_cents")
		}
		bonus = *in.BonusCents
	}
	loc := location(org.Timezone)
	now := s.now()
	start, err := time.ParseInLocation(time.DateOnly, in.Start, loc)
	if err != nil {
		return nil, ErrNotAPeriod.With("field", "start")
	}
	period, ok := PeriodAt(*p.Payment, start, loc)
	if !ok || period.StartDay() != in.Start || period.PayDay() != in.PayDate {
		return nil, ErrNotAPeriod.With("field", "start")
	}
	if period.Start.After(now) {
		return nil, ErrPeriodNotStarted
	}
	sessions, err := s.deps.Sessions.ListByPersonSince(orgID, in.PersonID, period.Start)
	if err != nil {
		return nil, err
	}
	t := within(sessions, period.Start, period.End, now, false, nil)
	all, err := s.payoutsOf([]uuid.UUID{p.ID})
	if err != nil {
		return nil, err
	}
	before := groupPayouts(all)[periodKey(p.ID, view(period))]
	rest := remaining(t, before)
	if len(before) > 0 && rest.cents <= 0 {
		return nil, ErrAlreadyPaid
	}
	po := &Payout{
		PersonID: p.ID, Start: in.Start, PayDate: in.PayDate, Seq: nextSeq(before),
		Seconds: rest.seconds, CalculatedCents: rest.cents, AmountCents: *in.AmountCents, BonusCents: bonus, PaidAt: now,
	}
	if err := s.deps.Payouts.Create(po); err != nil {
		return nil, err
	}
	return po, nil
}

// groupPayouts junta as baixas por período de cada pessoa (periodKey), na ordem em que foram dadas.
func groupPayouts(payouts []Payout) map[string][]Payout {
	out := map[string][]Payout{}
	for _, po := range payouts {
		out[po.key()] = append(out[po.key()], po)
	}
	for k := range out {
		slices.SortFunc(out[k], func(a, b Payout) int { return a.Seq - b.Seq })
	}
	return out
}

// remaining é o que o período tem agora (t) e as baixas dele não cobriram: as horas e o valor registrados depois da
// última. Sem baixa, é o período inteiro. Uma sessão apagada depois da baixa pode deixar o período com menos do que foi
// coberto; o restante não fica negativo.
func remaining(t sum, payouts []Payout) sum {
	for _, po := range payouts {
		t.seconds -= po.Seconds
		t.cents -= po.CalculatedCents
	}
	t.seconds, t.cents = max(0, t.seconds), max(0, t.cents)
	return t
}

// nextSeq é a posição da próxima baixa do período: uma depois da maior, para não repetir a de uma baixa desfeita no
// meio.
func nextSeq(payouts []Payout) int {
	next := 0
	for _, po := range payouts {
		next = max(next, po.Seq+1)
	}
	return next
}

// Unmark desfaz a baixa: o que ela cobria volta a ser um pagamento a fazer.
func (s *Service) Unmark(orgID, payoutID string) error {
	if s.deps.Payouts == nil {
		return database.ErrNotFound
	}
	return s.deps.Payouts.Delete(orgID, payoutID)
}
