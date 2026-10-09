package payment

import (
	"testing"
	"time"

	"working-time-tracker/internal/domain/person"
)

func loc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func monthlyRule(day int) person.PaymentRule {
	return person.PaymentRule{Frequency: person.PaymentMonthly, Day: day}
}

func biweeklyRule(start string) person.PaymentRule {
	return person.PaymentRule{Frequency: person.PaymentBiweekly, Start: start}
}

func TestPeriodAt_Monthly(t *testing.T) {
	sp := loc(t, "America/Sao_Paulo")
	cases := []struct {
		name       string
		day        int
		at         string
		start, pay string
	}{
		{"day 5 mid month", 5, "2026-10-09 12:00", "2026-10-06", "2026-11-05"},
		{"day 5 on the pay date", 5, "2026-10-05 23:59", "2026-09-06", "2026-10-05"},
		{"day 5 the day after", 5, "2026-10-06 00:00", "2026-10-06", "2026-11-05"},
		{"day 5 over the year", 5, "2026-12-20 10:00", "2026-12-06", "2027-01-05"},
		{"day 5 start of the year", 5, "2027-01-02 10:00", "2026-12-06", "2027-01-05"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, _ := time.ParseInLocation("2006-01-02 15:04", c.at, sp)
			p, ok := PeriodAt(monthlyRule(c.day), at, sp)
			if !ok || p.StartDay() != c.start || p.PayDay() != c.pay {
				t.Errorf("got %s..%s (ok %v), want %s..%s", p.StartDay(), p.PayDay(), ok, c.start, c.pay)
			}
		})
	}
}

func TestPeriodAt_MonthlyShortMonths(t *testing.T) {
	sp := loc(t, "America/Sao_Paulo")
	cases := []struct{ at, start, pay string }{
		{"2027-02-10 10:00", "2027-02-01", "2027-02-28"}, // fevereiro comum
		{"2028-02-10 10:00", "2028-02-01", "2028-02-29"}, // bissexto
		{"2026-04-10 10:00", "2026-04-01", "2026-04-30"}, // abril: o período anterior fechou dia 31/03
		{"2026-12-31 10:00", "2026-12-01", "2026-12-31"},
		{"2027-03-01 10:00", "2027-03-01", "2027-03-31"}, // fecha 28/02 e o próximo vai de 01/03
		{"2026-11-15 10:00", "2026-11-01", "2026-11-30"}, // novembro tem 30: paga no último dia
	}
	for _, c := range cases {
		at, _ := time.ParseInLocation("2006-01-02 15:04", c.at, sp)
		p, ok := PeriodAt(monthlyRule(31), at, sp)
		if !ok || p.StartDay() != c.start || p.PayDay() != c.pay {
			t.Errorf("%s: got %s..%s (ok %v), want %s..%s", c.at, p.StartDay(), p.PayDay(), ok, c.start, c.pay)
		}
	}
}

func TestPeriodAt_Biweekly(t *testing.T) {
	sp := loc(t, "America/Sao_Paulo")
	rule := biweeklyRule("2026-10-01")
	cases := []struct{ at, start, pay string }{
		{"2026-10-01 00:00", "2026-10-01", "2026-10-15"},
		{"2026-10-15 23:59", "2026-10-01", "2026-10-15"},
		{"2026-10-16 00:00", "2026-10-16", "2026-10-30"},
		{"2026-10-31 09:00", "2026-10-31", "2026-11-14"},
		{"2027-01-05 09:00", "2026-12-30", "2027-01-13"}, // atravessa o ano
	}
	for _, c := range cases {
		at, _ := time.ParseInLocation("2006-01-02 15:04", c.at, sp)
		p, ok := PeriodAt(rule, at, sp)
		if !ok || p.StartDay() != c.start || p.PayDay() != c.pay || p.Days() != 15 {
			t.Errorf("%s: got %s..%s days %d (ok %v), want %s..%s", c.at, p.StartDay(), p.PayDay(), p.Days(), ok, c.start, c.pay)
		}
	}
	before, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-30 23:59", sp)
	if _, ok := PeriodAt(rule, before, sp); ok {
		t.Error("before the start there should be no period")
	}
}

func TestUpcoming_BiweeklyStartingInTheFuture(t *testing.T) {
	sp := loc(t, "America/Sao_Paulo")
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-09-20 10:00", sp)
	up := Upcoming(biweeklyRule("2026-10-01"), now, 3, sp)
	if len(up) != 3 || up[0].StartDay() != "2026-10-01" || up[0].PayDay() != "2026-10-15" || up[2].PayDay() != "2026-11-14" {
		t.Errorf("upcoming = %+v", up)
	}
}

func TestPreviousAndNext(t *testing.T) {
	sp := loc(t, "America/Sao_Paulo")
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-20 10:00", sp)
	p, _ := PeriodAt(biweeklyRule("2026-10-01"), at, sp)
	prev, ok := Previous(p)
	if !ok || prev.PayDay() != "2026-10-15" {
		t.Errorf("previous = %s (ok %v)", prev.PayDay(), ok)
	}
	if _, ok := Previous(prev); ok {
		t.Error("nothing comes before the first biweekly period")
	}
	if n := Next(prev); n.PayDay() != p.PayDay() {
		t.Errorf("next = %s, want %s", n.PayDay(), p.PayDay())
	}
	m, _ := PeriodAt(monthlyRule(5), at, sp)
	pm, _ := Previous(m)
	if pm.PayDay() != "2026-10-05" || Next(pm).PayDay() != m.PayDay() {
		t.Errorf("monthly previous/next = %s / %s", pm.PayDay(), Next(pm).PayDay())
	}
}

// A virada do horário de verão no meio do período não desloca nenhum dia: o período dura 15 dias de calendário, ainda
// que um deles tenha 23 ou 25 horas.
func TestPeriodAt_DaylightSaving(t *testing.T) {
	ny := loc(t, "America/New_York")
	rule := biweeklyRule("2026-10-25") // 01/11/2026 é a virada para o horário padrão
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-11-03 12:00", ny)
	p, ok := PeriodAt(rule, at, ny)
	if !ok || p.StartDay() != "2026-10-25" || p.PayDay() != "2026-11-08" || p.Days() != 15 {
		t.Fatalf("got %s..%s days %d", p.StartDay(), p.PayDay(), p.Days())
	}
	if h := p.End.Sub(p.Start).Hours(); h != 15*24+1 {
		t.Errorf("period is %vh long, want 361h (one extra hour of the fall-back)", h)
	}
	// Em cima da meia-noite do fim: já é o período seguinte.
	end := p.End
	if next, _ := PeriodAt(rule, end, ny); next.StartDay() != "2026-11-09" {
		t.Errorf("at the exact end got %s", next.StartDay())
	}
	if last, _ := PeriodAt(rule, end.Add(-time.Nanosecond), ny); last.StartDay() != "2026-10-25" {
		t.Errorf("a nanosecond before the end got %s", last.StartDay())
	}
}
