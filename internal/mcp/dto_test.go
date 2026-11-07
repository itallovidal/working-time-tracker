package mcp

import (
	"errors"
	"testing"
	"time"
)

// Um dia vale às 23:59 do relógio no fuso da organização, como a tela grava, também no dia em que o horário de verão
// começa ou termina (o dia não tem 24 horas, e somar 23h59 à meia-noite cairia em outra hora ou no dia seguinte).
func TestParseDeadline_DayIsEndOfDayOnTheClock(t *testing.T) {
	cases := []struct {
		zone, day string
	}{
		{"America/Sao_Paulo", "2026-11-20"},
		{"America/New_York", "2026-03-08"}, // o relógio adianta: o dia tem 23 horas
		{"America/New_York", "2026-11-01"}, // o relógio atrasa: o dia tem 25 horas
	}
	for _, c := range cases {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Fatalf("load %s: %v", c.zone, err)
		}
		got, err := parseDeadline(c.day, loc)
		if err != nil {
			t.Fatalf("parseDeadline(%q, %s): %v", c.day, c.zone, err)
		}
		local := got.In(loc)
		if local.Format(time.DateOnly) != c.day || local.Hour() != 23 || local.Minute() != 59 {
			t.Errorf("parseDeadline(%q, %s) = %s, want %s 23:59", c.day, c.zone, local.Format(time.RFC3339), c.day)
		}
	}
}

func TestParseDeadline_InstantAndInvalid(t *testing.T) {
	loc := time.UTC
	got, err := parseDeadline("2026-12-01T15:04:05-03:00", loc)
	if err != nil || !got.Equal(time.Date(2026, 12, 1, 18, 4, 5, 0, time.UTC)) {
		t.Errorf("RFC 3339 instant = %v, %v", got, err)
	}
	if _, err := parseDeadline("01/12/2026", loc); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("invalid date error = %v, want mcp.invalid_date", err)
	}
}
