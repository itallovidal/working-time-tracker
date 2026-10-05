package validate

import (
	"net/url"
	"strings"
	"unicode"
)

// HTTPURL normaliza um endereço web e só aceita http e https, para que o valor
// possa virar um link na tela. Sem esquema, assume https: "acme.com.br" vale.
func HTTPURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", false
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", false
	}
	host := u.Hostname()
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", false
	}
	return u.String(), true
}
