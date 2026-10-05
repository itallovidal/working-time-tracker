package validate

import "regexp"

var phonePattern = regexp.MustCompile(`^[0-9+() .-]{8,32}$`)

// Phone aceita números, espaços, +, parênteses, ponto e hífen, de 8 a 32 caracteres.
func Phone(s string) bool {
	return phonePattern.MatchString(s)
}
