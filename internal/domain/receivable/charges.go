package receivable

// lateCharges calcula a multa e os juros de um recebível mensal pago em on. Só há cobrança quando on passa de
// vencimento + tolerância; a tolerância adia a cobrança, não a conta: os dias de atraso contam do vencimento.
//
//	multa  = valor × late_fee_bp / 10000
//	juros  = valor × late_interest_bp_month / 10000 × diasAtraso / 30
//
// Os dois são arredondados ao centavo (meio centavo sobe). Os pontos-base são os do cliente: 200 é 2%.
func lateCharges(amountCents, feeBp, interestBpMonth, graceDays int, due, on string) LateCharges {
	days := max(0, daysBetween(due, on))
	out := LateCharges{
		On: on, DueDate: due, GraceDays: graceDays, AmountCents: amountCents, DaysLate: days,
		Charged: daysBetween(due, on) > graceDays,
	}
	if out.Charged {
		out.LateFeeCents = roundDiv(amountCents*feeBp, 10000)
		out.InterestCents = roundDiv(amountCents*interestBpMonth*days, 10000*interestMonthDays)
	}
	out.TotalCents = amountCents + out.LateFeeCents + out.InterestCents
	return out
}

// roundDiv é n/d arredondado ao inteiro mais próximo, com o meio para cima. Vale para n ≥ 0 e d > 0.
func roundDiv(n, d int) int {
	return (2*n + d) / (2 * d)
}
