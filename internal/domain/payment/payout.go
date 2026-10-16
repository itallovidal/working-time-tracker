package payment

import (
	"time"

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

// MarkPaid dá baixa num período: grava que o dono pagou, com as horas e o valor que o período tem agora e o valor pago.
// Dá baixa a qualquer momento depois que o período começou, inclusive no corrente, antes da data: o que a pessoa
// registrar depois aparece na baixa como "depois da baixa". Uma pessoa de outra organização vale como "não encontrada".
func (s *Service) MarkPaid(orgID string, in MarkInput) (*Payout, error) {
	if s.deps.Payouts == nil {
		return nil, database.ErrNotFound
	}
	org, err := s.deps.Orgs.Get(orgID)
	if err != nil {
		return nil, err
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
	po := &Payout{
		PersonID: p.ID, Start: in.Start, PayDate: in.PayDate,
		Seconds: t.seconds, CalculatedCents: t.cents, AmountCents: *in.AmountCents, BonusCents: bonus, PaidAt: now,
	}
	if err := s.deps.Payouts.Create(po); err != nil {
		return nil, err
	}
	return po, nil
}

// Unmark desfaz a baixa: o período volta a ser um pagamento a fazer.
func (s *Service) Unmark(orgID, payoutID string) error {
	if s.deps.Payouts == nil {
		return database.ErrNotFound
	}
	return s.deps.Payouts.Delete(orgID, payoutID)
}
