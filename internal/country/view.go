package country

import "strings"

// Translate traduz uma chave do catálogo no idioma da requisição (é o T da página).
type Translate func(key string, args ...any) string

// LegalIDView, AddressView e View são o que o navegador recebe de cada país, já com os textos no idioma da requisição. A tela se desenha a partir disso: um país novo no cadastro aparece sem mexer no JavaScript.
type (
	LegalIDView struct {
		Field       string `json:"field"`
		Label       string `json:"label"`
		Placeholder string `json:"placeholder"`
		Mask        string `json:"mask"`
	}
	AddressView struct {
		Line1Placeholder string `json:"line1_placeholder"`
		Line2Label       string `json:"line2_label"`
	}
	View struct {
		Code     string      `json:"code"`
		Langs    []string    `json:"langs"`
		Currency string      `json:"currency"`
		Timezone string      `json:"timezone"`
		LegalID  LegalIDView `json:"legal_id"`
		Address  AddressView `json:"address"`
	}
	// Generic é o que vale para um país fora do cadastro (só o país de um cliente chega aqui).
	Generic struct {
		LegalID struct {
			Label string `json:"label"`
		} `json:"legal_id"`
	}
	// Registry é o window.BOOT.countries.
	Registry struct {
		List    []View   `json:"list"`
		Generic Generic  `json:"generic"`
		ISO     []string `json:"iso"`
	}
)

// textKeys são as chaves do catálogo que cada país precisa ter, nos dois idiomas. Um teste confere.
func textKeys(c *Country) []string {
	base := "countries." + strings.ToLower(c.Code) + "."
	return []string{
		base + "legal_id.label", base + "legal_id.placeholder",
		base + "address.line1_placeholder", base + "address.line2_label",
	}
}

// genericKey é o texto do documento fiscal de um país sem regra própria.
const genericKey = "countries.generic.legal_id.label"

// Describe monta o que vai para o navegador, com os textos no idioma de t.
func Describe(t Translate) Registry {
	r := Registry{List: make([]View, 0, len(order)), ISO: ISO()}
	for _, c := range order {
		base := "countries." + strings.ToLower(c.Code) + "."
		r.List = append(r.List, View{
			Code:     c.Code,
			Langs:    c.Langs,
			Currency: c.Currency,
			Timezone: c.Timezone,
			LegalID: LegalIDView{
				Field:       c.LegalID.Field,
				Label:       t(base + "legal_id.label"),
				Placeholder: t(base + "legal_id.placeholder"),
				Mask:        c.LegalID.Mask,
			},
			Address: AddressView{
				Line1Placeholder: t(base + "address.line1_placeholder"),
				Line2Label:       t(base + "address.line2_label"),
			},
		})
	}
	r.Generic.LegalID.Label = t(genericKey)
	return r
}
