package receivable

import (
	"math"
	"time"

	"working-time-tracker/ent"
	"working-time-tracker/internal/domain/customer"
	"working-time-tracker/internal/domain/payment"
	"working-time-tracker/internal/domain/person"
	"working-time-tracker/internal/domain/work_session"
	"working-time-tracker/internal/validate"
)

// cycle é o ciclo de cobrança de um cliente, com a mesma conta da regra de pagamento da pessoa (payment.PeriodAt,
// Previous e Next), que este pacote só usa. O período fecha à meia-noite do último dia, no fuso da organização, e os
// dois dias são inclusivos.
//   - Mensal: o período fecha no closing_day do cliente (31 é o último dia do mês, e um mês mais curto fecha no último
//     dia) e vai do dia seguinte ao fechamento do mês anterior até o fechamento do mês.
//   - A cada 14 dias (o bi-weekly dos EUA): os períodos começam no dia seguinte à âncora do cliente (o fim do último
//     período já pago fora do app) e têm 14 dias cada. Nada até a âncora é período: as horas de lá nunca são propostas.
type cycle struct {
	rule person.PaymentRule
	loc  *time.Location
}

func newCycle(closingDay int, loc *time.Location) cycle {
	return cycle{rule: person.PaymentRule{Frequency: person.PaymentMonthly, Day: closingDay}, loc: loc}
}

// newBiweeklyCycle é o ciclo de 14 dias que começa no dia seguinte à âncora (YYYY-MM-DD). Uma âncora que não é um dia
// dá um ciclo sem períodos: nada é proposto nem confirmado, em vez de períodos errados.
func newBiweeklyCycle(anchor string, loc *time.Location) cycle {
	start := ""
	if validDay(anchor) {
		start = addDays(anchor, 1)
	}
	return cycle{rule: person.PaymentRule{Frequency: person.PaymentBiweekly, Start: start, PeriodDays: customer.CycleDays}, loc: loc}
}

// cycleOf é o ciclo do cliente, pelo billing_cycle dele.
func cycleOf(c *ent.Customer, loc *time.Location) cycle {
	if c.BillingCycle == customer.CycleBiweekly {
		anchor := ""
		if c.CycleAnchor != nil {
			anchor = *c.CycleAnchor
		}
		return newBiweeklyCycle(anchor, loc)
	}
	return newCycle(c.ClosingDay, loc)
}

// maxClosed é quantos períodos fechados do cliente por hora a lista A confirmar traz: os de cerca de dois anos nos
// dois ciclos.
func (c cycle) maxClosed() int {
	if c.rule.Frequency == person.PaymentBiweekly {
		return MaxBiweeklyProposals
	}
	return MaxHourlyProposals
}

// at é o período que contém o instante t.
func (c cycle) at(t time.Time) (payment.Period, bool) {
	return payment.PeriodAt(c.rule, t, c.loc)
}

// ofDay é o período que contém o dia YYYY-MM-DD; ok falso para um texto que não é um dia.
func (c cycle) ofDay(day string) (payment.Period, bool) {
	t, err := time.ParseInLocation(time.DateOnly, day, c.loc)
	if err != nil {
		return payment.Period{}, false
	}
	return c.at(t)
}

// exact é o período que começa em start e fecha em end (YYYY-MM-DD, inclusivos); ok falso quando as duas datas não
// são as de um período do ciclo.
func (c cycle) exact(start, end string) (payment.Period, bool) {
	p, ok := c.ofDay(start)
	if !ok || p.StartDay() != start || p.PayDay() != end {
		return payment.Period{}, false
	}
	return p, true
}

// closed diz que o período já terminou em now: fecha à meia-noite depois do último dia.
func closed(p payment.Period, now time.Time) bool { return !p.End.After(now) }

// location carrega o fuso da organização; um nome inválido cai para UTC.
func location(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// addDays soma n dias de calendário a um dia YYYY-MM-DD, sem depender de fuso nem de horário de verão. Um texto que
// não é um dia volta como veio.
func addDays(day string, n int) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format(time.DateOnly)
}

// daysBetween são os dias de calendário de a até b (negativo se b vem antes). Um texto que não é um dia vale zero.
func daysBetween(a, b string) int {
	ta, errA := time.Parse(time.DateOnly, a)
	tb, errB := time.Parse(time.DateOnly, b)
	if errA != nil || errB != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

// validDay diz se v é um dia de calendário YYYY-MM-DD que existe.
func validDay(v string) bool {
	t, err := time.Parse(time.DateOnly, v)
	return err == nil && t.Format(time.DateOnly) == v
}

// sumWithin soma o que as sessões têm em [from, to): o tempo e o valor cobrado. O valor é por sessão: cada uma é
// cortada no intervalo e arredondada ao centavo pelo valor cobrado congelado nela (o do projeto dela), como em payment
// e overview, então a soma bate com as linhas. O tempo conta uma vez só o que a mesma pessoa trabalhou ao mesmo tempo
// em dois projetos do cliente (Decisão 26). A sessão aberta conta até now; a sem valor cobrado conta o tempo e nenhum
// centavo.
func sumWithin(sessions []work_session.WorkSession, from, to, now time.Time) (seconds float64, cents int) {
	for i := range sessions {
		sec, _, bill := sessions[i].WithinRange(from, to, now)
		if sec <= 0 {
			continue
		}
		if bill != nil {
			cents += *bill
		}
	}
	return work_session.MergedSeconds(sessions, from, to, now), cents
}

// effectiveRate é o valor por hora que cents rendem em seconds: o valor sobre as horas, ao centavo. Nulo sem horas.
// below diz que ficou abaixo do piso de valor por hora, o que é um aviso e não um erro.
func effectiveRate(cents int, seconds float64) (rate *int, below bool) {
	if seconds <= 0 {
		return nil, false
	}
	r := int(math.Round(float64(cents) * 3600 / seconds))
	return &r, r < validate.MinRateCents
}
