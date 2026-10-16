package payment

import (
	"context"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/ent/payout"
	entperson "working-time-tracker/ent/person"
	"working-time-tracker/internal/database"
)

// Payout é a baixa de um período: o dono marcou que pagou. Start e PayDate são as datas do período pago; Seconds e
// CalculatedCents, o que a baixa cobriu (na primeira, o período até ali; nas seguintes, o que a pessoa registrou depois
// da anterior); AmountCents, o que foi pago de fato por essas horas; BonusCents, um bônus pago junto (zero quando não
// houve). Seq é a ordem da baixa no período, 0 para a primeira.
type Payout struct {
	ID              uuid.UUID `json:"id"`
	PersonID        uuid.UUID `json:"person_id"`
	Start           string    `json:"start"`
	PayDate         string    `json:"pay_date"`
	Seconds         float64   `json:"seconds"`
	CalculatedCents int       `json:"calculated_cents"`
	AmountCents     int       `json:"amount_cents"`
	BonusCents      int       `json:"bonus_cents"`
	Seq             int       `json:"seq"`
	PaidAt          time.Time `json:"paid_at"`
}

// key identifica o período pago de uma pessoa.
func (p Payout) key() string { return p.PersonID.String() + "|" + p.Start + "|" + p.PayDate }

func periodKey(personID uuid.UUID, v PeriodView) string {
	return personID.String() + "|" + v.Start + "|" + v.PayDate
}

type Store struct {
	client *ent.Client
}

func NewStore(client *ent.Client) *Store {
	return &Store{client: client}
}

func toPayout(e *ent.Payout) Payout {
	return Payout{
		ID: e.ID, PersonID: e.PersonID, Start: e.PeriodStart, PayDate: e.PayDate, Seconds: e.Seconds,
		CalculatedCents: e.CalculatedCents, AmountCents: e.AmountCents, BonusCents: e.BonusCents, Seq: e.Seq, PaidAt: e.PaidAt,
	}
}

// Create grava a baixa. Outra baixa com a mesma seq no período, gravada ao mesmo tempo, esbarra no índice único:
// ErrAlreadyPaid.
func (s *Store) Create(p *Payout) error {
	created, err := s.client.Payout.Create().
		SetPersonID(p.PersonID).
		SetPeriodStart(p.Start).
		SetPayDate(p.PayDate).
		SetSeconds(p.Seconds).
		SetCalculatedCents(p.CalculatedCents).
		SetAmountCents(p.AmountCents).
		SetBonusCents(p.BonusCents).
		SetSeq(p.Seq).
		SetPaidAt(p.PaidAt).
		Save(context.Background())
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrAlreadyPaid
		}
		return err
	}
	*p = toPayout(created)
	return nil
}

// Delete apaga a baixa, se ela é de uma pessoa da organização.
func (s *Store) Delete(orgID, id string) error {
	oid, err := uuid.Parse(orgID)
	if err != nil {
		return database.ErrNotFound
	}
	pid, err := uuid.Parse(id)
	if err != nil {
		return database.ErrNotFound
	}
	n, err := s.client.Payout.Delete().
		Where(payout.ID(pid), payout.HasPersonWith(entperson.OrganizationID(oid))).
		Exec(context.Background())
	if err != nil {
		return err
	}
	if n == 0 {
		return database.ErrNotFound
	}
	return nil
}

// ListByPersons devolve as baixas das pessoas, todas: são poucas por pessoa, uma por período.
func (s *Store) ListByPersons(ids []uuid.UUID) ([]Payout, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.client.Payout.Query().Where(payout.PersonIDIn(ids...)).All(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Payout, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPayout(r))
	}
	return out, nil
}
