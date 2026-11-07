package receivable

import (
	"testing"
	"time"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func instant(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// O ciclo é o da regra mensal de pagamento: fecha no closing_day, o 31 é o último dia do mês e um mês curto fecha no
// último dia dele. O período vai do dia seguinte ao fechamento do mês anterior até o fechamento, os dois inclusivos.
func TestCycle_ClosingDay(t *testing.T) {
	sp := zone(t, "America/Sao_Paulo")
	cases := []struct {
		name       string
		closingDay int
		at         string
		start, end string
	}{
		{"day 31 in a 31-day month", 31, "2026-10-09T15:00:00Z", "2026-10-01", "2026-10-31"},
		{"day 31 in a 30-day month", 31, "2026-11-09T15:00:00Z", "2026-11-01", "2026-11-30"},
		{"day 31 in a common February", 31, "2027-02-10T15:00:00Z", "2027-02-01", "2027-02-28"},
		{"day 31 in a leap February", 31, "2028-02-10T15:00:00Z", "2028-02-01", "2028-02-29"},
		{"day 30 closes on the 28th of February", 30, "2027-02-10T15:00:00Z", "2027-01-31", "2027-02-28"},
		{"day 15 mid month", 15, "2026-10-09T15:00:00Z", "2026-09-16", "2026-10-15"},
		{"day 15 the day after", 15, "2026-10-16T15:00:00Z", "2026-10-16", "2026-11-15"},
		{"day 15 over the year", 15, "2026-12-20T15:00:00Z", "2026-12-16", "2027-01-15"},
		{"day 1 closes on the first", 1, "2026-10-09T15:00:00Z", "2026-10-02", "2026-11-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := newCycle(c.closingDay, sp).at(instant(t, c.at))
			if !ok || p.StartDay() != c.start || p.PayDay() != c.end {
				t.Errorf("got %s..%s (ok %v), want %s..%s", p.StartDay(), p.PayDay(), ok, c.start, c.end)
			}
		})
	}
}

// O ciclo fecha à meia-noite no fuso da organização, não da UTC: em São Paulo (UTC-3) setembro fecha às 03:00 UTC de
// 1º de outubro; em Tóquio (UTC+9), às 15:00 UTC de 30 de setembro.
func TestCycle_ClosesAtMidnightInTheOrganizationZone(t *testing.T) {
	for _, c := range []struct {
		zone       string
		before     string // ainda não fechou
		closedAt   string // já fechou
		wantPeriod string
	}{
		{"America/Sao_Paulo", "2026-10-01T02:59:59Z", "2026-10-01T03:00:00Z", "2026-09-01"},
		{"Asia/Tokyo", "2026-09-30T14:59:59Z", "2026-09-30T15:00:00Z", "2026-09-01"},
	} {
		t.Run(c.zone, func(t *testing.T) {
			cyc := newCycle(31, zone(t, c.zone))
			before, ok := cyc.at(instant(t, c.before))
			if !ok || before.StartDay() != c.wantPeriod {
				t.Fatalf("at %s the period is %s, want the one that starts on %s", c.before, before.StartDay(), c.wantPeriod)
			}
			if closed(before, instant(t, c.before)) {
				t.Errorf("the period is closed at %s, one second before the midnight", c.before)
			}
			if !closed(before, instant(t, c.closedAt)) {
				t.Errorf("the period is still open at %s, the midnight", c.closedAt)
			}
			after, _ := cyc.at(instant(t, c.closedAt))
			if after.StartDay() == c.wantPeriod {
				t.Errorf("at %s the period is still %s: the next one should have begun", c.closedAt, c.wantPeriod)
			}
		})
	}
}

// exact só aceita as duas datas de um período do ciclo, e nenhuma outra combinação.
func TestCycle_Exact(t *testing.T) {
	cyc := newCycle(31, zone(t, "America/Sao_Paulo"))
	for _, c := range []struct {
		start, end string
		ok         bool
	}{
		{"2026-09-01", "2026-09-30", true},
		{"2026-10-01", "2026-10-31", true},
		{"2027-02-01", "2027-02-28", true},
		{"2026-09-02", "2026-09-30", false}, // começa um dia depois
		{"2026-09-01", "2026-09-29", false}, // fecha um dia antes
		{"2026-09-01", "2026-10-31", false}, // dois períodos
		{"2026-09-30", "2026-09-01", false},
		{"nope", "2026-09-30", false},
	} {
		if _, ok := cyc.exact(c.start, c.end); ok != c.ok {
			t.Errorf("exact(%s, %s) = %v, want %v", c.start, c.end, ok, c.ok)
		}
	}
}

func TestDays(t *testing.T) {
	if got := addDays("2026-09-30", 5); got != "2026-10-05" {
		t.Errorf("addDays = %s, want 2026-10-05", got)
	}
	if got := addDays("2026-12-30", 5); got != "2027-01-04" {
		t.Errorf("addDays over the year = %s, want 2027-01-04", got)
	}
	if got := daysBetween("2026-10-05", "2026-10-15"); got != 10 {
		t.Errorf("daysBetween = %d, want 10", got)
	}
	if got := daysBetween("2026-10-15", "2026-10-05"); got != -10 {
		t.Errorf("daysBetween backwards = %d, want -10", got)
	}
	if !validDay("2028-02-29") || validDay("2027-02-29") || validDay("2026-13-01") || validDay("26-10-01") || validDay("") {
		t.Error("validDay accepts a day that does not exist or refuses one that does")
	}
}

// O valor por hora efetivo é o valor sobre as horas; abaixo do piso de 10,00 é um aviso. Sem horas não há valor por hora.
func TestEffectiveRate(t *testing.T) {
	if r, below := effectiveRate(20000, 7200); r == nil || *r != 10000 || below {
		t.Errorf("200,00 in 2h = %v / %v, want 100,00 and no warning", r, below)
	}
	if r, below := effectiveRate(2000, 7200); r == nil || *r != 1000 || below {
		t.Errorf("20,00 in 2h = %v / %v, want 10,00 (the floor itself is not below it)", r, below)
	}
	if r, below := effectiveRate(1999, 7200); r == nil || *r != 1000 || below {
		t.Errorf("19,99 in 2h rounds to 10,00 = %v / %v", r, below)
	}
	if r, below := effectiveRate(1900, 7200); r == nil || *r != 950 || !below {
		t.Errorf("19,00 in 2h = %v / %v, want 9,50 and a warning", r, below)
	}
	if r, below := effectiveRate(50000, 0); r != nil || below {
		t.Errorf("no hours = %v / %v, want no rate", r, below)
	}
}
