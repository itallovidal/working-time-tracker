package validate

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"working-time-tracker/internal/apperr"
)

// Text apara os espaços das pontas e confere o tamanho. Vazio é aceito: quem exige o campo usa Required. field é o
// nome do campo na API, e a tela mostra o erro embaixo dele.
func Text(field, v string, max int) (string, error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", apperr.ErrFieldTooLong.With("field", field, "max", max)
	}
	return v, nil
}

// Required é Text com o campo obrigatório: vazio, ou só espaços, é recusado.
func Required(field, v string, max int) (string, error) {
	v, err := Text(field, v, max)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", apperr.ErrFieldRequired.With("field", field)
	}
	return v, nil
}

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@.]+(\.[^\s@.]+)+$`)

// EmailFormat diz se v tem o formato local@dominio.tld, sem espaços e com até 255 caracteres. Não confere o e-mail em
// si: a confirmação real seria por mensagem.
func EmailFormat(v string) bool {
	return utf8.RuneCountInString(v) <= MaxEmail && emailPattern.MatchString(v)
}

// Email apara, deixa em minúsculas e confere o formato e o tamanho. Vazio é aceito: quem exige o campo confere antes.
func Email(field, v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", nil
	}
	if utf8.RuneCountInString(v) > MaxEmail {
		return "", apperr.ErrFieldTooLong.With("field", field, "max", MaxEmail)
	}
	if !emailPattern.MatchString(v) {
		return "", apperr.ErrFieldInvalid.With("field", field)
	}
	return v, nil
}

var phonePattern = regexp.MustCompile(`^[0-9+() .-]{8,32}$`)

// Phone aceita números, espaços, +, parênteses, ponto e hífen, de 8 a 32 caracteres e com pelo menos 7 dígitos:
// "--------" tem o tamanho e não é um telefone.
func Phone(s string) bool {
	if !phonePattern.MatchString(s) {
		return false
	}
	digits := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits >= MinPhoneDigits
}

// Money confere um valor em centavos: de 0 a MaxCents.
func Money(field string, cents int) error {
	if cents < 0 || cents > MaxCents {
		return apperr.ErrFieldInvalid.With("field", field)
	}
	return nil
}
