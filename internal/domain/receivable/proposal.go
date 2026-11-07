package receivable

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"working-time-tracker/ent"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/domain/work_session"
)

// terms são os termos comerciais do cliente de que as propostas dependem, lidos do cadastro dele. extraCents é a soma
// dos acréscimos dos projetos do cliente (Decisão 21): no mensal, eles somam à mensalidade de cada período; no por hora
// o acréscimo já está no valor cobrado congelado em cada sessão, e extraCents não entra na conta.
type terms struct {
	customer    *ent.Customer
	cycle       cycle
	paymentDays int
	extraCents  int
}

// termsOf monta os termos do cliente, que precisa vir com os projetos carregados (Store.Customer e Store.Customers).
func termsOf(c *ent.Customer, loc *time.Location) terms {
	extra := 0
	for _, p := range c.Edges.Projects {
		extra += p.BillExtraCents
	}
	return terms{customer: c, cycle: cycleOf(c, loc), paymentDays: c.PaymentDays, extraCents: extra}
}

// monthlyCents é o valor de um período do contrato mensal: a mensalidade mais os acréscimos mensais dos projetos.
// Vale o que está cadastrado agora, inclusive para um período passado ainda sem recebível: o dono confere ao confirmar.
func (t terms) monthlyCents() int {
	return *t.customer.MonthlyFeeCents + t.extraCents
}

func (t terms) ref() Ref { return Ref{ID: t.customer.ID, Name: t.customer.Name} }

// key identifica o período de um cliente, o mesmo do índice único dos recebíveis.
func key(customerID uuid.UUID, start, end string) string {
	return customerID.String() + "|" + start + "|" + end
}

// proposal monta a proposta de um período com o tempo e o valor dados.
func (t terms) proposal(kind string, p payment.Period, seconds float64, cents int, now time.Time) Proposal {
	rate, below := effectiveRate(cents, seconds)
	return Proposal{
		Customer: t.ref(), Kind: kind,
		PeriodStart: p.StartDay(), PeriodEnd: p.PayDay(),
		Seconds: seconds, CalculatedCents: cents,
		DueDate:            addDays(p.PayDay(), t.paymentDays),
		Open:               !closed(p, now),
		EffectiveRateCents: rate, BelowMinRate: below,
	}
}

// hourly propõe os períodos do cliente por hora: os já fechados com tempo nos projetos dele, os mais recentes primeiro
// até o teto do ciclo (MaxHourlyProposals no mensal, MaxBiweeklyProposals no de 14 dias), sem os que já foram
// confirmados (done). No de 14 dias nada vem de antes da âncora, e com a âncora hoje ainda não há período nenhum. O valor de cada um é a soma das sessões de todos os
// projetos do cliente, cortadas no período e arredondadas por sessão pelo valor cobrado congelado nelas. Com
// includeOpen entra também o período corrente, com as horas até agora; ele não conta no limite.
func (t terms) hourly(sessions []work_session.WorkSession, done map[string]bool, now time.Time, includeOpen bool) []Proposal {
	cur, ok := t.cycle.at(now)
	if !ok || len(sessions) == 0 {
		return nil
	}
	// Nada antes da primeira sessão tem tempo: é onde a busca para.
	first := sessions[0].StartAt
	for i := range sessions {
		if sessions[i].StartAt.Before(first) {
			first = sessions[i].StartAt
		}
	}
	var out []Proposal
	if includeOpen && !done[key(t.customer.ID, cur.StartDay(), cur.PayDay())] {
		if sec, cents := sumWithin(sessions, cur.Start, cur.End, now); sec > 0 {
			out = append(out, t.proposal(KindHourly, cur, sec, cents, now))
		}
	}
	closedCount := 0
	for p, ok := payment.Previous(cur); ok && closedCount < t.cycle.maxClosed() && p.End.After(first); p, ok = payment.Previous(p) {
		if done[key(t.customer.ID, p.StartDay(), p.PayDay())] {
			continue
		}
		if sec, cents := sumWithin(sessions, p.Start, p.End, now); sec > 0 {
			out = append(out, t.proposal(KindHourly, p, sec, cents, now))
			closedCount++
		}
	}
	return out
}

// contractPeriods são os períodos do contrato mensal que se podem cobrar: do período do contract_start até o corrente,
// parando no contract_end (o período que o contém é o último). O corrente se confirma adiantado, a qualquer momento do
// mês; o seguinte só entra quando começa (propor os dois parecia cobrança em dobro). Vazio quando o cliente não tem
// mensalidade e início de contrato, ou quando o contrato só começa depois do período corrente. Do mais antigo para o
// mais novo.
func (t terms) contractPeriods(now time.Time) []payment.Period {
	c := t.customer
	if c.MonthlyFeeCents == nil || *c.MonthlyFeeCents <= 0 || c.ContractStart == nil {
		return nil
	}
	first, ok := t.cycle.ofDay(*c.ContractStart)
	if !ok {
		return nil
	}
	cur, ok := t.cycle.at(now)
	if !ok {
		return nil
	}
	var out []payment.Period
	for p := first; !p.Start.After(cur.Start); p = payment.Next(p) {
		if c.ContractEnd != nil && p.StartDay() > *c.ContractEnd {
			break
		}
		out = append(out, p)
	}
	return out
}

// monthly propõe os períodos do contrato mensal que ainda não foram confirmados, do mais novo ao limite
// MaxMonthlyProposals. O valor é a mensalidade mais os acréscimos mensais dos projetos (ExtraCents diz quanto deles);
// as horas são as dos projetos do cliente no período, só para a conta do valor por hora efetivo. O primeiro mês parcial
// o dono corrige ao confirmar: não há pro-rata.
func (t terms) monthly(sessions []work_session.WorkSession, done map[string]bool, now time.Time) []Proposal {
	periods := t.contractPeriods(now)
	var out []Proposal
	for i := len(periods) - 1; i >= 0 && len(out) < MaxMonthlyProposals; i-- {
		p := periods[i]
		if done[key(t.customer.ID, p.StartDay(), p.PayDay())] {
			continue
		}
		sec, _ := sumWithin(sessions, p.Start, p.End, now)
		prop := t.proposal(KindMonthly, p, sec, t.monthlyCents(), now)
		prop.ExtraCents = t.extraCents
		out = append(out, prop)
	}
	return out
}

// proposals é o que se propõe para o cliente, pelo modo de cobrança dele.
func (t terms) proposals(sessions []work_session.WorkSession, done map[string]bool, now time.Time, includeOpen bool) []Proposal {
	switch t.customer.BillingMode {
	case KindHourly:
		return t.hourly(sessions, done, now, includeOpen)
	case KindMonthly:
		return t.monthly(sessions, done, now)
	}
	return nil
}

// sortProposals põe as propostas na ordem da tela: o período que fecha primeiro vem antes, e no mesmo dia o cliente de
// nome mais cedo.
func sortProposals(list []Proposal) {
	slices.SortFunc(list, func(a, b Proposal) int {
		if c := strings.Compare(a.PeriodEnd, b.PeriodEnd); c != 0 {
			return c
		}
		if c := strings.Compare(a.Customer.Name, b.Customer.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Customer.ID.String(), b.Customer.ID.String())
	})
}
