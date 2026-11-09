package oauth

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

var errBadResource = errors.New("oauth: resource is not an absolute http(s) URL without query or fragment")

// CanonicalResource é a forma canônica de um endereço de recurso (RFC 8707), para comparar o que o app manda com o
// /mcp: esquema e nome em minúsculas, sem a porta padrão e sem barra no fim. Um endereço com query, fragmento ou
// usuário não é recurso.
func CanonicalResource(raw string) (string, error) {
	u, err := url.Parse(raw)
	// O url.Parse já põe o esquema em minúsculas.
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil ||
		strings.ContainsAny(raw, "?#") {
		return "", errBadResource
	}
	scheme := u.Scheme
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	return scheme + "://" + host + path, nil
}
