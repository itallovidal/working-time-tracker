// Package validate reúne as validações de formato usadas por mais de um domínio.
package validate

import "strings"

// CNPJ tira a máscara e confere os dígitos verificadores, devolvendo os 14
// caracteres. Aceita o CNPJ alfanumérico (letras nas 12 primeiras posições), em
// que cada caractere vale o código ASCII menos 48.
func CNPJ(s string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		switch {
		case (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z'):
			b.WriteRune(r)
		case r == '.' || r == '/' || r == '-' || r == ' ':
		default:
			return "", false
		}
	}
	c := b.String()
	if len(c) != 14 || strings.Count(c, c[:1]) == 14 {
		return "", false
	}
	if c[12] != cnpjDigit(c[:12]) || c[13] != cnpjDigit(c[:13]) {
		return "", false
	}
	return c, true
}

// cnpjDigit calcula um dígito verificador: módulo 11 com pesos de 2 a 9, da
// direita para a esquerda.
func cnpjDigit(base string) byte {
	sum, weight := 0, 2
	for i := len(base) - 1; i >= 0; i-- {
		sum += int(base[i]-'0') * weight
		if weight++; weight > 9 {
			weight = 2
		}
	}
	if rest := sum % 11; rest >= 2 {
		return byte('0' + 11 - rest)
	}
	return '0'
}
