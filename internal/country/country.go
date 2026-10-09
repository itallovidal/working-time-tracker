// Package country é o cadastro dos países que a organização conhece. Cada entrada diz o que muda de um país para
// outro: o documento fiscal da empresa (CNPJ, EIN), os textos do endereço, a moeda e o fuso de quem acaba de criar a
// organização. O banco tem uma tabela só, com todas as colunas; este pacote decide quais delas
// valem para o país e como se validam. Um país novo é uma entrada nova aqui, mais os textos em locales/*.yaml.
//
// Os rótulos e as máscaras vão para o navegador (ver Describe), que desenha a tela a partir deles, e os validadores
// ficam só no servidor, que continua sendo quem decide.
package country

import (
	"strings"

	"working-time-tracker/internal/validate"
)

// Default é o país de quem não diz qual é: o do primeiro uso do sistema.
const Default = "BR"

// LegalID é o documento fiscal da empresa que só existe em alguns países.
type LegalID struct {
	// Field é a coluna da organização e a chave dela no JSON: "cnpj", "ein".
	Field string
	// Mask descreve como o campo se mostra: 9 é um dígito, * é um dígito ou uma letra, o resto é literal.
	Mask string
	// Normalize confere o valor e devolve a forma guardada, sem máscara.
	Normalize func(string) (string, bool)
}

// Country é uma entrada do cadastro.
type Country struct {
	// Code é o código do país (BR, US), o que a organização guarda.
	Code string
	// Aliases são grafias antigas que Parse ainda entende, em minúsculas: o campo país já foi texto livre.
	Aliases []string
	// Langs são os idiomas da interface (o primeiro trecho do código: "pt", "en") que sugerem este país.
	Langs []string
	// Currency e Timezone valem para a organização nova, e para o campo que a pessoa esvazia na edição.
	Currency, Timezone string
	LegalID            LegalID
}

var (
	order  = []*Country{brazil, unitedStates}
	byCode = func() map[string]*Country {
		m := make(map[string]*Country, len(order))
		for _, c := range order {
			m[c.Code] = c
		}
		return m
	}()
)

// All lista os países na ordem em que a tela os mostra.
func All() []*Country { return append([]*Country(nil), order...) }

// Get devolve o país de um código já normalizado (maiúsculas).
func Get(code string) (*Country, bool) {
	c, ok := byCode[code]
	return c, ok
}

// MustGet é o Get para quem tem um código que Parse já aceitou.
func MustGet(code string) *Country {
	c, ok := byCode[code]
	if !ok {
		panic("country: unknown code " + code)
	}
	return c
}

// Parse entende um código (BR, us) ou uma grafia antiga ("Brasil", "EUA") e devolve o código do país. Vazio não vale
// aqui: quem chama decide se vazio quer dizer Default.
func Parse(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if _, ok := byCode[strings.ToUpper(s)]; ok {
		return strings.ToUpper(s), true
	}
	lower := strings.ToLower(s)
	for _, c := range order {
		for _, a := range c.Aliases {
			if a == lower {
				return c.Code, true
			}
		}
	}
	return "", false
}

// ForLang escolhe o país sugerido para um idioma da interface ("pt-BR", "en"), ou Default.
func ForLang(lang string) string {
	tag := strings.ToLower(lang)
	if i := strings.IndexAny(tag, "-_"); i >= 0 {
		tag = tag[:i]
	}
	for _, c := range order {
		for _, l := range c.Langs {
			if l == tag {
				return c.Code
			}
		}
	}
	return Default
}

// CustomerCountry confere o país de um cliente: os do cadastro ou qualquer código ISO 3166-1, porque a organização
// atende clientes de qualquer lugar. Devolve o código em maiúsculas.
func CustomerCountry(code string) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if _, ok := byCode[code]; ok {
		return code, true
	}
	if isoSet[code] {
		return code, true
	}
	return "", false
}

// NormalizeTaxID confere o documento fiscal de um cliente do país. Os países do cadastro têm regra própria (CNPJ,
// EIN); os outros aceitam texto livre curto, sem conferência. Vazio é inválido: quem chama já tratou vazio como apagar.
func NormalizeTaxID(code, raw string) (string, bool) {
	if c, ok := byCode[code]; ok && c.LegalID.Normalize != nil {
		return c.LegalID.Normalize(raw)
	}
	return validate.GenericTaxID(raw)
}
