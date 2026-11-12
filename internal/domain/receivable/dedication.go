package receivable

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"working-time-tracker/ent"
	"working-time-tracker/internal/database"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/domain/work_session"
)

// Dedication é o acompanhamento de um cliente com contrato mensal e estimativa de horas (Decisões 26 e 29): as horas
// trabalhadas num período do contrato, em todos os projetos dele, contra a meta do período, que sai das horas que o dono
// estima dedicar por semana (customers.estimated_weekly_hours) vezes os dias do período sobre 7, a mesma conta da
// jornada semanal da pessoa. É só leitura: não muda a mensalidade nem cobra excedente. O período é o mesmo que a
// mensalidade paga, o do dia de fechamento do cliente, e os dias são YYYY-MM-DD no fuso da organização, os dois
// inclusivos.
type Dedication struct {
	Customer    Ref    `json:"customer"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Open diz que o período ainda não fechou: as horas são até agora, com a sessão aberta.
	Open bool `json:"open"`
	// EstimatedWeeklyHours é a estimativa do cliente, por semana; EstimatedSeconds, a meta do período que sai dela.
	EstimatedWeeklyHours int     `json:"estimated_weekly_hours"`
	EstimatedSeconds     float64 `json:"estimated_seconds"`
	// Seconds é o tempo do período em todos os projetos do cliente, cada sessão cortada nas bordas dele. O que a mesma
	// pessoa trabalhou ao mesmo tempo em dois projetos do cliente conta uma vez só (Decisão 27), então Seconds pode ser
	// menor que a soma dos Projects.
	Seconds float64 `json:"seconds"`
	// Percent é Seconds sobre a meta do período, arredondado; passa de 100 quando as horas passam da meta.
	Percent int `json:"percent"`
	// Projects são os projetos do cliente com o tempo de cada um no período, em ordem de nome (nunca por horas: o
	// sistema não ranqueia), cada um com o tempo inteiro das sessões dele. Os sem tempo também entram, com zero.
	Projects []DedicationProject `json:"projects"`
	// Week é a semana em andamento contra a estimativa semanal. Só vem com o período em andamento: nula no período
	// anterior e no A receber.
	Week *DedicationWeek `json:"week"`
}

// DedicationProject é um projeto do cliente na dedicação, com o tempo dele no período.
type DedicationProject struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Seconds float64   `json:"seconds"`
}

// DedicationWeek é a semana em andamento, de segunda a domingo no fuso da organização (YYYY-MM-DD, os dois inclusivos):
// o tempo em todos os projetos do cliente, até agora e com o sobreposto contado uma vez, contra as horas estimadas por
// semana. A semana é sempre inteira, como o período, que também não tem pro-rata.
type DedicationWeek struct {
	Start   string  `json:"start"`
	End     string  `json:"end"`
	Seconds float64 `json:"seconds"`
	// Percent é Seconds sobre a estimativa semanal, arredondado; passa de 100 quando as horas passam dela.
	Percent int `json:"percent"`
}

// tracked diz se o cliente tem o que acompanhar: o contrato mensal com estimativa de horas.
func tracked(c *ent.Customer) bool {
	return c.BillingMode == KindMonthly && c.EstimatedWeeklyHours != nil && *c.EstimatedWeeklyHours > 0
}

// percentOf é seconds sobre goal, em porcentagem arredondada.
func percentOf(seconds, goal float64) int {
	return int(math.Round(seconds * 100 / goal))
}

// dedication é a dedicação do cliente no período p, com as sessões dele (as de todos os projetos). Nula quando o
// cliente não tem o que acompanhar ou quando p não é um período do contrato (contractPeriods): antes do início, depois
// do fim, ou o período de um dia do contrato que começa no dia de fechamento. Não traz a semana (veja dedicationAt).
func (t terms) dedication(sessions []work_session.WorkSession, p payment.Period, now time.Time) *Dedication {
	c := t.customer
	if !tracked(c) || !slices.ContainsFunc(t.contractPeriods(now), func(q payment.Period) bool { return q.Start.Equal(p.Start) }) {
		return nil
	}
	byProject := map[uuid.UUID]float64{}
	for i := range sessions {
		sec, _, _ := sessions[i].WithinRange(p.Start, p.End, now)
		if sec <= 0 {
			continue
		}
		byProject[sessions[i].ProjectID] += sec
	}
	// Cada projeto fica com o tempo inteiro dele; o total do cliente conta uma vez o tempo sobreposto (Decisão 27).
	total := work_session.MergedSeconds(sessions, p.Start, p.End, now)
	projects := make([]DedicationProject, 0, len(c.Edges.Projects))
	for _, pr := range c.Edges.Projects {
		projects = append(projects, DedicationProject{ID: pr.ID, Name: pr.Name, Seconds: byProject[pr.ID]})
	}
	byName := collate.New(language.Und)
	slices.SortFunc(projects, func(a, b DedicationProject) int {
		return cmp.Or(byName.CompareString(a.Name, b.Name), cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID.String(), b.ID.String()))
	})
	weekly := *c.EstimatedWeeklyHours
	// A meta do período é a semanal vezes os dias dele sobre 7 (Decisão 29): 20h por semana num período de 30 dias
	// são 85h43.
	goal := float64(weekly) * 3600 * float64(p.Days()) / 7
	return &Dedication{
		Customer: t.ref(), PeriodStart: p.StartDay(), PeriodEnd: p.PayDay(), Open: !closed(p, now),
		EstimatedWeeklyHours: weekly, EstimatedSeconds: goal, Seconds: total,
		Percent:  percentOf(total, goal),
		Projects: projects,
	}
}

// week é a semana em andamento do cliente, de segunda a domingo no fuso da organização, contra a estimativa semanal.
func (t terms) week(sessions []work_session.WorkSession, now time.Time) *DedicationWeek {
	start := payment.WeekStart(now, t.loc)
	end := start.AddDate(0, 0, 7)
	total := work_session.MergedSeconds(sessions, start, end, now)
	return &DedicationWeek{
		Start: start.Format(time.DateOnly), End: end.AddDate(0, 0, -1).Format(time.DateOnly),
		Seconds: total, Percent: percentOf(total, float64(*t.customer.EstimatedWeeklyHours)*3600),
	}
}

// dedicationAt é a dedicação do cliente no período do contrato que contém now, com a semana em andamento, ou no
// período anterior a ele com previous, sem a semana.
func (t terms) dedicationAt(sessions []work_session.WorkSession, now time.Time, previous bool) *Dedication {
	p, ok := t.cycle.at(now)
	if ok && previous {
		p, ok = payment.Previous(p)
	}
	if !ok {
		return nil
	}
	d := t.dedication(sessions, p, now)
	if d != nil && !previous {
		d.Week = t.week(sessions, now)
	}
	return d
}

// Dedications devolve a dedicação de cada cliente com contrato mensal e estimativa de horas, no período do contrato em
// andamento (ou no anterior a ele, com previous), em ordem de nome. Fica de fora o cliente cujo contrato não cobre esse
// período. Nada é gravado.
func (s *Service) Dedications(orgID string, previous bool) ([]Dedication, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	customers, err := s.deps.Store.Customers(e.id)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, c := range customers {
		if tracked(c) {
			ids = append(ids, c.ID)
		}
	}
	out := []Dedication{}
	if len(ids) == 0 {
		return out, nil
	}
	sessions, err := s.deps.Store.SessionsByCustomer(e.id, ids)
	if err != nil {
		return nil, err
	}
	for _, c := range customers {
		if !tracked(c) {
			continue
		}
		if d := termsOf(c, e.loc).dedicationAt(sessions[c.ID], e.now, previous); d != nil {
			out = append(out, *d)
		}
	}
	return out, nil
}

// Dedication devolve a dedicação do cliente no período do contrato em andamento, ou nil quando ele não tem o que
// acompanhar (não é mensal, não tem estimativa, ou o contrato não cobre o período). Um cliente de outra organização ou
// um ID malformado é database.ErrNotFound.
func (s *Service) Dedication(orgID, customerID string) (*Dedication, error) {
	e, err := s.env(orgID)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(customerID)
	if err != nil {
		return nil, database.ErrNotFound
	}
	c, err := s.deps.Store.Customer(e.id, id)
	if err != nil {
		return nil, err
	}
	if !tracked(c) {
		return nil, nil
	}
	sessions, err := s.deps.Store.SessionsByCustomer(e.id, []uuid.UUID{c.ID})
	if err != nil {
		return nil, err
	}
	return termsOf(c, e.loc).dedicationAt(sessions[c.ID], e.now, false), nil
}
