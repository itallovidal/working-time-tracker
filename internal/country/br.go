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
	Address: Address{
		Postal: Postal{Mask: "99999-999", Normalize: validate.CEP},
		States: []State{
			{"AC", "Acre"}, {"AL", "Alagoas"}, {"AP", "Amapá"}, {"AM", "Amazonas"}, {"BA", "Bahia"},
			{"CE", "Ceará"}, {"DF", "Distrito Federal"}, {"ES", "Espírito Santo"}, {"GO", "Goiás"},
			{"MA", "Maranhão"}, {"MT", "Mato Grosso"}, {"MS", "Mato Grosso do Sul"}, {"MG", "Minas Gerais"},
			{"PA", "Pará"}, {"PB", "Paraíba"}, {"PR", "Paraná"}, {"PE", "Pernambuco"}, {"PI", "Piauí"},
			{"RJ", "Rio de Janeiro"}, {"RN", "Rio Grande do Norte"}, {"RS", "Rio Grande do Sul"},
			{"RO", "Rondônia"}, {"RR", "Roraima"}, {"SC", "Santa Catarina"}, {"SP", "São Paulo"},
			{"SE", "Sergipe"}, {"TO", "Tocantins"},
		},
	},
}
