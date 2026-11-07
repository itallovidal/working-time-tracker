package receivable

import "testing"

// O exemplo da especificação: 6.300,00 com 2% de multa e 1% ao mês de juros, pagos 10 dias depois do vencimento, são
// 126,00 de multa, 21,00 de juros e 6.447,00 no total.
func TestLateCharges_SpecExample(t *testing.T) {
	got := lateCharges(630000, 200, 100, 0, "2026-10-05", "2026-10-15")
	want := LateCharges{
		On: "2026-10-15", DueDate: "2026-10-05", AmountCents: 630000, DaysLate: 10, Charged: true,
		LateFeeCents: 12600, InterestCents: 2100, TotalCents: 644700,
	}
	if got != want {
		t.Errorf("lateCharges = %+v, want %+v", got, want)
	}
}

func TestLateCharges_GraceAndDays(t *testing.T) {
	cases := []struct {
		name                            string
		amount, fee, interest, grace    int
		on                              string
		wantCharged                     bool
		wantDays, wantFee, wantInterest int
	}{
		{"before the due date", 630000, 200, 100, 0, "2026-10-01", false, 0, 0, 0},
		{"on the due date", 630000, 200, 100, 0, "2026-10-05", false, 0, 0, 0},
		{"the day after, no grace", 630000, 200, 100, 0, "2026-10-06", true, 1, 12600, 210},
		{"the last day of the grace is still free", 630000, 200, 100, 5, "2026-10-10", false, 5, 0, 0},
		{"one day past the grace charges from the due date", 630000, 200, 100, 5, "2026-10-11", true, 6, 12600, 1260},
		{"a month late", 630000, 200, 100, 0, "2026-11-04", true, 30, 12600, 6300},
		{"no fee configured", 630000, 0, 100, 0, "2026-10-15", true, 10, 0, 2100},
		{"no interest configured", 630000, 200, 0, 0, "2026-10-15", true, 10, 12600, 0},
		// 10,00 a 2%: 0,20; juros 10,00 × 1% × 1/30 = 0,0033 -> 0.
		{"rounding down to the cent", 1000, 200, 100, 0, "2026-10-06", true, 1, 20, 0},
		// 100,00 × 1% × 15/30 = 0,50 exatos; 100,00 × 1% × 16/30 = 0,5333 -> 0,53.
		{"interest in cents", 10000, 0, 100, 0, "2026-10-21", true, 16, 0, 53},
		// 3,00 × 1,5% = 0,045 -> o meio centavo sobe para 0,05.
		{"half a cent goes up", 300, 150, 0, 0, "2026-10-06", true, 1, 5, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lateCharges(c.amount, c.fee, c.interest, c.grace, "2026-10-05", c.on)
			if got.Charged != c.wantCharged || got.DaysLate != c.wantDays || got.LateFeeCents != c.wantFee || got.InterestCents != c.wantInterest {
				t.Errorf("got charged=%v days=%d fee=%d interest=%d, want charged=%v days=%d fee=%d interest=%d",
					got.Charged, got.DaysLate, got.LateFeeCents, got.InterestCents, c.wantCharged, c.wantDays, c.wantFee, c.wantInterest)
			}
			if got.TotalCents != c.amount+got.LateFeeCents+got.InterestCents {
				t.Errorf("total = %d, want the amount plus both charges", got.TotalCents)
			}
		})
	}
}
