// Package payment calcula, a partir da regra de pagamento de cada pessoa e das sessões dela, os períodos de
// pagamento: o corrente, os próximos e os já fechados, com as horas e o valor de cada um. Não tem tabela: nada é
// registrado como pago, tudo é calculado na hora.
package payment

import (
	"time"

	"working-time-tracker/internal/domain/person"
)

// payPeriodDays é o tamanho do período quinzenal.
const payPeriodDays = 15

// Period é um período de pagamento: o intervalo [Start, End) em instantes (meia-noite no fuso da organização) e a
// data do pagamento, que é o último dia do período (meia-noite desse dia). Os instantes servem só para cortar
// sessões; para a tela, o que vale são os dias de calendário (StartDay, PayDay).
type Period struct {
	Start, End, PayDate time.Time
	rule                person.PaymentRule
	loc                 *time.Location
}

// StartDay e PayDay são os dias de calendário do período, YYYY-MM-DD no fuso da organização, ambos inclusivos.
func (p Period) StartDay() string { return p.Start.Format(time.DateOnly) }
func (p Period) PayDay() string   { return p.PayDate.Format(time.DateOnly) }

// Days é quantos dias de calendário o período tem.
func (p Period) Days() int { return daysBetween(p.Start, p.End) }

// midnight é a meia-noite do dia de t no fuso loc.
func midnight(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// daysBetween conta os dias de calendário de a até b (meias-noites), sem depender do horário de verão.
func daysBetween(a, b time.Time) int {
	ua := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	ub := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(ub.Sub(ua).Hours() / 24)
}

// monthlyPayDate é a data de pagamento do mês: o dia da regra, ou o último dia do mês quando ele é mais curto.
func monthlyPayDate(day, year int, month time.Month, loc *time.Location) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	return time.Date(year, month, min(day, last), 0, 0, 0, 0, loc)
}

func monthly(rule person.PaymentRule, payDate time.Time, loc *time.Location) Period {
	prev := monthlyPayDate(rule.Day, payDate.Year(), payDate.Month()-1, loc)
	return Period{
		Start:   prev.AddDate(0, 0, 1),
		End:     payDate.AddDate(0, 0, 1),
		PayDate: payDate,
		rule:    rule,
		loc:     loc,
	}
}

// biweeklyStart é o dia de início da contagem quinzenal, ou false quando a regra não tem um válido.
func biweeklyStart(rule person.PaymentRule, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation(time.DateOnly, rule.Start, loc)
	return t, err == nil
}

func biweekly(rule person.PaymentRule, s time.Time, k int, loc *time.Location) Period {
	return Period{
		Start:   s.AddDate(0, 0, payPeriodDays*k),
		End:     s.AddDate(0, 0, payPeriodDays*(k+1)),
		PayDate: s.AddDate(0, 0, payPeriodDays*k+payPeriodDays-1),
		rule:    rule,
		loc:     loc,
	}
}

// PeriodAt devolve o período que contém o instante t. Nos quinzenais, antes do início da contagem não há período
// (ok falso). Uma regra inválida também dá ok falso.
func PeriodAt(rule person.PaymentRule, t time.Time, loc *time.Location) (Period, bool) {
	day := midnight(t, loc)
	switch rule.Frequency {
	case person.PaymentMonthly:
		if rule.Day < 1 || rule.Day > 31 {
			return Period{}, false
		}
		pay := monthlyPayDate(rule.Day, day.Year(), day.Month(), loc)
		if day.After(pay) {
			pay = monthlyPayDate(rule.Day, day.Year(), day.Month()+1, loc)
		}
		return monthly(rule, pay, loc), true
	case person.PaymentBiweekly:
		s, ok := biweeklyStart(rule, loc)
		if !ok || day.Before(s) {
			return Period{}, false
		}
		return biweekly(rule, s, daysBetween(s, day)/payPeriodDays, loc), true
	}
	return Period{}, false
}

// Previous é o período anterior; ok falso quando não há (o quinzenal não volta antes do início da contagem).
func Previous(p Period) (Period, bool) {
	switch p.rule.Frequency {
	case person.PaymentMonthly:
		prev := monthlyPayDate(p.rule.Day, p.PayDate.Year(), p.PayDate.Month()-1, p.loc)
		return monthly(p.rule, prev, p.loc), true
	case person.PaymentBiweekly:
		s, ok := biweeklyStart(p.rule, p.loc)
		if !ok || !p.Start.After(s) {
			return Period{}, false
		}
		return biweekly(p.rule, s, daysBetween(s, p.Start)/payPeriodDays-1, p.loc), true
	}
	return Period{}, false
}

// Next é o período seguinte.
func Next(p Period) Period {
	if p.rule.Frequency == person.PaymentMonthly {
		return monthly(p.rule, monthlyPayDate(p.rule.Day, p.PayDate.Year(), p.PayDate.Month()+1, p.loc), p.loc)
	}
	s, _ := biweeklyStart(p.rule, p.loc)
	return biweekly(p.rule, s, daysBetween(s, p.Start)/payPeriodDays+1, p.loc)
}

// Upcoming devolve os n próximos períodos a pagar a partir de from: o que está em curso (ou que paga hoje) e os
// seguintes. Num quinzenal que ainda não começou, o primeiro é o que começa na data de início.
func Upcoming(rule person.PaymentRule, from time.Time, n int, loc *time.Location) []Period {
	if n <= 0 {
		return nil
	}
	p, ok := PeriodAt(rule, from, loc)
	if !ok {
		s, valid := biweeklyStart(rule, loc)
		if rule.Frequency != person.PaymentBiweekly || !valid {
			return nil
		}
		p = biweekly(rule, s, 0, loc)
	}
	out := []Period{p}
	for len(out) < n {
		p = Next(p)
		out = append(out, p)
	}
	return out
}
