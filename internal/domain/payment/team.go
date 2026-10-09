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

// TeamPerson é a linha de uma pessoa com regra: o período corrente (horas e valor até agora) e o último fechado.
// O dono tem horas e nenhum valor.
type TeamPerson struct {
	Person PersonRef          `json:"person"`
	Rule   person.PaymentRule `json:"rule"`
	// PayDate é a próxima data de pagamento: a do período corrente, ou a do primeiro, se a contagem não começou.
	PayDate    string      `json:"pay_date"`
	Current    *PeriodView `json:"current"`
	LastClosed *PeriodView `json:"last_closed"`
}

// CalendarDay é uma data de pagamento próxima, com quem recebe nela. TotalCents soma só os períodos que já estão
// correndo (os futuros ainda não têm horas) e não inclui o dono.
type CalendarDay struct {
	Date       string      `json:"date"`
	People     []PersonRef `json:"people"`
	TotalCents int         `json:"total_cents"`
}

// TeamTotals são os números dos cartões. Os valores não incluem o dono, que não é pago.
type TeamTotals struct {
	// ToPayCents é o que se deve pagar nos períodos correntes, até agora.
	ToPayCents int `json:"to_pay_cents"`
	// Closing7Cents e Closing30Cents são a parte dele que fecha nos próximos 7 e 30 dias.
	Closing7Cents  int `json:"closing_7_cents"`
	Closing30Cents int `json:"closing_30_cents"`
	// Seconds são as horas da equipe nos períodos correntes, dono incluído; OwnerSeconds é a parte dele.
	Seconds      float64 `json:"seconds"`
	OwnerSeconds float64 `json:"owner_seconds"`
	WithoutRule  int     `json:"without_rule"`
}

// Team é a resposta de GET /orgs/:orgId/payments.
type Team struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Timezone    string        `json:"timezone"`
	People      []TeamPerson  `json:"people"`
	WithoutRule []PersonRef   `json:"without_rule"`
	Calendar    []CalendarDay `json:"calendar"`
	Totals      TeamTotals    `json:"totals"`
}

const calendarDays = 60

// Team monta a visão do dono e dos admins: quanto pagar, as horas de cada pessoa e as datas. As linhas seguem a
// data do pagamento e, no empate, o nome: sem ordem por horas, de propósito (comparar o tempo das pessoas sugere
// que quem trabalhou mais merece mais reconhecimento).
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
	out := &Team{GeneratedAt: now, Timezone: org.Timezone, People: []TeamPerson{}, WithoutRule: []PersonRef{}, Calendar: []CalendarDay{}}

	since := now
	for i := range people {
		if people[i].Payment != nil {
			since = earliest(since, sinceFor(*people[i].Payment, now, 1, loc))
		}
	}
	byPerson := map[uuid.UUID][]work_session.WorkSession{}
	if len(people) > 0 {
		sessions, err := s.deps.Sessions.ListByOrganizationSince(orgID, since)
		if err != nil {
			return nil, err
		}
		for _, w := range sessions {
			byPerson[w.PersonID] = append(byPerson[w.PersonID], w)
		}
	}

	today := midnight(now, loc)
	days := map[string]*CalendarDay{}
	for i := range people {
		p := &people[i]
		ref := PersonRef{ID: p.ID, Name: p.Name, IsOwner: p.IsOwner}
		if p.Payment == nil {
			out.WithoutRule = append(out.WithoutRule, ref)
			continue
		}
		sum := newSummary(p, org.Timezone, now)
		fill(sum, p, byPerson[p.ID], now, 1, loc)
		row := TeamPerson{Person: ref, Rule: *p.Payment, Current: sum.Current}
		if sum.Current != nil {
			sum.Current.Projects = nil
			row.PayDate = sum.Current.PayDate
		} else if len(sum.Upcoming) > 0 {
			row.PayDate = sum.Upcoming[0].PayDate
		}
		if len(sum.History) > 0 {
			row.LastClosed = &sum.History[0]
		}
		out.People = append(out.People, row)

		if cur := sum.Current; cur != nil {
			out.Totals.Seconds += cur.Seconds
			if p.IsOwner {
				out.Totals.OwnerSeconds += cur.Seconds
			} else if cur.AmountCents != nil {
				out.Totals.ToPayCents += *cur.AmountCents
				if cur.DaysLeft != nil && *cur.DaysLeft <= 7 {
					out.Totals.Closing7Cents += *cur.AmountCents
				}
				if cur.DaysLeft != nil && *cur.DaysLeft <= 30 {
					out.Totals.Closing30Cents += *cur.AmountCents
				}
			}
		}
		for j, up := range sum.Upcoming {
			d, err := time.ParseInLocation(time.DateOnly, up.PayDate, loc)
			if err != nil || daysBetween(today, d) > calendarDays {
				continue
			}
			day := days[up.PayDate]
			if day == nil {
				day = &CalendarDay{Date: up.PayDate, People: []PersonRef{}}
				days[up.PayDate] = day
			}
			day.People = append(day.People, ref)
			// Só o primeiro dos próximos é o período corrente, que já tem horas.
			if j == 0 && sum.Current != nil && !p.IsOwner && sum.Current.AmountCents != nil {
				day.TotalCents += *sum.Current.AmountCents
			}
		}
	}
	out.Totals.WithoutRule = len(out.WithoutRule)

	slices.SortFunc(out.People, func(a, b TeamPerson) int {
		return cmp.Or(strings.Compare(a.PayDate, b.PayDate), strings.Compare(strings.ToLower(a.Person.Name), strings.ToLower(b.Person.Name)), strings.Compare(a.Person.ID.String(), b.Person.ID.String()))
	})
	slices.SortFunc(out.WithoutRule, func(a, b PersonRef) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.ID.String(), b.ID.String()))
	})
	for _, d := range days {
		slices.SortFunc(d.People, func(a, b PersonRef) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
		out.Calendar = append(out.Calendar, *d)
	}
	slices.SortFunc(out.Calendar, func(a, b CalendarDay) int { return strings.Compare(a.Date, b.Date) })
	return out, nil
}

func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
