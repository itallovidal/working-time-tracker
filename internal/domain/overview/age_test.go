package overview

import (
	"testing"
	"time"
)

// A idade do projeto sai em três medidas do mesmo tempo, cada uma arredondada
// para baixo: dias e semanas corridos, e meses de calendário já completos.
func TestAgeBetween(t *testing.T) {
	cases := []struct {
		name       string
		start, now string
		want       Age
	}{
		{"same instant", "2026-01-15T10:00:00Z", "2026-01-15T10:00:00Z", Age{}},
		{"hours into the first day", "2026-01-15T10:00:00Z", "2026-01-15T23:00:00Z", Age{}},
		{"one hour short of a week", "2026-01-15T10:00:00Z", "2026-01-22T09:00:00Z", Age{Days: 6}},
		{"a week", "2026-01-15T10:00:00Z", "2026-01-22T10:00:00Z", Age{Days: 7, Weeks: 1}},
		{"one day short of a month", "2026-01-15T10:00:00Z", "2026-02-14T10:00:00Z", Age{Days: 30, Weeks: 4}},
		{"one hour short of a month", "2026-01-15T10:00:00Z", "2026-02-15T09:00:00Z", Age{Days: 30, Weeks: 4}},
		{"a month", "2026-01-15T10:00:00Z", "2026-02-15T10:00:00Z", Age{Days: 31, Weeks: 4, Months: 1}},
		// Fevereiro não tem dia 31: o mês se completa no último dia dele.
		{"started on the 31st, one day before a shorter month ends", "2026-01-31T10:00:00Z", "2026-02-27T10:00:00Z", Age{Days: 27, Weeks: 3}},
		{"started on the 31st, on the last day of a shorter month", "2026-01-31T10:00:00Z", "2026-02-28T10:00:00Z", Age{Days: 28, Weeks: 4, Months: 1}},
		{"across the turn of the year", "2025-11-20T08:00:00Z", "2026-02-20T08:00:00Z", Age{Days: 92, Weeks: 13, Months: 3}},
		{"a year", "2025-03-10T08:00:00Z", "2026-03-10T08:00:00Z", Age{Days: 365, Weeks: 52, Months: 12}},
		{"written in another time zone", "2026-01-15T23:30:00-03:00", "2026-02-16T02:30:00Z", Age{Days: 31, Weeks: 4, Months: 1}},
		{"now before the start", "2026-05-01T00:00:00Z", "2026-04-01T00:00:00Z", Age{}},
	}
	for _, c := range cases {
		start, err := time.Parse(time.RFC3339, c.start)
		if err != nil {
			t.Fatalf("%s: start: %v", c.name, err)
		}
		now, err := time.Parse(time.RFC3339, c.now)
		if err != nil {
			t.Fatalf("%s: now: %v", c.name, err)
		}
		if got := ageBetween(start, now); got != c.want {
			t.Errorf("%s: ageBetween(%s, %s) = %+v, want %+v", c.name, c.start, c.now, got, c.want)
		}
	}
}
