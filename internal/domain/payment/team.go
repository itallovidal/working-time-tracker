package payment

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/work_session"
)

// PersonRef identifica a pessoa numa linha da visão da equipe.
type PersonRef struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	IsOwner bool      `json:"is_owner"`
}

// DueRow é um pagamento a fazer: um período de uma pessoa sem baixa, ou o restante de um que já teve. É o corrente
// (Running, com as horas e o valor até agora) ou um fechado nos últimos overdueDays dias (Overdue, quando a data já
// passou).
type DueRow struct {
	Person  PersonRef          `json:"person"`
	Rule    person.PaymentRule `json:"rule"`
	Start   string             `json:"start"`
	PayDate string             `json:"pay_date"`
	Days    int                `json:"days"`
	// DaysLeft são os dias até o pagamento: 0 é hoje, negativo é atraso.
	DaysLeft    int     `json:"days_left"`
	Overdue     bool    `json:"overdue"`
	Running     bool    `json:"running"`
	Seconds     float64 `json:"seconds"`
	AmountCents int     `json:"amount_cents"`
	// Rest diz que o período já teve baixa e a linha é o restante: Seconds e AmountCents são só o que a pessoa
	// registrou depois da última baixa, e PaidCents é o que as baixas dele já pagaram pelas horas.
	Rest      bool `json:"rest"`
	PaidCents int  `json:"paid_cents"`
	// LastClosed é o último período fechado da pessoa, com a baixa dele, se houve.
	LastClosed *PeriodView `json:"last_closed"`
}

// PaidRow é uma baixa recente. LaterSeconds e LaterCents, só na última baixa do período, são o que ele ganhou depois
// dela e ainda não foi pago: quem foi pago antes da data e seguiu trabalhando. Esse restante também está em Due.
type PaidRow struct {
	Payout
	Person       PersonRef `json:"person"`
	LaterSeconds float64   `json:"later_seconds"`
	LaterCents   int       `json:"later_cents"`
}

// TeamTotals são os números dos cartões. Os valores somam as linhas a pagar, atrasados incluídos; o dono não entra.
type TeamTotals struct {
	// ToPayCents é tudo o que está sem baixa: os atrasados e os correntes, até agora, e o restante dos que já tiveram
	// baixa. OverdueCents é a parte atrasada.
	ToPayCents   int `json:"to_pay_cents"`
	OverdueCents int `json:"overdue_cents"`
	// Closing7Cents e Closing30Cents são a parte que paga em até 7 e 30 dias (os atrasados entram).
	Closing7Cents  int `json:"closing_7_cents"`
	Closing30Cents int `json:"closing_30_cents"`
	// Seconds são as horas da equipe nos períodos correntes, dono incluído; OwnerSeconds é a parte dele.
	Seconds      float64 `json:"seconds"`
	OwnerSeconds float64 `json:"owner_seconds"`
	WithoutRule  int     `json:"without_rule"`
}

// Team é a resposta de GET /orgs/:orgId/payments.
type Team struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Timezone    string      `json:"timezone"`
	Due         []DueRow    `json:"due"`
	Paid        []PaidRow   `json:"paid"`
	WithoutRule []PersonRef `json:"without_rule"`
	Totals      TeamTotals  `json:"totals"`
}

const (
	// overdueDays é até quantos dias para trás um período fechado sem baixa (ou com restante) ainda aparece, como
	// atrasado.
	overdueDays = 60
	// paidDays é até quantos dias para trás uma baixa aparece em Pagos recentemente.
	paidDays = 30
)

// Team monta a visão do dono: o que pagar, a quem e quando, e o que foi pago há pouco. As linhas a pagar seguem a data
// do pagamento e, no empate, o nome: sem ordem por horas, de propósito (comparar o tempo das pessoas sugere que quem
// trabalhou mais merece mais reconhecimento). O dono não é pago: fica fora das linhas e de "sem regra", e só as horas
// dele entram no cartão de horas.
func (s *Service) Team(orgID string) (*Team, error) {
	org, err := s.deps.Orgs.Get(orgID)
	if err != nil {
		return nil, err
	}
	people, err := s.deps.People.ListByOrg(orgID)
	if err != nil {
		return nil, err
	}
	loc := location(org.Timezone)
	now := s.now()
	today := midnight(now, loc)
	out := &Team{GeneratedAt: now, Timezone: org.Timezone, Due: []DueRow{}, Paid: []PaidRow{}, WithoutRule: []PersonRef{}}

	ids := make([]uuid.UUID, 0, len(people))
	for i := range people {
		ids = append(ids, people[i].ID)
	}
	payouts, err := s.payoutsOf(ids)
	if err != nil {
		return nil, err
	}
	paid := groupPayouts(payouts)
	var recent []Payout
	paidFrom := now.AddDate(0, 0, -paidDays)
	for _, po := range payouts {
		if !po.PaidAt.Before(paidFrom) {
			recent = append(recent, po)
		}
	}

	// Os períodos de cada pessoa com regra: o corrente e os fechados que ainda podem estar atrasados (o primeiro
	// fechado é sempre o "último fechado" da linha). As sessões são buscadas desde o começo do mais antigo deles, ou
	// do período de uma baixa recente, que é recalculado para mostrar o que veio depois dela.
	type plan struct {
		p      *person.Person
		cur    *Period
		closed []Period
	}
	var plans []plan
	since := now
	overdueFrom := today.AddDate(0, 0, -overdueDays)
	for i := range people {
		p := &people[i]
		ref := PersonRef{ID: p.ID, Name: p.Name, IsOwner: p.IsOwner}
		if p.Payment == nil {
			if !p.IsOwner {
				out.WithoutRule = append(out.WithoutRule, ref)
			}
			continue
		}
		pl := plan{p: p}
		if cur, ok := PeriodAt(*p.Payment, now, loc); ok {
			pl.cur = &cur
			since = earliest(since, cur.Start)
			for prev, ok := Previous(cur); ok; prev, ok = Previous(prev) {
				if len(pl.closed) > 0 && prev.PayDate.Before(overdueFrom) {
					break
				}
				pl.closed = append(pl.closed, prev)
				since = earliest(since, prev.Start)
			}
		}
		plans = append(plans, pl)
	}
	starts := map[string]time.Time{}
	for _, po := range recent {
		if t, err := time.ParseInLocation(time.DateOnly, po.Start, loc); err == nil {
			starts[po.Start] = t
			since = earliest(since, t)
		}
	}

	byPerson := map[uuid.UUID][]work_session.WorkSession{}
	if len(plans) > 0 || len(recent) > 0 {
		sessions, err := s.deps.Sessions.ListByOrganizationSince(orgID, since)
		if err != nil {
			return nil, err
		}
		for _, w := range sessions {
			byPerson[w.PersonID] = append(byPerson[w.PersonID], w)
		}
	}

	for _, pl := range plans {
		p := pl.p
		ref := PersonRef{ID: p.ID, Name: p.Name, IsOwner: p.IsOwner}
		sessions := byPerson[p.ID]
		if pl.cur != nil {
			t := within(sessions, pl.cur.Start, pl.cur.End, now, p.IsOwner, nil)
			out.Totals.Seconds += t.seconds
			if p.IsOwner {
				out.Totals.OwnerSeconds += t.seconds
			}
		}
		if p.IsOwner || pl.cur == nil {
			continue
		}
		var last *PeriodView
		if len(pl.closed) > 0 {
			t := within(sessions, pl.closed[0].Start, pl.closed[0].End, now, false, nil)
			v := view(pl.closed[0])
			v.Seconds, v.AmountCents = t.seconds, t.amount(false)
			v.Paid = paidMark(paid, p.ID, v)
			last = &v
		}
		// Do fechado mais antigo para o corrente. Um fechado sem horas não tem o que pagar e não vira atraso; um que já
		// teve baixa só volta com o que a pessoa registrou depois dela.
		periods := append(slices.Clone(pl.closed), *pl.cur)
		for i := len(periods) - 1; i >= 0; i-- {
			per := periods[i]
			running := i == len(periods)-1
			if !running && per.PayDate.Before(overdueFrom) {
				continue
			}
			before := paid[periodKey(p.ID, view(per))]
			t := remaining(within(sessions, per.Start, per.End, now, false, nil), before)
			if len(before) > 0 && t.cents <= 0 || len(before) == 0 && !running && t.seconds <= 0 {
				continue
			}
			left := daysBetween(today, per.PayDate)
			row := DueRow{
				Person: ref, Rule: *p.Payment, Start: per.StartDay(), PayDate: per.PayDay(), Days: per.Days(),
				DaysLeft: left, Overdue: left < 0, Running: running, Seconds: t.seconds, AmountCents: t.cents,
				Rest: len(before) > 0, LastClosed: last,
			}
			for _, po := range before {
				row.PaidCents += po.AmountCents
			}
			out.Due = append(out.Due, row)
			out.Totals.ToPayCents += t.cents
			if row.Overdue {
				out.Totals.OverdueCents += t.cents
			}
			if left <= 7 {
				out.Totals.Closing7Cents += t.cents
			}
			if left <= 30 {
				out.Totals.Closing30Cents += t.cents
			}
		}
	}

	names := map[uuid.UUID]PersonRef{}
	for i := range people {
		names[people[i].ID] = PersonRef{ID: people[i].ID, Name: people[i].Name, IsOwner: people[i].IsOwner}
	}
	for _, po := range recent {
		row := PaidRow{Payout: po, Person: names[po.PersonID]}
		period := paid[po.key()]
		start, ok := starts[po.Start]
		if pay, err := time.ParseInLocation(time.DateOnly, po.PayDate, loc); ok && err == nil && period[len(period)-1].ID == po.ID {
			t := remaining(within(byPerson[po.PersonID], start, pay.AddDate(0, 0, 1), now, false, nil), period)
			row.LaterSeconds, row.LaterCents = t.seconds, t.cents
		}
		out.Paid = append(out.Paid, row)
	}
	out.Totals.WithoutRule = len(out.WithoutRule)

	slices.SortFunc(out.Due, func(a, b DueRow) int {
		return cmp.Or(strings.Compare(a.PayDate, b.PayDate), strings.Compare(strings.ToLower(a.Person.Name), strings.ToLower(b.Person.Name)), strings.Compare(a.Person.ID.String(), b.Person.ID.String()))
	})
	slices.SortFunc(out.Paid, func(a, b PaidRow) int {
		return cmp.Or(b.PaidAt.Compare(a.PaidAt), strings.Compare(a.ID.String(), b.ID.String()))
	})
	slices.SortFunc(out.WithoutRule, func(a, b PersonRef) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.ID.String(), b.ID.String()))
	})
	return out, nil
}

// payoutsOf lê as baixas das pessoas. Sem o store (testes antigos), não há baixa nenhuma.
func (s *Service) payoutsOf(ids []uuid.UUID) ([]Payout, error) {
	if s.deps.Payouts == nil {
		return nil, nil
	}
	return s.deps.Payouts.ListByPersons(ids)
}

// paidMark é a baixa do período, se ele tem uma. Com mais de uma (a do período e a do restante), soma o que foi pago e
// fica com a data e o ID da última.
func paidMark(paid map[string][]Payout, personID uuid.UUID, v PeriodView) *PaidMark {
	payouts := paid[periodKey(personID, v)]
	if len(payouts) == 0 {
		return nil
	}
	last := payouts[len(payouts)-1]
	mark := &PaidMark{ID: last.ID, PaidAt: last.PaidAt}
	for _, po := range payouts {
		mark.AmountCents += po.AmountCents
		mark.BonusCents += po.BonusCents
	}
	return mark
}

func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
