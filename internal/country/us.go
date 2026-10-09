package country

import "working-time-tracker/internal/validate"

var unitedStates = &Country{
	Code: "US",
	Aliases: []string{
		"usa", "u.s.", "u.s.a.", "eua", "united states", "united states of america",
		"estados unidos", "estados unidos da américa", "estados unidos da america",
	},
	Langs:    []string{"en"},
	Currency: "USD",
	// O país tem vários fusos; o do leste é o padrão, e a pessoa troca na edição.
	Timezone: "America/New_York",
	LegalID: LegalID{
		Field:     "ein",
		Mask:      "99-9999999",
		Normalize: validate.EIN,
	},
}
