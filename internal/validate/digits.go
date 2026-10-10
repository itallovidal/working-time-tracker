package validate

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// digitsOnly junta os dígitos de s, ignorando os separadores de skip. Qualquer outro
// caractere invalida: um identificador com letra não vira número por engano.
func digitsOnly(s, skip string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune(skip, r):
		default:
			return "", false
		}
	}
	return b.String(), true
}

// EIN confere o Employer Identification Number dos EUA (XX-XXXXXXX) e devolve os 9
// dígitos, sem máscara. O prefixo 00 não existe. Os outros prefixos não são
// conferidos contra a lista do IRS, porque ela ganha prefixos novos.
func EIN(s string) (string, bool) {
	d, ok := digitsOnly(s, " -")
	if !ok || len(d) != 9 || d[:2] == "00" {
		return "", false
	}
	return d, true
}

// GenericTaxID aceita o documento fiscal de um país sem regra própria: letras, dígitos,
// espaço, ponto, hífen, barra e sublinhado, de 1 a 32 caracteres, com os espaços
// de sobra reduzidos. Não confere dígito verificador, porque cada país tem o seu.
// Vazio é inválido: quem chama já tratou o campo vazio como "apagar".
func GenericTaxID(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > MaxTaxID {
		return "", false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && !strings.ContainsRune("./-_", r) {
			return "", false
		}
	}
	return s, true
}
