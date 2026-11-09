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

// Dedication é o acompanhamento de um cliente com contrato mensal e estimativa de horas (Decisão 26): as horas
// trabalhadas num período do contrato, em todos os projetos dele, contra as horas que o dono estima dedicar por mês
// (customers.estimated_monthly_hours). É só leitura: não muda a mensalidade nem cobra excedente. O período é o mesmo que
// a mensalidade paga, o do dia de fechamento do cliente, e os dias são YYYY-MM-DD no fuso da organização, os dois
// inclusivos.
type Dedication struct {
	Customer    Ref    `json:"customer"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	// Open diz que o período ainda não fechou: as horas são até agora, com a sessão aberta.
	Open           bool `json:"open"`
	EstimatedHours int  `json:"estimated_hours"`
	// Seconds é o tempo do período em todos os projetos do cliente, cada sessão cortada nas bordas dele.
	Seconds float64 `json:"seconds"`
	// Percent é Seconds sobre a estimativa, arredondado; passa de 100 quando as horas passam da estimativa.
	Percent int `json:"percent"`
	// Projects são os projetos do cliente com o tempo de cada um no período, em ordem de nome (nunca por horas: o
	// sistema não ranqueia). Os sem tempo também entram, com zero.
	Projects []DedicationProject `json:"projects"`
}

// DedicationProject é um projeto do cliente na dedicação, com o tempo dele no período.
type DedicationProject struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Seconds float64   `json:"seconds"`
}

// tracked diz se o cliente tem o que acompanhar: o contrato mensal com estimativa de horas.
func tracked(c *ent.Customer) bool {
	return c.BillingMode == KindMonthly && c.EstimatedMonthlyHours != nil && *c.EstimatedMonthlyHours > 0
}

// dedication é a dedicação do cliente no período p, com as sessões dele (as de todos os projetos). Nula quando o
// cliente não tem o que acompanhar ou quando p não é um período do contrato (contractPeriods): antes do início, depois
// do fim, ou o período de um dia do contrato que começa no dia de fechamento.
func (t terms) dedication(sessions []work_session.WorkSession, p payment.Period, now time.Time) *Dedication {
	c := t.customer
	if !tracked(c) || !slices.ContainsFunc(t.contractPeriods(now), func(q payment.Period) bool { return q.Start.Equal(p.Start) }) {
		return nil
	}
	byProject := map[uuid.UUID]float64{}
	var total float64
	for i := range sessions {
		sec, _, _ := sessions[i].WithinRange(p.Start, p.End, now)
		if sec <= 0 {
			continue
		}
		total += sec
		byProject[sessions[i].ProjectID] += sec
	}
	projects := make([]DedicationProject, 0, len(c.Edges.Projects))
	for _, pr := range c.Edges.Projects {
		projects = append(projects, DedicationProject{ID: pr.ID, Name: pr.Name, Seconds: byProject[pr.ID]})
	}
	byName := collate.New(language.Und)
	slices.SortFunc(projects, func(a, b DedicationProject) int {
		return cmp.Or(byName.CompareString(a.Name, b.Name), cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID.String(), b.ID.String()))
	})
	estimate := *c.EstimatedMonthlyHours
	return &Dedication{
		Customer: t.ref(), PeriodStart: p.StartDay(), PeriodEnd: p.PayDay(), Open: !closed(p, now),
		EstimatedHours: estimate, Seconds: total,
		Percent:  int(math.Round(total * 100 / (float64(estimate) * 3600))),
		Projects: projects,
	}
}

// dedicationAt é a dedicação do cliente no período do contrato que contém now, ou no anterior a ele com previous.
func (t terms) dedicationAt(sessions []work_session.WorkSession, now time.Time, previous bool) *Dedication {
	p, ok := t.cycle.at(now)
	if ok && previous {
		p, ok = payment.Previous(p)
	}
	if !ok {
		return nil
	}
	return t.dedication(sessions, p, now)
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
