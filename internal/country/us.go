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
	Address: Address{
		Postal: Postal{Mask: "99999-9999", Normalize: validate.ZIP},
		States: []State{
			{"AL", "Alabama"}, {"AK", "Alaska"}, {"AZ", "Arizona"}, {"AR", "Arkansas"}, {"CA", "California"},
			{"CO", "Colorado"}, {"CT", "Connecticut"}, {"DE", "Delaware"}, {"FL", "Florida"}, {"GA", "Georgia"},
			{"HI", "Hawaii"}, {"ID", "Idaho"}, {"IL", "Illinois"}, {"IN", "Indiana"}, {"IA", "Iowa"},
			{"KS", "Kansas"}, {"KY", "Kentucky"}, {"LA", "Louisiana"}, {"ME", "Maine"}, {"MD", "Maryland"},
			{"MA", "Massachusetts"}, {"MI", "Michigan"}, {"MN", "Minnesota"}, {"MS", "Mississippi"},
			{"MO", "Missouri"}, {"MT", "Montana"}, {"NE", "Nebraska"}, {"NV", "Nevada"}, {"NH", "New Hampshire"},
			{"NJ", "New Jersey"}, {"NM", "New Mexico"}, {"NY", "New York"}, {"NC", "North Carolina"},
			{"ND", "North Dakota"}, {"OH", "Ohio"}, {"OK", "Oklahoma"}, {"OR", "Oregon"}, {"PA", "Pennsylvania"},
			{"RI", "Rhode Island"}, {"SC", "South Carolina"}, {"SD", "South Dakota"}, {"TN", "Tennessee"},
			{"TX", "Texas"}, {"UT", "Utah"}, {"VT", "Vermont"}, {"VA", "Virginia"}, {"WA", "Washington"},
			{"WV", "West Virginia"}, {"WI", "Wisconsin"}, {"WY", "Wyoming"},
			// O Distrito de Columbia, os territórios e os códigos postais militares.
			{"DC", "District of Columbia"}, {"PR", "Puerto Rico"}, {"GU", "Guam"}, {"VI", "U.S. Virgin Islands"},
			{"AS", "American Samoa"}, {"MP", "Northern Mariana Islands"}, {"AA", "Armed Forces Americas"},
			{"AE", "Armed Forces Europe"}, {"AP", "Armed Forces Pacific"},
		},
	},
}
