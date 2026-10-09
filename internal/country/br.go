package country

import "working-time-tracker/internal/validate"

var brazil = &Country{
	Code:     "BR",
	Aliases:  []string{"brasil", "brazil"},
	Langs:    []string{"pt"},
	Currency: "BRL",
	Timezone: "America/Sao_Paulo",
	LegalID: LegalID{
		Field:     "cnpj",
		Mask:      "**.***.***/****-99",
		Normalize: validate.CNPJ,
	},
}
